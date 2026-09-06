package product

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/config"
	"go-boilerplate/internal/domain/lexicon/money"
	domainproduct "go-boilerplate/internal/domain/product"
	"go-boilerplate/internal/infrastructure/rdb/driver"
	"go-boilerplate/internal/infrastructure/rdb/pgerror"
	"go-boilerplate/internal/infrastructure/rdb/sqlc/gen"
	"go-boilerplate/internal/infrastructure/rdb/testkit"
	"go-boilerplate/internal/infrastructure/system"
	"go-boilerplate/internal/logging"
	"go-boilerplate/internal/observability"
	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/ptr"
	"go-boilerplate/pkg/safecast"
	"go-boilerplate/pkg/uuid"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 既存 seed に含まれるカテゴリ / ステータス ID（FK 制約を満たすために使用）。
const (
	categoryElectronics = "5dd52d84-78eb-4a52-ba0b-2e11c95c2af2"
	categoryBooks       = "b39be992-fe5a-4b4c-9f98-e695f0f5101e"
	categoryClothes     = "3a60c501-7049-4a63-bfd3-bf34555f3aec"
	statusInStock       = "093170fb-83a2-4864-a2b3-53236eaf3597"
	statusOutOfStock    = "f33654fe-1041-498d-be18-3a1384c10df4"
	// 上の ID に対応する code。code 絞り込みは ID ではなくこちらを指定します。
	categoryCodeElectronics int16 = 1
	categoryCodeBooks       int16 = 2
	statusCodeInStock       int16 = 1
	statusCodeOutOfStock    int16 = 2
	// maxCategoryCode は、どのマスタ行にも割り当たっていない code です（絞り込みが 0 件になる側の確認用）。
	maxCategoryCode int16 = 32767
	// probeKeyword は、seed データと隔離してテスト挿入行のみを対象化するための一意なキーワードです。
	probeKeyword = "KEYSETPROBE563"
	// probeProductPrice は、insertProductAt が挿入する商品の価格。価格での絞り込みを検証する
	// テストが同じ値をフィルタへ渡すため、固定値にしている。
	probeProductPrice = 1000
)

// testCreatedAt は、テスト用の商品の登録日時です。公開日時と取り違えを検出できるよう異なる値にします。
var testCreatedAt = time.Date(2025, time.December, 31, 23, 59, 58, 0, time.UTC)

// batchProductFetch は、ID 集合から商品をまとめて取得する Repository メソッドの形です。
type batchProductFetch func(ctx context.Context, ids []uuid.UUID) (domainproduct.Products, error)

// insertProduct は、keyset 検証用に published_at / id を明示した商品を挿入するヘルパーです。
func insertProduct(
	ctx context.Context, t *testing.T, db driver.DBTX,
	id, name string, description *string, price int, statusID, categoryID string, publishedAt *time.Time,
) {
	t.Helper()
	_, err := db.Exec(ctx,
		"INSERT INTO products "+
			"(id, name, description, price, quantity, stock_warning_threshold, status_id, category_id, published_at) "+
			"VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)",
		id, name, description, price, 10, nil, statusID, categoryID, publishedAt,
	)
	require.NoError(t, err)
}

// insertProductAt は、登録日時を明示して商品を挿入します。
// 未公開込みの一覧は登録日時で並ぶため、DEFAULT now() 任せでは順序を検証できません。
func insertProductAt(
	ctx context.Context, t *testing.T, db driver.DBTX,
	id, name string, publishedAt *time.Time, createdAt time.Time,
) {
	t.Helper()
	_, err := db.Exec(ctx,
		"INSERT INTO products "+
			"(id, name, description, price, quantity, stock_warning_threshold, status_id, category_id, "+
			"published_at, created_at) "+
			"VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)",
		id, name, nil, probeProductPrice, 10, nil, statusInStock, categoryElectronics, publishedAt, createdAt,
	)
	require.NoError(t, err)
}

func mustParse(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	require.NoError(t, err)
	return id
}

func mustPriceFilter(t *testing.T) money.Price {
	t.Helper()
	price, err := money.NewPrice(decimal.FromInt(1000))
	require.NoError(t, err)
	return price
}

func Test_repository_FindPublishedList(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	lt := observability.NewMockInfraLayerTracer(t)
	txm := testkit.NewTestTransactionRunner(t)

	repo := &repository{tracer: lt, db: testDB}

	// seed より確実に新しい固定時刻。id の大小でタイブレークを検証するため tie ペアを含む。
	base := time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)
	tieHigh := "eeeeeeee-0000-4000-8000-000000000002"
	tieLow := "eeeeeeee-0000-4000-8000-000000000001"
	mid := "eeeeeeee-0000-4000-8000-000000000003"  // base-1h
	old := "eeeeeeee-0000-4000-8000-000000000004"  // base-2h
	null := "eeeeeeee-0000-4000-8000-000000000005" // published_at=NULL

	// insertProbeSet は、tie ペア + 2 件 + 未公開(NULL) を probeKeyword 付きで挿入します。
	insertProbeSet := func(ctx context.Context, t *testing.T, drv driver.DBTX) {
		t.Helper()
		insertProduct(ctx, t, drv, tieHigh, probeKeyword+"-A", nil, 1000, statusInStock, categoryElectronics, ptr.To(base))
		insertProduct(ctx, t, drv, tieLow, probeKeyword+"-B", nil, 1000, statusInStock, categoryElectronics, ptr.To(base))
		insertProduct(ctx, t, drv, mid, probeKeyword+"-C", nil, 1000, statusInStock, categoryElectronics, ptr.To(base.Add(-time.Hour)))
		insertProduct(ctx, t, drv, old, probeKeyword+"-D", nil, 1000, statusInStock, categoryElectronics, ptr.To(base.Add(-2*time.Hour)))
		insertProduct(ctx, t, drv, null, probeKeyword+"-NULL", nil, 1000, statusInStock, categoryElectronics, nil)
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("降順で先頭ページとafter境界がkeyset安定順に次ページを返し未公開行は除外される", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				insertProbeSet(ctx, t, drv)
				price := mustPriceFilter(t)
				quantity := int32(10)

				// 先頭ページ(limit=2): 公開日時降順・id降順で tieHigh, tieLow。
				firstPage, err := repo.FindPublishedList(ctx, domainproduct.ListParams{
					Keyword:     ptr.To(probeKeyword),
					MinPrice:    &price,
					MaxPrice:    &price,
					MinQuantity: &quantity,
					MaxQuantity: &quantity,
					Limit:       2,
					Ascending:   false,
				})
				require.NoError(t, err)
				require.Len(t, firstPage, 2)
				assert.Equal(t, mustParse(t, tieHigh), firstPage[0].ID())
				assert.Equal(t, mustParse(t, tieLow), firstPage[1].ID())

				// 次ページ: 末尾行(tieLow)を境界に keyset を進めると mid → old。
				last := firstPage[len(firstPage)-1]
				secondPage, err := repo.FindPublishedList(ctx, domainproduct.ListParams{
					Keyword:          ptr.To(probeKeyword),
					MinPrice:         &price,
					MaxPrice:         &price,
					MinQuantity:      &quantity,
					MaxQuantity:      &quantity,
					Limit:            2,
					Ascending:        false,
					AfterPublishedAt: last.PublishedAt(),
					AfterID:          ptr.To(last.ID()),
				})
				require.NoError(t, err)
				require.Len(t, secondPage, 2)
				assert.Equal(t, mustParse(t, mid), secondPage[0].ID())
				assert.Equal(t, mustParse(t, old), secondPage[1].ID())

				// 未公開(NULL)行はどのページにも現れない。
				all, err := repo.FindPublishedList(ctx, domainproduct.ListParams{
					Keyword:   ptr.To(probeKeyword),
					Limit:     100,
					Ascending: false,
				})
				require.NoError(t, err)
				for _, p := range all {
					assert.NotEqual(t, mustParse(t, null), p.ID())
				}
			})
		})

		t.Run("範囲条件付きの昇順で先頭ページとafter境界がkeyset安定順に次ページを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				insertProbeSet(ctx, t, drv)
				price := mustPriceFilter(t)
				quantity := int32(10)

				firstPage, err := repo.FindPublishedList(ctx, domainproduct.ListParams{
					Keyword:     ptr.To(probeKeyword),
					MinPrice:    &price,
					MaxPrice:    &price,
					MinQuantity: &quantity,
					MaxQuantity: &quantity,
					Limit:       2,
					Ascending:   true,
				})
				require.NoError(t, err)
				require.Len(t, firstPage, 2)
				assert.Equal(t, mustParse(t, old), firstPage[0].ID())
				assert.Equal(t, mustParse(t, mid), firstPage[1].ID())

				last := firstPage[len(firstPage)-1]
				secondPage, err := repo.FindPublishedList(ctx, domainproduct.ListParams{
					Keyword:          ptr.To(probeKeyword),
					MinPrice:         &price,
					MaxPrice:         &price,
					MinQuantity:      &quantity,
					MaxQuantity:      &quantity,
					Limit:            2,
					Ascending:        true,
					AfterPublishedAt: last.PublishedAt(),
					AfterID:          ptr.To(last.ID()),
				})
				require.NoError(t, err)
				require.Len(t, secondPage, 2)
				assert.Equal(t, mustParse(t, tieLow), secondPage[0].ID())
				assert.Equal(t, mustParse(t, tieHigh), secondPage[1].ID())
			})
		})

		t.Run("category_idフィルタは該当カテゴリのみを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				bookID := "eeeeeeee-0000-4000-8000-0000000000b1"
				insertProduct(ctx, t, drv, tieHigh, probeKeyword+"-E", nil, 1000, statusInStock, categoryElectronics, ptr.To(base))
				insertProduct(ctx, t, drv, bookID, probeKeyword+"-BOOK", nil, 1000, statusInStock, categoryBooks, ptr.To(base))

				categoryID := mustParse(t, categoryBooks)
				page, err := repo.FindPublishedList(ctx, domainproduct.ListParams{
					Keyword: ptr.To(probeKeyword), CategoryID: &categoryID,
					Limit:     10,
					Ascending: false,
				})
				require.NoError(t, err)
				require.Len(t, page, 1)
				assert.Equal(t, mustParse(t, bookID), page[0].ID())
			})
		})

		t.Run("category_codesフィルタは指定したカテゴリの和集合を返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				bookID := "eeeeeeee-0000-4000-8000-0000000000b2"
				clothesID := "eeeeeeee-0000-4000-8000-0000000000b3"
				insertProduct(ctx, t, drv, tieHigh, probeKeyword+"-CE", nil, 1000, statusInStock, categoryElectronics, ptr.To(base))
				insertProduct(ctx, t, drv, bookID, probeKeyword+"-CB", nil, 1000, statusInStock, categoryBooks, ptr.To(base))
				insertProduct(ctx, t, drv, clothesID, probeKeyword+"-CC", nil, 1000, statusInStock, categoryClothes, ptr.To(base))

				page, err := repo.FindPublishedList(ctx, domainproduct.ListParams{
					Keyword:       ptr.To(probeKeyword),
					CategoryCodes: []int16{categoryCodeElectronics, categoryCodeBooks},
					Limit:         10,
					Ascending:     false,
				})
				require.NoError(t, err)
				require.Len(t, page, 2)

				ids := []uuid.UUID{page[0].ID(), page[1].ID()}
				assert.ElementsMatch(t, []uuid.UUID{mustParse(t, tieHigh), mustParse(t, bookID)}, ids)
			})
		})

		t.Run("空のcategory_codesは絞り込みなしとして扱う", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				insertProduct(ctx, t, drv, tieHigh, probeKeyword+"-CZ", nil, 1000, statusInStock, categoryElectronics, ptr.To(base))

				// 非 nil の空スライスは「未指定」であり「どのコードにも一致しない」ではない。
				page, err := repo.FindPublishedList(ctx, domainproduct.ListParams{
					Keyword:       ptr.To(probeKeyword),
					CategoryCodes: []int16{},
					Limit:         10,
					Ascending:     false,
				})
				require.NoError(t, err)
				require.Len(t, page, 1)
				assert.Equal(t, mustParse(t, tieHigh), page[0].ID())
			})
		})

		t.Run("存在しないcategory_codeを指定した場合、0件を返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				insertProduct(ctx, t, drv, tieHigh, probeKeyword+"-CN", nil, 1000, statusInStock, categoryElectronics, ptr.To(base))

				page, err := repo.FindPublishedList(ctx, domainproduct.ListParams{
					Keyword:       ptr.To(probeKeyword),
					CategoryCodes: []int16{maxCategoryCode},
					Limit:         10,
					Ascending:     false,
				})
				require.NoError(t, err)
				assert.Empty(t, page)
			})
		})

		t.Run("status_codesフィルタは指定したステータスの和集合を返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				outID := "eeeeeeee-0000-4000-8000-0000000000c2"
				insertProduct(ctx, t, drv, tieHigh, probeKeyword+"-SI", nil, 1000, statusInStock, categoryElectronics, ptr.To(base))
				insertProduct(ctx, t, drv, outID, probeKeyword+"-SO", nil, 1000, statusOutOfStock, categoryElectronics, ptr.To(base))

				page, err := repo.FindPublishedList(ctx, domainproduct.ListParams{
					Keyword:     ptr.To(probeKeyword),
					StatusCodes: []int16{statusCodeInStock, statusCodeOutOfStock},
					Limit:       10,
					Ascending:   false,
				})
				require.NoError(t, err)
				require.Len(t, page, 2)

				ids := []uuid.UUID{page[0].ID(), page[1].ID()}
				assert.ElementsMatch(t, []uuid.UUID{mustParse(t, tieHigh), mustParse(t, outID)}, ids)
			})
		})

		t.Run("status_codesフィルタは指定しなかったステータスを除く", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				outID := "eeeeeeee-0000-4000-8000-0000000000c3"
				insertProduct(ctx, t, drv, tieHigh, probeKeyword+"-SX", nil, 1000, statusInStock, categoryElectronics, ptr.To(base))
				insertProduct(ctx, t, drv, outID, probeKeyword+"-SY", nil, 1000, statusOutOfStock, categoryElectronics, ptr.To(base))

				page, err := repo.FindPublishedList(ctx, domainproduct.ListParams{
					Keyword:     ptr.To(probeKeyword),
					StatusCodes: []int16{statusCodeOutOfStock},
					Limit:       10,
					Ascending:   false,
				})
				require.NoError(t, err)
				require.Len(t, page, 1)
				assert.Equal(t, mustParse(t, outID), page[0].ID())
			})
		})

		t.Run("昇順・カーソル以降のページでもcode絞り込みが効く", func(t *testing.T) {
			t.Parallel()

			// code 条件は 4 本のクエリ（昇順／降順 × 先頭／カーソル以降）へ個別に書かれているため、
			// 既定の降順・先頭ページだけを通しても残る 3 本の書き間違いを検出できない。
			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				bookID := "eeeeeeee-0000-4000-8000-0000000000c4"
				insertProduct(ctx, t, drv, tieHigh, probeKeyword+"-AA", nil, 1000, statusInStock, categoryElectronics, ptr.To(base))
				insertProduct(ctx, t, drv, bookID, probeKeyword+"-AB", nil, 1000, statusInStock, categoryBooks, ptr.To(base.Add(time.Hour)))

				filter := domainproduct.SearchFilter{
					Keyword:       ptr.To(probeKeyword),
					CategoryCodes: []int16{categoryCodeElectronics, categoryCodeBooks},
				}

				first, err := repo.FindPublishedList(ctx, domainproduct.ListParams{
					SearchFilter: filter, Limit: 1, Ascending: true,
				})
				require.NoError(t, err)
				require.Len(t, first, 1)
				assert.Equal(t, mustParse(t, tieHigh), first[0].ID())

				next, err := repo.FindPublishedList(ctx, domainproduct.ListParams{
					SearchFilter:     filter,
					Limit:            10,
					Ascending:        true,
					AfterPublishedAt: first[0].PublishedAt(),
					AfterID:          ptr.To(first[0].ID()),
				})
				require.NoError(t, err)
				require.Len(t, next, 1)
				assert.Equal(t, mustParse(t, bookID), next[0].ID())
			})
		})

		t.Run("status_idフィルタは該当ステータスのみを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				outID := "eeeeeeee-0000-4000-8000-0000000000c1"
				insertProduct(ctx, t, drv, tieHigh, probeKeyword+"-F", nil, 1000, statusInStock, categoryElectronics, ptr.To(base))
				insertProduct(ctx, t, drv, outID, probeKeyword+"-OUT", nil, 1000, statusOutOfStock, categoryElectronics, ptr.To(base))

				statusID := mustParse(t, statusOutOfStock)
				page, err := repo.FindPublishedList(ctx, domainproduct.ListParams{
					Keyword: ptr.To(probeKeyword), StatusID: &statusID,
					Limit:     10,
					Ascending: false,
				})
				require.NoError(t, err)
				require.Len(t, page, 1)
				assert.Equal(t, mustParse(t, outID), page[0].ID())
			})
		})

		t.Run("keywordは名称と説明の両方に部分一致する", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				nameHit := "eeeeeeee-0000-4000-8000-0000000000d1"
				descHit := "eeeeeeee-0000-4000-8000-0000000000d2"
				// name に一致。
				insertProduct(ctx, t, drv, nameHit, probeKeyword+"-NAME", nil, 1000, statusInStock, categoryElectronics, ptr.To(base))
				// description に一致（name は非一致）。
				insertProduct(
					ctx,
					t,
					drv,
					descHit,
					"無関係な商品名",
					ptr.To("これは "+probeKeyword+" を含む説明"),
					1000,
					statusInStock,
					categoryElectronics,
					ptr.To(base.Add(-time.Hour)),
				)

				page, err := repo.FindPublishedList(ctx, domainproduct.ListParams{
					Keyword:   ptr.To(probeKeyword),
					Limit:     10,
					Ascending: false,
				})
				require.NoError(t, err)
				require.Len(t, page, 2)
			})
		})

		t.Run("価格と在庫数は境界値を含む範囲で絞り込まれる", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				lowID := "eeeeeeee-0000-4000-8000-0000000000f1"
				matchID := "eeeeeeee-0000-4000-8000-0000000000f2"
				highID := "eeeeeeee-0000-4000-8000-0000000000f3"
				priceInQuantityOutID := "eeeeeeee-0000-4000-8000-0000000000f4"
				priceOutQuantityInID := "eeeeeeee-0000-4000-8000-0000000000f5"
				insertProduct(ctx, t, drv, lowID, probeKeyword+"-RANGE-LOW", nil, 999, statusInStock, categoryElectronics, ptr.To(base))
				insertProduct(ctx, t, drv, matchID, probeKeyword+"-RANGE-MATCH", nil, 1000, statusInStock, categoryElectronics, ptr.To(base))
				insertProduct(ctx, t, drv, highID, probeKeyword+"-RANGE-HIGH", nil, 1001, statusInStock, categoryElectronics, ptr.To(base))
				insertProduct(
					ctx, t, drv, priceInQuantityOutID, probeKeyword+"-RANGE-QUANTITY-OUT", nil,
					1000, statusInStock, categoryElectronics, ptr.To(base),
				)
				insertProduct(
					ctx, t, drv, priceOutQuantityInID, probeKeyword+"-RANGE-PRICE-OUT", nil,
					1001, statusInStock, categoryElectronics, ptr.To(base),
				)
				_, err := drv.Exec(
					ctx,
					"UPDATE products SET quantity = CASE id WHEN $1 THEN 9 WHEN $2 THEN 10 ELSE 11 END "+
						"WHERE id = ANY($3::uuid[])",
					lowID,
					matchID,
					[]string{lowID, matchID, highID, priceInQuantityOutID, priceOutQuantityInID},
				)
				require.NoError(t, err)
				_, err = drv.Exec(ctx, "UPDATE products SET quantity = 10 WHERE id = $1", priceOutQuantityInID)
				require.NoError(t, err)

				price := mustPriceFilter(t)
				quantity := int32(10)
				page, err := repo.FindPublishedList(ctx, domainproduct.ListParams{
					Keyword:     ptr.To(probeKeyword + "-RANGE"),
					MinPrice:    &price,
					MaxPrice:    &price,
					MinQuantity: &quantity,
					MaxQuantity: &quantity,
					Limit:       10,
					Ascending:   false,
				})
				require.NoError(t, err)
				require.Len(t, page, 1)
				assert.Equal(t, mustParse(t, matchID), page[0].ID())
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("limitが負数の場合、ErrInternalへ正規化される", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				actual, err := repo.FindPublishedList(ctx, domainproduct.ListParams{Limit: -1, Ascending: false})
				assert.Nil(t, actual)
				require.ErrorIs(t, err, apperror.ErrInternal)
			})
		})

		t.Run("不正な行(価格が負数)が含まれるとデータ不整合としてErrInternalを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				invalidID := "eeeeeeee-0000-4000-8000-0000000000e1"
				// price=-1 はドメイン不変条件違反(再構築エラー)を誘発する。
				insertProduct(ctx, t, drv, invalidID, probeKeyword+"-BADPRICE", nil, -1, statusInStock, categoryElectronics, ptr.To(base))

				actual, err := repo.FindPublishedList(ctx, domainproduct.ListParams{
					Keyword:   ptr.To(probeKeyword),
					Limit:     10,
					Ascending: false,
				})
				assert.Nil(t, actual)
				require.ErrorIs(t, err, apperror.ErrInternal)
				require.NotErrorIs(t, err, money.ErrNegativePrice)
			})
		})
	})
}

func Test_repository_CountPublished(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	repo := &repository{tracer: observability.NewMockInfraLayerTracer(t), db: testDB}
	txm := testkit.NewTestTransactionRunner(t)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("公開済み商品のうち全検索条件に一致する件数を返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				publishedAt := time.Date(2099, time.February, 1, 0, 0, 0, 0, time.UTC)
				keyword := "COUNTPROBE721"
				insertProduct(
					ctx, t, drv, "dddddddd-0000-4000-8000-000000000001",
					keyword+"-match", nil, 1000, statusInStock, categoryElectronics, &publishedAt,
				)
				insertProduct(
					ctx, t, drv, "dddddddd-0000-4000-8000-000000000002",
					keyword+"-price-out", nil, 2000, statusInStock, categoryElectronics, &publishedAt,
				)
				insertProduct(
					ctx, t, drv, "dddddddd-0000-4000-8000-000000000003",
					keyword+"-unpublished", nil, 1000, statusInStock, categoryElectronics, nil,
				)

				price := mustPriceFilter(t)
				count, err := repo.CountPublished(ctx, domainproduct.SearchFilter{
					CategoryID: ptr.To(mustParse(t, categoryElectronics)),
					StatusID:   ptr.To(mustParse(t, statusInStock)), Keyword: &keyword,
					MinPrice: &price, MaxPrice: &price,
					MinQuantity: ptr.To[int32](10), MaxQuantity: ptr.To[int32](10),
				})
				require.NoError(t, err)
				assert.Equal(t, int64(1), count)
			})
		})

		t.Run("category_idフィルタは該当カテゴリの件数のみを数える", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				publishedAt := time.Date(2099, time.February, 2, 0, 0, 0, 0, time.UTC)
				keyword := "COUNTPROBECAT"
				insertProduct(
					ctx, t, drv, "dddddddd-0000-4000-8000-00000000000a",
					keyword+"-electronics", nil, 1000, statusInStock, categoryElectronics, &publishedAt,
				)
				insertProduct(
					ctx, t, drv, "dddddddd-0000-4000-8000-00000000000b",
					keyword+"-books", nil, 1000, statusInStock, categoryBooks, &publishedAt,
				)

				count, err := repo.CountPublished(ctx, domainproduct.SearchFilter{
					CategoryID: ptr.To(mustParse(t, categoryElectronics)), Keyword: &keyword,
				})
				require.NoError(t, err)
				assert.Equal(t, int64(1), count)
			})
		})

		t.Run("category_codesフィルタは指定したカテゴリの件数を合算する", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				publishedAt := time.Date(2099, time.February, 2, 0, 0, 0, 0, time.UTC)
				keyword := "COUNTPROBECODES"
				insertProduct(
					ctx, t, drv, "dddddddd-0000-4000-8000-00000000001a",
					keyword+"-electronics", nil, 1000, statusInStock, categoryElectronics, &publishedAt,
				)
				insertProduct(
					ctx, t, drv, "dddddddd-0000-4000-8000-00000000001b",
					keyword+"-books", nil, 1000, statusInStock, categoryBooks, &publishedAt,
				)
				insertProduct(
					ctx, t, drv, "dddddddd-0000-4000-8000-00000000001c",
					keyword+"-clothes", nil, 1000, statusInStock, categoryClothes, &publishedAt,
				)

				count, err := repo.CountPublished(ctx, domainproduct.SearchFilter{
					CategoryCodes: []int16{categoryCodeElectronics, categoryCodeBooks}, Keyword: &keyword,
				})
				require.NoError(t, err)
				assert.Equal(t, int64(2), count)
			})
		})

		t.Run("status_codesフィルタは指定したステータスの件数を合算する", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				publishedAt := time.Date(2099, time.February, 2, 0, 0, 0, 0, time.UTC)
				keyword := "COUNTPROBESTATUS"
				insertProduct(
					ctx, t, drv, "dddddddd-0000-4000-8000-00000000002a",
					keyword+"-in", nil, 1000, statusInStock, categoryElectronics, &publishedAt,
				)
				insertProduct(
					ctx, t, drv, "dddddddd-0000-4000-8000-00000000002b",
					keyword+"-out", nil, 1000, statusOutOfStock, categoryElectronics, &publishedAt,
				)

				count, err := repo.CountPublished(ctx, domainproduct.SearchFilter{
					StatusCodes: []int16{statusCodeOutOfStock}, Keyword: &keyword,
				})
				require.NoError(t, err)
				assert.Equal(t, int64(1), count)
			})
		})

		t.Run("status_idフィルタは該当ステータスの件数のみを数える", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				publishedAt := time.Date(2099, time.February, 3, 0, 0, 0, 0, time.UTC)
				keyword := "COUNTPROBESTATUS"
				insertProduct(
					ctx, t, drv, "dddddddd-0000-4000-8000-00000000000c",
					keyword+"-in-stock", nil, 1000, statusInStock, categoryElectronics, &publishedAt,
				)
				insertProduct(
					ctx, t, drv, "dddddddd-0000-4000-8000-00000000000d",
					keyword+"-out-of-stock", nil, 1000, statusOutOfStock, categoryElectronics, &publishedAt,
				)

				count, err := repo.CountPublished(ctx, domainproduct.SearchFilter{
					StatusID: ptr.To(mustParse(t, statusInStock)), Keyword: &keyword,
				})
				require.NoError(t, err)
				assert.Equal(t, int64(1), count)
			})
		})

		t.Run("keywordは名称と説明の両方に部分一致する", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				publishedAt := time.Date(2099, time.February, 4, 0, 0, 0, 0, time.UTC)
				keyword := "COUNTPROBEKEYWORD"
				insertProduct(
					ctx, t, drv, "dddddddd-0000-4000-8000-00000000000e",
					keyword+"-in-name", nil, 1000, statusInStock, categoryElectronics, &publishedAt,
				)
				insertProduct(
					ctx, t, drv, "dddddddd-0000-4000-8000-00000000000f",
					"unrelated-name", ptr.To("説明に"+keyword+"を含む"),
					1000, statusInStock, categoryElectronics, &publishedAt,
				)
				insertProduct(
					ctx, t, drv, "dddddddd-0000-4000-8000-000000000010",
					"unrelated-other", nil, 1000, statusInStock, categoryElectronics, &publishedAt,
				)

				count, err := repo.CountPublished(ctx, domainproduct.SearchFilter{Keyword: &keyword})
				require.NoError(t, err)
				assert.Equal(t, int64(2), count)
			})
		})

		t.Run("価格と在庫数は境界値を含む範囲で絞り込まれる", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				publishedAt := time.Date(2099, time.February, 5, 0, 0, 0, 0, time.UTC)
				keyword := "COUNTPROBERANGE"
				lowID := "dddddddd-0000-4000-8000-000000000011"
				matchID := "dddddddd-0000-4000-8000-000000000012"
				highID := "dddddddd-0000-4000-8000-000000000013"
				quantityLowID := "dddddddd-0000-4000-8000-000000000018"
				quantityOutID := "dddddddd-0000-4000-8000-000000000014"
				insertProduct(
					ctx, t, drv, lowID, keyword+"-low", nil, 999, statusInStock, categoryElectronics, &publishedAt,
				)
				insertProduct(
					ctx, t, drv, matchID, keyword+"-match", nil, 1000, statusInStock, categoryElectronics, &publishedAt,
				)
				insertProduct(
					ctx, t, drv, highID, keyword+"-high", nil, 1001, statusInStock, categoryElectronics, &publishedAt,
				)
				insertProduct(
					ctx, t, drv, quantityOutID,
					keyword+"-quantity-high", nil, 1000, statusInStock, categoryElectronics, &publishedAt,
				)
				insertProduct(
					ctx, t, drv, quantityLowID,
					keyword+"-quantity-low", nil, 1000, statusInStock, categoryElectronics, &publishedAt,
				)
				_, err := drv.Exec(
					ctx, "UPDATE products SET quantity = CASE id WHEN $1 THEN 11 ELSE 9 END WHERE id = ANY($2::uuid[])",
					quantityOutID, []string{quantityOutID, quantityLowID},
				)
				require.NoError(t, err)

				price := mustPriceFilter(t)
				quantity := int32(10)
				count, err := repo.CountPublished(ctx, domainproduct.SearchFilter{
					Keyword: &keyword, MinPrice: &price, MaxPrice: &price,
					MinQuantity: &quantity, MaxQuantity: &quantity,
				})
				require.NoError(t, err)
				assert.Equal(t, int64(1), count)
			})
		})

		t.Run("一致する公開商品が無い場合は0を返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				count, err := repo.CountPublished(ctx, domainproduct.SearchFilter{
					Keyword: ptr.To("COUNTPROBENOMATCH"),
				})
				require.NoError(t, err)
				assert.Equal(t, int64(0), count)
			})
		})

		t.Run("同一条件では一覧の件数と一致する", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				publishedAt := time.Date(2099, time.February, 6, 0, 0, 0, 0, time.UTC)
				keyword := "COUNTPROBEPARITY"
				insertProduct(
					ctx, t, drv, "dddddddd-0000-4000-8000-000000000015",
					keyword+"-match", nil, 1000, statusInStock, categoryElectronics, &publishedAt,
				)
				insertProduct(
					ctx, t, drv, "dddddddd-0000-4000-8000-000000000016",
					keyword+"-other-category", nil, 1000, statusInStock, categoryBooks, &publishedAt,
				)
				insertProduct(
					ctx, t, drv, "dddddddd-0000-4000-8000-000000000017",
					keyword+"-unpublished", nil, 1000, statusInStock, categoryElectronics, nil,
				)

				categoryID := ptr.To(mustParse(t, categoryElectronics))
				page, err := repo.FindPublishedList(ctx, domainproduct.ListParams{
					CategoryID: categoryID, Keyword: &keyword,
					Limit:     100,
					Ascending: false,
				})
				require.NoError(t, err)
				count, err := repo.CountPublished(ctx, domainproduct.SearchFilter{
					CategoryID: categoryID, Keyword: &keyword,
				})
				require.NoError(t, err)
				assert.Equal(t, int64(len(page)), count)
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("キャンセル済みコンテキストではErrCanceledへ正規化される", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			count, err := repo.CountPublished(ctx, domainproduct.SearchFilter{})
			assert.Zero(t, count)
			require.ErrorIs(t, err, apperror.ErrCanceled)
		})
	})
}

func Test_repository_FindPublishedByID(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	lt := observability.NewMockInfraLayerTracer(t)
	txm := testkit.NewTestTransactionRunner(t)

	repo := &repository{tracer: lt, db: testDB}

	publishedID := "ffffffff-0000-4000-8000-000000000001"
	unpublishedID := "ffffffff-0000-4000-8000-000000000002"
	base := time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("公開中の商品をIDで取得できる", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				insertProduct(ctx, t, drv, publishedID, probeKeyword+"-PUB", ptr.To("説明"), 1999, statusInStock, categoryElectronics, ptr.To(base))

				got, err := repo.FindPublishedByID(ctx, mustParse(t, publishedID))
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, mustParse(t, publishedID), got.ID())
				require.NotNil(t, got.Description())
				assert.Equal(t, "説明", *got.Description())
				assert.Equal(t, "1999", got.Price().String())
				assert.Equal(t, 10, got.Quantity())
				assert.Nil(t, got.StockWarningThreshold())
				// 固定参照マスタとの結合で解決した status / category を ID・名称とも実 DB 経由で固定する
				// （SQL の結合エイリアス取り違え＝status/category の名称入れ替わりを検出するため、異なる 2 値で検証）。
				assert.Equal(t, mustParse(t, statusInStock), got.Status().ID())
				assert.Equal(t, "在庫あり", got.Status().Name())
				assert.Equal(t, mustParse(t, categoryElectronics), got.Category().ID())
				assert.Equal(t, "電子機器", got.Category().Name())
				// DB は timestamptz をローカルタイムゾーンで返すため、格納した瞬間の一致で比較する。
				require.NotNil(t, got.PublishedAt())
				assert.True(t, base.Equal(*got.PublishedAt()))
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未存在のIDはNotFoundを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				got, err := repo.FindPublishedByID(ctx, uuidtestkit.NewTestFromSalt(t, "find_published_by_id_missing"))
				assert.Nil(t, got)
				require.ErrorIs(t, err, apperror.ErrNotFound)
			})
		})

		t.Run("非公開(published_atがNULL)の商品はNotFoundを返し存在を秘匿する", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				// published_at が NULL の行は公開述語に合致しないため、未存在と同じ NotFound が返る（存在秘匿）。
				insertProduct(ctx, t, drv, unpublishedID, probeKeyword+"-UNPUB", nil, 1999, statusInStock, categoryElectronics, nil)

				got, err := repo.FindPublishedByID(ctx, mustParse(t, unpublishedID))
				assert.Nil(t, got)
				require.ErrorIs(t, err, apperror.ErrNotFound)
			})
		})

		t.Run("不正な行(価格が負数)が取得されるとデータ不整合としてErrInternalを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				invalidID := "ffffffff-0000-4000-8000-000000000003"
				// price=-1 はドメイン不変条件違反(再構築エラー)を誘発する。公開述語を通すため published_at は設定する。
				insertProduct(ctx, t, drv, invalidID, probeKeyword+"-BADPRICE", nil, -1, statusInStock, categoryElectronics, ptr.To(base))

				got, err := repo.FindPublishedByID(ctx, mustParse(t, invalidID))
				assert.Nil(t, got)
				require.ErrorIs(t, err, apperror.ErrInternal)
				require.NotErrorIs(t, err, money.ErrNegativePrice)
			})
		})
	})
}

func Test_rowToProduct(t *testing.T) {
	t.Parallel()

	publishedAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	id := uuidtestkit.NewTestFromSalt(t, "row_to_product_id")
	statusID := uuidtestkit.NewTestFromSalt(t, "row_to_product_status")
	categoryID := uuidtestkit.NewTestFromSalt(t, "row_to_product_category")

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("商品行(マスタ JOIN 込み)をドメインエンティティへ変換する", func(t *testing.T) {
			t.Parallel()
			row := gen.Products{
				ID:                    id,
				Name:                  "商品",
				Description:           ptr.To("説明"),
				Price:                 decimal.FromInt(1999),
				Quantity:              100,
				StockWarningThreshold: ptr.To(int32(10)),
				StatusID:              statusID,
				CategoryID:            categoryID,
				PublishedAt:           ptr.To(publishedAt),
				LockVersion:           3,
				CreatedAt:             testCreatedAt,
			}
			images := []domainproduct.Image{
				domainproduct.NewImage(
					uuidtestkit.NewTestFromSalt(t, "row_image"),
					domainproduct.ImageAttributes{ImagePath: "products/earphone.png", DisplaySort: 1},
				),
			}
			got, err := rowToProduct(productRow{p: row, statusName: "在庫あり", categoryName: "電子機器"}, images)
			require.NoError(t, err)
			assert.Equal(t, id, got.ID())
			assert.Equal(t, "商品", got.Name())
			assert.Equal(t, "1999", got.Price().String())
			assert.Equal(t, 100, got.Quantity())
			require.NotNil(t, got.StockWarningThreshold())
			assert.Equal(t, 10, *got.StockWarningThreshold())
			assert.Equal(t, statusID, got.Status().ID())
			assert.Equal(t, "在庫あり", got.Status().Name())
			assert.Equal(t, categoryID, got.Category().ID())
			assert.Equal(t, "電子機器", got.Category().Name())
			require.NotNil(t, got.PublishedAt())
			assert.Equal(t, publishedAt, *got.PublishedAt())
			require.NotNil(t, got.Description())
			assert.Equal(t, "説明", *got.Description())
			require.Len(t, got.Images(), 1)
			assert.Equal(t, "products/earphone.png", got.Images()[0].ImagePath())
			assert.Equal(t, 3, got.Version())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("不変条件違反(価格が負数)はデータ不整合としてErrInternalへ正規化される", func(t *testing.T) {
			t.Parallel()
			row := gen.Products{
				ID:          id,
				Name:        "商品",
				Price:       decimal.FromInt(-1),
				Quantity:    100,
				StatusID:    statusID,
				CategoryID:  categoryID,
				PublishedAt: ptr.To(publishedAt),
				CreatedAt:   testCreatedAt,
			}
			got, err := rowToProduct(productRow{p: row, statusName: "在庫あり", categoryName: "電子機器"}, nil)
			assert.Nil(t, got)
			require.ErrorIs(t, err, apperror.ErrInternal)
		})

		t.Run("JOIN で解決したステータス名が空の場合はデータ不整合としてErrInternalへ正規化される", func(t *testing.T) {
			t.Parallel()
			row := gen.Products{
				ID:          id,
				Name:        "商品",
				Price:       decimal.FromInt(1999),
				Quantity:    100,
				StatusID:    statusID,
				CategoryID:  categoryID,
				PublishedAt: ptr.To(publishedAt),
				CreatedAt:   testCreatedAt,
			}
			got, err := rowToProduct(productRow{p: row, statusName: "", categoryName: "電子機器"}, nil)
			assert.Nil(t, got)
			require.ErrorIs(t, err, apperror.ErrInternal)
		})

		t.Run("JOIN で解決したカテゴリ名が空の場合はデータ不整合としてErrInternalへ正規化される", func(t *testing.T) {
			t.Parallel()
			row := gen.Products{
				ID:          id,
				Name:        "商品",
				Price:       decimal.FromInt(1999),
				Quantity:    100,
				StatusID:    statusID,
				CategoryID:  categoryID,
				PublishedAt: ptr.To(publishedAt),
				CreatedAt:   testCreatedAt,
			}
			got, err := rowToProduct(productRow{p: row, statusName: "在庫あり", categoryName: ""}, nil)
			assert.Nil(t, got)
			require.ErrorIs(t, err, apperror.ErrInternal)
		})

		t.Run("バージョンが下限未満の行はエンティティ再構築の失敗としてErrInternalへ正規化される", func(t *testing.T) {
			t.Parallel()
			row := gen.Products{
				ID:          id,
				Name:        "商品",
				Price:       decimal.FromInt(1999),
				Quantity:    100,
				StatusID:    statusID,
				CategoryID:  categoryID,
				PublishedAt: ptr.To(publishedAt),
				LockVersion: 0,
				CreatedAt:   testCreatedAt,
			}
			got, err := rowToProduct(productRow{p: row, statusName: "在庫あり", categoryName: "電子機器"}, nil)
			assert.Nil(t, got)
			require.ErrorIs(t, err, apperror.ErrInternal)
			require.NotErrorIs(t, err, domainproduct.ErrInvalidVersion)
		})
	})
}

func Test_repository_Create(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	lt := observability.NewMockInfraLayerTracer(t)
	txm := testkit.NewTestTransactionRunner(t)

	repo := &repository{tracer: lt, db: testDB}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("商品を登録し image_path / published_at / 説明を往復して取得できる", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				id := uuidtestkit.NewTestFromSalt(t, "create_roundtrip_id")
				publishedAt := time.Date(2099, time.June, 1, 0, 0, 0, 0, time.UTC)
				statusRef, err := domainproduct.NewStatusRef(mustParse(t, statusInStock), "在庫あり")
				require.NoError(t, err)
				categoryRef, err := domainproduct.NewCategoryRef(mustParse(t, categoryElectronics), "電子機器")
				require.NoError(t, err)
				price, err := money.NewPrice(decimal.FromInt(1999))
				require.NoError(t, err)
				entity, err := domainproduct.New(id, domainproduct.Attributes{
					Name:                  "作成商品",
					Description:           ptr.To("<p>リッチテキスト説明</p>"),
					Price:                 price,
					Quantity:              100,
					StockWarningThreshold: ptr.To(10),
					Status:                statusRef,
					Category:              categoryRef,
					PublishedAt:           ptr.To(publishedAt),
					Images: []domainproduct.Image{
						domainproduct.NewImage(
							uuidtestkit.NewTestFromSalt(t, "create_image_1"),
							domainproduct.ImageAttributes{ImagePath: "products/created.png", DisplaySort: 1},
						),
						domainproduct.NewImage(
							uuidtestkit.NewTestFromSalt(t, "create_image_2"),
							domainproduct.ImageAttributes{ImagePath: "products/created-sub.png", DisplaySort: 2},
						),
					},
				}, testCreatedAt)
				require.NoError(t, err)

				require.NoError(t, repo.Create(ctx, entity))

				got, err := repo.FindPublishedByID(ctx, id)
				require.NoError(t, err)
				assert.Equal(t, "作成商品", got.Name())
				require.NotNil(t, got.Description())
				assert.Equal(t, "<p>リッチテキスト説明</p>", *got.Description())
				require.Len(t, got.Images(), 2)
				assert.Equal(t, "products/created.png", got.Images()[0].ImagePath())
				assert.Equal(t, 1, got.Images()[0].DisplaySort())
				assert.Equal(t, "products/created-sub.png", got.Images()[1].ImagePath())
				assert.Equal(t, 2, got.Images()[1].DisplaySort())
				require.NotNil(t, got.PublishedAt())
				assert.True(t, publishedAt.Equal(*got.PublishedAt()))
			})
		})

		t.Run("published_at が nil・画像が空の場合、DB へ NULL と 0 件として登録される", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				id := uuidtestkit.NewTestFromSalt(t, "create_nil_optional_id")
				statusRef, err := domainproduct.NewStatusRef(mustParse(t, statusInStock), "在庫あり")
				require.NoError(t, err)
				categoryRef, err := domainproduct.NewCategoryRef(mustParse(t, categoryElectronics), "電子機器")
				require.NoError(t, err)
				price, err := money.NewPrice(decimal.FromInt(500))
				require.NoError(t, err)
				entity, err := domainproduct.New(id, domainproduct.Attributes{
					Name:     "未公開商品",
					Price:    price,
					Quantity: 0,
					Status:   statusRef,
					Category: categoryRef,
				}, testCreatedAt)
				require.NoError(t, err)

				require.NoError(t, repo.Create(ctx, entity))

				// 公開述語(published_at IS NOT NULL)で除外されるため FindPublishedByID では読み戻せない。
				// 実際に NULL 列として書き込まれたことを生 SQL で直接検証する。
				var publishedAt *time.Time
				err = driver.New(ctx, testDB).
					QueryRow(ctx, "SELECT published_at FROM products WHERE id = $1", id).
					Scan(&publishedAt)
				require.NoError(t, err)
				assert.Nil(t, publishedAt)

				var imageCount int
				err = driver.New(ctx, testDB).
					QueryRow(ctx, "SELECT COUNT(*) FROM product_images WHERE product_id = $1", id).
					Scan(&imageCount)
				require.NoError(t, err)
				assert.Equal(t, 0, imageCount)
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("キャンセル済みコンテキストではErrCanceledへ正規化される", func(t *testing.T) {
			t.Parallel()

			id := uuidtestkit.NewTestFromSalt(t, "create_canceled_id")
			statusRef, err := domainproduct.NewStatusRef(mustParse(t, statusInStock), "在庫あり")
			require.NoError(t, err)
			categoryRef, err := domainproduct.NewCategoryRef(mustParse(t, categoryElectronics), "電子機器")
			require.NoError(t, err)
			price, err := money.NewPrice(decimal.FromInt(100))
			require.NoError(t, err)
			entity, err := domainproduct.New(id, domainproduct.Attributes{
				Name:     "キャンセル商品",
				Price:    price,
				Quantity: 1,
				Status:   statusRef,
				Category: categoryRef,
			}, testCreatedAt)
			require.NoError(t, err)

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			// db.CreateProduct が context.Canceled を返し、pgerror.NormalizeError が ErrCanceled へ正規化する。
			err = repo.Create(ctx, entity)
			require.ErrorIs(t, err, apperror.ErrCanceled)
		})
	})
}

func Test_repository_FindByID(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	lt := observability.NewMockInfraLayerTracer(t)
	txm := testkit.NewTestTransactionRunner(t)

	repo := &repository{tracer: lt, db: testDB}

	unpublishedID := "aaaaaaaa-0000-4000-8000-000000000001"
	versionedID := "aaaaaaaa-0000-4000-8000-000000000002"
	base := time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("非公開(published_atがNULL)の商品も取得できる", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				insertProduct(ctx, t, drv, unpublishedID, probeKeyword+"-UNPUB-ANY", nil, 1999, statusInStock, categoryElectronics, nil)

				got, err := repo.FindByID(ctx, mustParse(t, unpublishedID))
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, mustParse(t, unpublishedID), got.ID())
				assert.Equal(t, probeKeyword+"-UNPUB-ANY", got.Name())
				assert.Nil(t, got.PublishedAt())
			})
		})

		t.Run("DBに永続化されたversionがエンティティに反映される", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				insertProduct(ctx, t, drv, versionedID, probeKeyword+"-VER", nil, 1999, statusInStock, categoryElectronics, ptr.To(base))
				// 初期値(1)との区別が付くよう、DB 側のバージョンだけを進めた行を用意する。
				_, err := drv.Exec(ctx, "UPDATE products SET lock_version = 3 WHERE id = $1", mustParse(t, versionedID))
				require.NoError(t, err)

				got, err := repo.FindByID(ctx, mustParse(t, versionedID))
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, 3, got.Version())
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未存在のIDはNotFoundを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				got, err := repo.FindByID(ctx, uuidtestkit.NewTestFromSalt(t, "find_by_id_missing"))
				assert.Nil(t, got)
				require.ErrorIs(t, err, apperror.ErrNotFound)
			})
		})
	})
}

func Test_repository_Update(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	lt := observability.NewMockInfraLayerTracer(t)
	txm := testkit.NewTestTransactionRunner(t)

	repo := &repository{tracer: lt, db: testDB}

	updatedID := "bbbbbbbb-0000-4000-8000-000000000001"
	conflictID := "bbbbbbbb-0000-4000-8000-000000000002"
	base := time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("更新に成功すると採番後のversionを返しDBの行が更新される", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				insertProduct(ctx, t, drv, updatedID, probeKeyword+"-UPD", nil, 1999, statusInStock, categoryElectronics, ptr.To(base))

				entity, err := repo.FindByID(ctx, mustParse(t, updatedID))
				require.NoError(t, err)
				require.Equal(t, 1, entity.Version())

				statusRef, err := domainproduct.NewStatusRef(mustParse(t, statusOutOfStock), "在庫切れ")
				require.NoError(t, err)
				categoryRef, err := domainproduct.NewCategoryRef(mustParse(t, categoryBooks), "書籍")
				require.NoError(t, err)
				price, err := money.NewPrice(decimal.FromInt(2500))
				require.NoError(t, err)
				publishedAt := base.Add(24 * time.Hour)
				require.NoError(t, entity.Update(domainproduct.Attributes{
					Name:                  "更新後商品",
					Description:           ptr.To("更新後の説明"),
					Price:                 price,
					Quantity:              5,
					StockWarningThreshold: ptr.To(3),
					Status:                statusRef,
					Category:              categoryRef,
					PublishedAt:           ptr.To(publishedAt),
				}))

				version, err := repo.Update(ctx, entity)
				require.NoError(t, err)
				assert.Equal(t, 2, version)

				got, err := repo.FindByID(ctx, mustParse(t, updatedID))
				require.NoError(t, err)
				assert.Equal(t, "更新後商品", got.Name())
				require.NotNil(t, got.Description())
				assert.Equal(t, "更新後の説明", *got.Description())
				assert.Equal(t, "2500", got.Price().String())
				assert.Equal(t, 5, got.Quantity())
				require.NotNil(t, got.StockWarningThreshold())
				assert.Equal(t, 3, *got.StockWarningThreshold())
				assert.Equal(t, mustParse(t, statusOutOfStock), got.Status().ID())
				assert.Equal(t, mustParse(t, categoryBooks), got.Category().ID())
				require.NotNil(t, got.PublishedAt())
				assert.True(t, publishedAt.Equal(*got.PublishedAt()))
				assert.Equal(t, 2, got.Version())
			})
		})

		t.Run("description / stock_warning_threshold / published_at を nil で更新するとDBへNULLとして書き込まれる", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				id := uuidtestkit.NewTestFromSalt(t, "update_clear_nullable_id")
				statusRef, err := domainproduct.NewStatusRef(mustParse(t, statusInStock), "在庫あり")
				require.NoError(t, err)
				categoryRef, err := domainproduct.NewCategoryRef(mustParse(t, categoryElectronics), "電子機器")
				require.NoError(t, err)
				price, err := money.NewPrice(decimal.FromInt(1999))
				require.NoError(t, err)
				entity, err := domainproduct.New(id, domainproduct.Attributes{
					Name:                  "クリア対象商品",
					Description:           ptr.To("<p>クリア前の説明</p>"),
					Price:                 price,
					Quantity:              5,
					StockWarningThreshold: ptr.To(10),
					Status:                statusRef,
					Category:              categoryRef,
					PublishedAt:           ptr.To(base),
				}, testCreatedAt)
				require.NoError(t, err)
				require.NoError(t, repo.Create(ctx, entity))

				loaded, err := repo.FindByID(ctx, id)
				require.NoError(t, err)
				// クリア前が非 NULL であることを確かめないと、最初から NULL だった場合と区別が付かない。
				require.NotNil(t, loaded.Description())
				require.NotNil(t, loaded.StockWarningThreshold())
				require.NotNil(t, loaded.PublishedAt())

				require.NoError(t, loaded.Update(domainproduct.Attributes{
					Name:     loaded.Name(),
					Price:    loaded.Price(),
					Quantity: loaded.Quantity(),
					Status:   loaded.Status(),
					Category: loaded.Category(),
				}))

				version, err := repo.Update(ctx, loaded)
				require.NoError(t, err)
				assert.Equal(t, 2, version)

				// FindByID は公開述語を持たないため、published_at をクリアした後も読み戻せる。
				got, err := repo.FindByID(ctx, id)
				require.NoError(t, err)
				assert.Nil(t, got.Description())
				assert.Nil(t, got.StockWarningThreshold())
				assert.Nil(t, got.PublishedAt())
			})
		})

		t.Run("画像もエンティティが保持する集合へ一致する", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				id := uuidtestkit.NewTestFromSalt(t, "update_images_id")
				require.NoError(t, repo.Create(ctx, newProductWithImages(t, id, []domainproduct.Image{
					newImage(t, "update_images_before", "products/before.png", 1),
				})))

				loaded, err := repo.FindByID(ctx, id)
				require.NoError(t, err)
				require.NoError(t, loaded.Update(domainproduct.Attributes{
					Name:     loaded.Name(),
					Price:    loaded.Price(),
					Quantity: loaded.Quantity(),
					Status:   loaded.Status(),
					Category: loaded.Category(),
					Images: []domainproduct.Image{
						newImage(t, "update_images_after", "products/after.png", 1),
					},
				}))

				version, err := repo.Update(ctx, loaded)
				require.NoError(t, err)
				assert.Equal(t, 2, version)

				got, err := repo.FindByID(ctx, id)
				require.NoError(t, err)
				require.Len(t, got.Images(), 1)
				assert.Equal(t, "products/after.png", got.Images()[0].ImagePath())
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("バージョンが一致しない場合は画像に触れない", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				id := uuidtestkit.NewTestFromSalt(t, "update_images_conflict_id")
				require.NoError(t, repo.Create(ctx, newProductWithImages(t, id, []domainproduct.Image{
					newImage(t, "update_images_conflict", "products/kept.png", 1),
				})))

				stale, err := repo.FindByID(ctx, id)
				require.NoError(t, err)

				drv := driver.New(ctx, testDB)
				_, err = drv.Exec(ctx, "UPDATE products SET lock_version = lock_version + 1 WHERE id = $1", id)
				require.NoError(t, err)

				require.NoError(t, stale.Update(domainproduct.Attributes{
					Name:     stale.Name(),
					Price:    stale.Price(),
					Quantity: stale.Quantity(),
					Status:   stale.Status(),
					Category: stale.Category(),
					Images: []domainproduct.Image{
						newImage(t, "update_images_conflict_new", "products/rejected.png", 1),
					},
				}))

				_, err = repo.Update(ctx, stale)
				require.ErrorIs(t, err, domainproduct.ErrVersionConflict)

				live, deleted := countProductImages(ctx, t, drv, id)
				assert.Equal(t, 1, live)
				assert.Equal(t, 0, deleted)

				got, err := repo.FindByID(ctx, id)
				require.NoError(t, err)
				require.Len(t, got.Images(), 1)
				assert.Equal(t, "products/kept.png", got.Images()[0].ImagePath())
			})
		})

		t.Run("古いversionを持つエンティティの更新はErrVersionConflictを返しDBの行を変更しない", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				insertProduct(ctx, t, drv, conflictID, probeKeyword+"-CONFLICT", nil, 1999, statusInStock, categoryElectronics, ptr.To(base))

				stale, err := repo.FindByID(ctx, mustParse(t, conflictID))
				require.NoError(t, err)

				// 読み込み後に他トランザクションが更新し、DB 側のバージョンだけが進んだ状態を再現する。
				_, err = drv.Exec(ctx,
					"UPDATE products SET name = $2, lock_version = lock_version + 1 WHERE id = $1",
					mustParse(t, conflictID), probeKeyword+"-BY-OTHER",
				)
				require.NoError(t, err)

				require.NoError(t, stale.Update(domainproduct.Attributes{
					Name:                  "衝突する更新",
					Description:           stale.Description(),
					Price:                 stale.Price(),
					Quantity:              stale.Quantity(),
					StockWarningThreshold: stale.StockWarningThreshold(),
					Status:                stale.Status(),
					Category:              stale.Category(),
					PublishedAt:           stale.PublishedAt(),
					Images:                stale.Images(),
				}))

				version, err := repo.Update(ctx, stale)
				require.ErrorIs(t, err, domainproduct.ErrVersionConflict)
				assert.Equal(t, 0, version)

				got, err := repo.FindByID(ctx, mustParse(t, conflictID))
				require.NoError(t, err)
				assert.Equal(t, probeKeyword+"-BY-OTHER", got.Name())
				assert.Equal(t, 2, got.Version())
			})
		})

		t.Run("キャンセル済みコンテキストではErrCanceledへ正規化され衝突とは区別される", func(t *testing.T) {
			t.Parallel()

			id := uuidtestkit.NewTestFromSalt(t, "update_canceled_id")
			statusRef, err := domainproduct.NewStatusRef(mustParse(t, statusInStock), "在庫あり")
			require.NoError(t, err)
			categoryRef, err := domainproduct.NewCategoryRef(mustParse(t, categoryElectronics), "電子機器")
			require.NoError(t, err)
			price, err := money.NewPrice(decimal.FromInt(100))
			require.NoError(t, err)
			entity, err := domainproduct.New(id, domainproduct.Attributes{
				Name:     "キャンセル商品",
				Price:    price,
				Quantity: 1,
				Status:   statusRef,
				Category: categoryRef,
			}, testCreatedAt)
			require.NoError(t, err)

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			version, err := repo.Update(ctx, entity)
			assert.Equal(t, 0, version)
			require.ErrorIs(t, err, apperror.ErrCanceled)
			require.NotErrorIs(t, err, domainproduct.ErrVersionConflict)
		})

		t.Run("versionがINTEGER列に収まらない場合、クエリを発行せずオーバーフローエラーを返す", func(t *testing.T) {
			t.Parallel()

			entity := reconstructWithVersion(t, "update_version_overflow_id", math.MaxInt32+1)

			version, err := repo.Update(context.Background(), entity)
			assert.Equal(t, 0, version)
			require.ErrorIs(t, err, safecast.ErrOverflow)
		})
	})
}

// reconstructWithVersion は、指定バージョンで再構築した最小構成の商品エンティティを返します。
func reconstructWithVersion(t *testing.T, salt string, version int) *domainproduct.Product {
	t.Helper()

	statusRef, err := domainproduct.NewStatusRef(mustParse(t, statusInStock), "在庫あり")
	require.NoError(t, err)
	categoryRef, err := domainproduct.NewCategoryRef(mustParse(t, categoryElectronics), "電子機器")
	require.NoError(t, err)
	price, err := money.NewPrice(decimal.FromInt(100))
	require.NoError(t, err)

	entity, err := domainproduct.Reconstruct(uuidtestkit.NewTestFromSalt(t, salt), domainproduct.Attributes{
		Name:     "バージョン過大商品",
		Price:    price,
		Quantity: 1,
		Status:   statusRef,
		Category: categoryRef,
	}, version, testCreatedAt)
	require.NoError(t, err)
	return entity
}

func Test_repository_LockByID(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	lt := observability.NewMockInfraLayerTracer(t)
	txm := testkit.NewTestTransactionRunner(t)

	repo := &repository{tracer: lt, db: testDB}

	unpublishedID := "cccccccc-0000-4000-8000-000000000001"
	versionedID := "cccccccc-0000-4000-8000-000000000002"
	base := time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("非公開(published_atがNULL)の商品も取得できる", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				insertProduct(ctx, t, drv, unpublishedID, probeKeyword+"-LOCK-UNPUB", nil, 1999, statusInStock, categoryElectronics, nil)

				got, err := repo.LockByID(ctx, mustParse(t, unpublishedID))
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, mustParse(t, unpublishedID), got.ID())
				assert.Equal(t, probeKeyword+"-LOCK-UNPUB", got.Name())
				assert.Nil(t, got.PublishedAt())
				assert.Equal(t, "在庫あり", got.Status().Name())
				assert.Equal(t, "電子機器", got.Category().Name())
			})
		})

		t.Run("DBに永続化されたversionがエンティティに反映される", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				insertProduct(ctx, t, drv, versionedID, probeKeyword+"-LOCK-VER", nil, 1999, statusInStock, categoryElectronics, ptr.To(base))
				// 初期値(1)との区別が付くよう、DB 側のバージョンだけを進めた行を用意する。
				_, err := drv.Exec(ctx, "UPDATE products SET lock_version = 4 WHERE id = $1", mustParse(t, versionedID))
				require.NoError(t, err)

				got, err := repo.LockByID(ctx, mustParse(t, versionedID))
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, 4, got.Version())
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未存在のIDはNotFoundを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				got, err := repo.LockByID(ctx, uuidtestkit.NewTestFromSalt(t, "lock_by_id_missing"))
				assert.Nil(t, got)
				require.ErrorIs(t, err, apperror.ErrNotFound)
			})
		})
	})
}

// assertBatchProductFetchContract は、ID 集合でまとめて取得するメソッドが共通して満たす契約を検証します。
// LockByIDs と FindByIDs は行を排他ロックするかどうかだけが異なり、返す内容の契約は同一のため、
// 表明を 1 箇所へ集めて片方だけが緩む状態を作らないようにします。
// ロックの有無そのものは、この契約では表現できないため各メソッド側のテストが担います。
func assertBatchProductFetchContract(
	t *testing.T, testDB driver.DatabaseDriver, txm testkit.TransactionRunner,
	idPrefix, nameTag string, fetch batchProductFetch,
) {
	t.Helper()

	lowID := idPrefix + "000000000001"
	highID := idPrefix + "000000000002"
	base := time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("複数IDをまとめてID昇順で取得し非公開の商品も含める", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				// 挿入順を id 降順にし、戻り値が挿入順ではなく id 昇順であることを検証する。
				insertProduct(ctx, t, drv, highID, probeKeyword+"-"+nameTag+"-PUB", nil, 1999, statusInStock, categoryElectronics, ptr.To(base))
				insertProduct(ctx, t, drv, lowID, probeKeyword+"-"+nameTag+"-UNPUB", nil, 2999, statusInStock, categoryElectronics, nil)

				got, err := fetch(ctx, []uuid.UUID{mustParse(t, highID), mustParse(t, lowID)})
				require.NoError(t, err)
				require.Len(t, got, 2)
				assert.Equal(t, mustParse(t, lowID), got[0].ID())
				assert.Equal(t, mustParse(t, highID), got[1].ID())
				assert.Nil(t, got[0].PublishedAt())
				assert.Equal(t, "在庫あり", got[0].Status().Name())
				assert.Equal(t, "電子機器", got[0].Category().Name())
			})
		})

		t.Run("未存在のIDは結果から除外され件数が引数より少なくなる", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				existing := idPrefix + "000000000003"
				insertProduct(ctx, t, drv, existing, probeKeyword+"-"+nameTag+"-PARTIAL", nil, 1999, statusInStock, categoryElectronics, ptr.To(base))

				ids := []uuid.UUID{mustParse(t, existing), uuidtestkit.NewTestFromSalt(t, nameTag+"_missing")}
				got, err := fetch(ctx, ids)
				require.NoError(t, err)
				require.Len(t, got, 1)
				assert.Equal(t, mustParse(t, existing), got[0].ID())
			})
		})

		t.Run("空のID集合は空の結果を返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				got, err := fetch(ctx, []uuid.UUID{})
				require.NoError(t, err)
				assert.Empty(t, got)
			})
		})

		t.Run("サブセント単価をNUMERIC精度を保ったまま取得する", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				subcentID := idPrefix + "0000000000aa"
				// price=19.995（サブセント）を NUMERIC 列へ直接挿入し、価格スケールの往復を検証する。
				_, err := drv.Exec(ctx,
					"INSERT INTO products "+
						"(id, name, description, price, quantity, stock_warning_threshold, status_id, category_id, published_at) "+
						"VALUES ($1,$2,$3,$4::numeric,$5,$6,$7,$8,$9)",
					subcentID, probeKeyword+"-"+nameTag+"-SUBCENT", nil, "19.995", 10, nil, statusInStock, categoryElectronics, base,
				)
				require.NoError(t, err)

				got, err := fetch(ctx, []uuid.UUID{mustParse(t, subcentID)})
				require.NoError(t, err)
				require.Len(t, got, 1)
				assert.Equal(t, "19.995", got[0].Price().String())
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("負値の価格は再構築不能としてErrInternalへ正規化する", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				negativeID := idPrefix + "0000000000bb"
				// NUMERIC 列は負値を格納できるが、money.Price は非負不変条件を持つ。
				_, err := drv.Exec(ctx,
					"INSERT INTO products "+
						"(id, name, description, price, quantity, stock_warning_threshold, status_id, category_id, published_at) "+
						"VALUES ($1,$2,$3,$4::numeric,$5,$6,$7,$8,$9)",
					negativeID, probeKeyword+"-"+nameTag+"-NEG", nil, "-1", 10, nil, statusInStock, categoryElectronics, base,
				)
				require.NoError(t, err)

				_, err = fetch(ctx, []uuid.UUID{mustParse(t, negativeID)})
				require.ErrorIs(t, err, apperror.ErrInternal)
			})
		})

		t.Run("キャンセル済みコンテキストではErrCanceledへ正規化される", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			actual, err := fetch(ctx, []uuid.UUID{mustParse(t, lowID)})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, apperror.ErrCanceled)
		})
	})
}

func Test_repository_LockByIDs(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	repo := &repository{tracer: observability.NewMockInfraLayerTracer(t), db: testDB}

	assertBatchProductFetchContract(
		t, testDB, testkit.NewTestTransactionRunner(t),
		"cccccccc-0000-4000-8001-", "LOCKS", repo.LockByIDs,
	)
}

func Test_repository_UpdateStock(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	lt := observability.NewMockInfraLayerTracer(t)
	txm := testkit.NewTestTransactionRunner(t)

	repo := &repository{tracer: lt, db: testDB}

	adjustedID := "dddddddd-0000-4000-8000-000000000001"
	conflictID := "dddddddd-0000-4000-8000-000000000002"
	untouchedID := "dddddddd-0000-4000-8000-000000000003"
	base := time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("在庫を更新すると採番後のversionを返しDBの行が更新される", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				insertProduct(ctx, t, drv, adjustedID, probeKeyword+"-STOCK", nil, 1999, statusInStock, categoryElectronics, ptr.To(base))

				entity, err := repo.LockByID(ctx, mustParse(t, adjustedID))
				require.NoError(t, err)
				before := entity.Quantity()
				require.Equal(t, 1, entity.Version())
				require.NoError(t, entity.AdjustStock(7))

				version, err := repo.UpdateStock(ctx, entity)
				require.NoError(t, err)
				assert.Equal(t, 2, version)

				got, err := repo.FindByID(ctx, mustParse(t, adjustedID))
				require.NoError(t, err)
				assert.Equal(t, before+7, got.Quantity())
				assert.Equal(t, 2, got.Version())
			})
		})

		t.Run("在庫以外の列は更新されない", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				insertProduct(
					ctx,
					t,
					drv,
					untouchedID,
					probeKeyword+"-STOCK-KEEP",
					ptr.To("据え置きの説明"),
					1999,
					statusInStock,
					categoryElectronics,
					ptr.To(base),
				)

				entity, err := repo.LockByID(ctx, mustParse(t, untouchedID))
				require.NoError(t, err)
				require.NoError(t, entity.AdjustStock(1))
				_, err = repo.UpdateStock(ctx, entity)
				require.NoError(t, err)

				got, err := repo.FindByID(ctx, mustParse(t, untouchedID))
				require.NoError(t, err)
				assert.Equal(t, probeKeyword+"-STOCK-KEEP", got.Name())
				require.NotNil(t, got.Description())
				assert.Equal(t, "据え置きの説明", *got.Description())
				assert.Equal(t, "1999", got.Price().String())
				assert.Equal(t, mustParse(t, statusInStock), got.Status().ID())
				require.NotNil(t, got.PublishedAt())
				assert.True(t, base.Equal(*got.PublishedAt()))
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("古いversionを持つエンティティの在庫更新はErrVersionConflictを返しDBの行を変更しない", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				insertProduct(ctx, t, drv, conflictID, probeKeyword+"-STOCK-CONFLICT", nil, 1999, statusInStock, categoryElectronics, ptr.To(base))

				stale, err := repo.FindByID(ctx, mustParse(t, conflictID))
				require.NoError(t, err)
				require.NoError(t, stale.AdjustStock(3))

				// 取得後に他トランザクションが更新し、DB 側のバージョンだけが進んだ状態を再現する。
				_, err = drv.Exec(ctx,
					"UPDATE products SET quantity = quantity + 100, lock_version = lock_version + 1 WHERE id = $1",
					mustParse(t, conflictID),
				)
				require.NoError(t, err)

				version, err := repo.UpdateStock(ctx, stale)
				require.ErrorIs(t, err, domainproduct.ErrVersionConflict)
				assert.Equal(t, 0, version)

				got, err := repo.FindByID(ctx, mustParse(t, conflictID))
				require.NoError(t, err)
				assert.Equal(t, 110, got.Quantity())
				assert.Equal(t, 2, got.Version())
			})
		})

		t.Run("キャンセル済みコンテキストではErrCanceledへ正規化され衝突とは区別される", func(t *testing.T) {
			t.Parallel()

			id := uuidtestkit.NewTestFromSalt(t, "update_stock_canceled_id")
			statusRef, err := domainproduct.NewStatusRef(mustParse(t, statusInStock), "在庫あり")
			require.NoError(t, err)
			categoryRef, err := domainproduct.NewCategoryRef(mustParse(t, categoryElectronics), "電子機器")
			require.NoError(t, err)
			price, err := money.NewPrice(decimal.FromInt(100))
			require.NoError(t, err)
			entity, err := domainproduct.New(id, domainproduct.Attributes{
				Name:     "キャンセル商品",
				Price:    price,
				Quantity: 1,
				Status:   statusRef,
				Category: categoryRef,
			}, testCreatedAt)
			require.NoError(t, err)

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			version, err := repo.UpdateStock(ctx, entity)
			assert.Equal(t, 0, version)
			require.ErrorIs(t, err, apperror.ErrCanceled)
			require.NotErrorIs(t, err, domainproduct.ErrVersionConflict)
		})

		t.Run("versionがINTEGER列に収まらない場合、クエリを発行せずオーバーフローエラーを返す", func(t *testing.T) {
			t.Parallel()

			entity := reconstructWithVersion(t, "update_stock_version_overflow_id", math.MaxInt32+1)

			version, err := repo.UpdateStock(context.Background(), entity)
			assert.Equal(t, 0, version)
			require.ErrorIs(t, err, safecast.ErrOverflow)
		})
	})
}

func Test_repository_UpdateStock_concurrentRowLock(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	// 検証用商品の作成から 2 本の tx の完了までを、他パッケージの CASCADE TRUNCATE から守る。
	testkit.HoldSuiteSerialization(t, testDB)

	lt := observability.NewMockInfraLayerTracer(t)
	repo := &repository{tracer: lt, db: testDB}

	dbCfg := config.NewDatabaseConfig(config.MockConfigForTest(t))
	txm := driver.NewTransactionManager(testDB, dbCfg, logging.NewTestLogger(t), system.NewSleeper())

	// commit する専用行のため、rollback 前提の他テストが使う ID・keyword とは重ならない値にする。
	productID := mustParse(t, "9a592000-0000-4000-8000-000000000001")

	t.Cleanup(func() {
		_ = txm.Do(context.Background(), func(ctx context.Context) error {
			_, err := driver.New(ctx, testDB).Exec(ctx, "DELETE FROM products WHERE id = $1", productID)
			return err
		})
	})

	// 両 tx から見える commit 済みの在庫 10・バージョン 1 の行を用意する。
	require.NoError(t, txm.Do(context.Background(), func(ctx context.Context) error {
		_, err := driver.New(ctx, testDB).Exec(ctx, "DELETE FROM products WHERE id = $1", productID)
		require.NoError(t, err)
		insertProduct(ctx, t, driver.New(ctx, testDB), productID.String(), "STOCKCONCURRENT592",
			nil, 1999, statusInStock, categoryElectronics, nil)
		return nil
	}))

	holderLocked := make(chan struct{})
	release := make(chan struct{})
	holderDone := make(chan error, 1)

	var once sync.Once
	rel := func() { once.Do(func() { close(release) }) }
	t.Cleanup(rel) // 失敗時も保持側 goroutine をリークさせない

	go func() {
		holderDone <- txm.Do(context.Background(), func(ctx context.Context) error {
			entity, err := repo.LockByID(ctx, productID)
			if err != nil {
				return err
			}
			close(holderLocked)
			<-release
			if err = entity.AdjustStock(5); err != nil {
				return err
			}
			_, err = repo.UpdateStock(ctx, entity)
			return err
		})
	}()

	select {
	case <-holderLocked:
	case err := <-holderDone:
		require.NoError(t, err, "保持側 tx がロック取得前に失敗した")
		return
	}

	timeoutErr := txm.Do(context.Background(), func(ctx context.Context) error {
		if _, err := driver.New(ctx, testDB).Exec(ctx, "SET LOCAL lock_timeout = '50ms'"); err != nil {
			return err
		}
		_, err := repo.LockByID(ctx, productID)
		return err
	})
	require.ErrorIs(t, timeoutErr, apperror.ErrUnavailable, "ロック待ちのタイムアウト(55P03)は待てば解消しうる一時障害として扱う")
	assert.True(t, pgerror.IsLockNotAvailable(timeoutErr), "ErrUnavailable は 40001 / 40P01 / 57014 とも共有するため、待ち合わせが 55P03 で失敗したことまで固定する")
	require.NotErrorIs(t, timeoutErr, apperror.ErrInternal, "サーバ内部エラー(500)として露出させない")

	contenderDone := make(chan error, 1)
	go func() {
		contenderDone <- txm.Do(context.Background(), func(ctx context.Context) error {
			entity, err := repo.LockByID(ctx, productID)
			if err != nil {
				return err
			}
			assert.Equal(t, 15, entity.Quantity(), "後続 tx は先行 tx の更新後の在庫を読む")
			if err = entity.AdjustStock(-3); err != nil {
				return err
			}
			_, err = repo.UpdateStock(ctx, entity)
			return err
		})
	}()

	rel()
	require.NoError(t, <-holderDone)
	require.NoError(t, <-contenderDone)

	got, err := repo.FindByID(context.Background(), productID)
	require.NoError(t, err)
	assert.Equal(t, 12, got.Quantity(), "両 tx の増減が失われず合成される")
	assert.Equal(t, 3, got.Version(), "在庫更新のたびにバージョンが進む")
}

func Test_toProductRows(t *testing.T) {
	t.Parallel()

	newRow := func(t *testing.T, salt string) productRow {
		t.Helper()
		return productRow{
			p: gen.Products{
				ID:          uuidtestkit.NewTestFromSalt(t, salt),
				Name:        "商品",
				Description: ptr.To("説明"),
				Price:       decimal.FromInt(1999),
				Quantity:    100,
				StatusID:    uuidtestkit.NewTestFromSalt(t, salt+"_status"),
				CategoryID:  uuidtestkit.NewTestFromSalt(t, salt+"_category"),
				LockVersion: 3,
				CreatedAt:   testCreatedAt,
			},
			statusName:   "在庫あり",
			categoryName: "電子機器",
		}
	}
	identity := func(r productRow) productRow { return r }

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("行スライスを順序を保って共通の変換元へ揃える", func(t *testing.T) {
			t.Parallel()

			r1 := newRow(t, "rows_a")
			r2 := newRow(t, "rows_b")

			got := toProductRows([]productRow{r1, r2}, identity)

			require.Len(t, got, 2)
			assert.Equal(t, r1.p.ID, got[0].p.ID)
			assert.Equal(t, r2.p.ID, got[1].p.ID)
		})

		t.Run("空スライスは空の列を返す", func(t *testing.T) {
			t.Parallel()

			assert.Empty(t, toProductRows([]productRow{}, identity))
		})
	})
}

func Test_repository_Count(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	txm := testkit.NewTestTransactionRunner(t)
	repo := &repository{db: testDB, tracer: observability.NewMockInfraLayerTracer(t)}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("公開商品は総数と公開数の双方を増やし非公開は総数のみを増やす", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				before, err := repo.Count(ctx)
				require.NoError(t, err)

				publishedAt := time.Now()
				insertProduct(ctx, t, drv, "e1000000-0000-4000-8000-000000000001", "集計対象の公開商品",
					nil, 100, statusInStock, categoryElectronics, &publishedAt)
				insertProduct(ctx, t, drv, "e1000000-0000-4000-8000-000000000002", "集計対象の非公開商品",
					nil, 100, statusInStock, categoryElectronics, nil)

				after, err := repo.Count(ctx)
				require.NoError(t, err)

				assert.Equal(t, before.Total+2, after.Total)
				assert.Equal(t, before.Published+1, after.Published)
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("キャンセル済みコンテキストではErrCanceledへ正規化して返す", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			got, err := repo.Count(ctx)
			require.ErrorIs(t, err, apperror.ErrCanceled)
			assert.Zero(t, got.Total)
		})
	})
}

// insertStockProduct は、在庫数・在庫警告閾値・公開日時を明示した商品を挿入するヘルパーです。
func insertStockProduct(
	ctx context.Context, t *testing.T, db driver.DBTX,
	id, name string, quantity int, threshold *int, publishedAt *time.Time,
) {
	t.Helper()
	_, err := db.Exec(ctx,
		"INSERT INTO products "+
			"(id, name, description, price, quantity, stock_warning_threshold, status_id, category_id, published_at) "+
			"VALUES ($1,$2,NULL,$3,$4,$5,$6,$7,$8)",
		id, name, 1000, quantity, threshold, statusInStock, categoryElectronics, publishedAt,
	)
	require.NoError(t, err)
}

// insertNegativePriceLowStockProduct は、在庫僅少の条件を満たしつつ価格が負数の行を挿入するヘルパーです。
func insertNegativePriceLowStockProduct(ctx context.Context, t *testing.T, db driver.DBTX, id string) {
	t.Helper()
	_, err := db.Exec(ctx,
		"INSERT INTO products "+
			"(id, name, description, price, quantity, stock_warning_threshold, status_id, category_id, published_at) "+
			"VALUES ($1,$2,NULL,$3,$4,$5,$6,$7,NULL)",
		id, "在庫僅少-価格不正", -1, 0, 100, statusInStock, categoryElectronics,
	)
	require.NoError(t, err)
}

func Test_repository_FindAllLowStock(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	txm := testkit.NewTestTransactionRunner(t)
	repo := &repository{db: testDB, tracer: observability.NewMockInfraLayerTracer(t)}

	// seed 済み商品にも在庫僅少行が含まれるため、検証は挿入した probe 行に絞り込んで行う。
	const (
		below       = "ffffffff-0000-4000-8000-000000000001" // quantity(50) < threshold(100)
		unpublished = "ffffffff-0000-4000-8000-000000000002" // quantity(55) < threshold(100) / 未公開
		tieLow      = "ffffffff-0000-4000-8000-000000000003" // quantity(60) / id 小
		tieHigh     = "ffffffff-0000-4000-8000-000000000004" // quantity(60) / id 大
		equal       = "ffffffff-0000-4000-8000-000000000005" // quantity(100) == threshold(100)
		above       = "ffffffff-0000-4000-8000-000000000006" // quantity(101) > threshold(100)
		noThreshold = "ffffffff-0000-4000-8000-000000000007" // threshold 未設定 / 在庫 0
	)
	// probeLimit は、挿入した probe 行が seed 行に押し出されない十分な取得件数です。
	const probeLimit = 1000

	publishedAt := time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)

	insertProbeSet := func(ctx context.Context, t *testing.T, drv driver.DBTX) {
		t.Helper()
		insertStockProduct(ctx, t, drv, below, "在庫僅少-閾値未満", 50, ptr.To(100), &publishedAt)
		insertStockProduct(ctx, t, drv, unpublished, "在庫僅少-未公開", 55, ptr.To(100), nil)
		insertStockProduct(ctx, t, drv, tieLow, "在庫僅少-同数A", 60, ptr.To(100), &publishedAt)
		insertStockProduct(ctx, t, drv, tieHigh, "在庫僅少-同数B", 60, ptr.To(100), &publishedAt)
		insertStockProduct(ctx, t, drv, equal, "在庫僅少-閾値ちょうど", 100, ptr.To(100), &publishedAt)
		insertStockProduct(ctx, t, drv, above, "在庫十分-閾値超過", 101, ptr.To(100), &publishedAt)
		insertStockProduct(ctx, t, drv, noThreshold, "閾値未設定-在庫ゼロ", 0, nil, &publishedAt)
	}

	// probeIDs は、取得結果から probe 行だけを挿入順ではなく取得順で抜き出します。
	probeIDs := func(t *testing.T, products domainproduct.Products) []uuid.UUID {
		t.Helper()
		probes := map[uuid.UUID]struct{}{
			mustParse(t, below): {}, mustParse(t, unpublished): {}, mustParse(t, tieLow): {},
			mustParse(t, tieHigh): {}, mustParse(t, equal): {}, mustParse(t, above): {},
			mustParse(t, noThreshold): {},
		}
		var ids []uuid.UUID
		for _, p := range products {
			if _, ok := probes[p.ID()]; ok {
				ids = append(ids, p.ID())
			}
		}
		return ids
	}

	// findProbe は、取得結果から指定 ID の probe 行を 1 件取り出します。
	findProbe := func(t *testing.T, products domainproduct.Products, id string) *domainproduct.Product {
		t.Helper()
		for _, p := range products {
			if p.ID() == mustParse(t, id) {
				return p
			}
		}
		return nil
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("閾値以下の商品のみを在庫昇順同数はID昇順で返し閾値超過と閾値未設定は返さない", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				insertProbeSet(ctx, t, drv)

				actual, err := repo.FindAllLowStock(ctx, probeLimit)
				require.NoError(t, err)

				assert.Equal(t, []uuid.UUID{
					mustParse(t, below),
					mustParse(t, unpublished),
					mustParse(t, tieLow),
					mustParse(t, tieHigh),
					mustParse(t, equal),
				}, probeIDs(t, actual))
			})
		})

		t.Run("未公開商品も在庫僅少であれば返る", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				insertProbeSet(ctx, t, drv)

				actual, err := repo.FindAllLowStock(ctx, probeLimit)
				require.NoError(t, err)

				found := findProbe(t, actual, unpublished)
				require.NotNil(t, found)
				assert.Nil(t, found.PublishedAt())
			})
		})

		t.Run("ステータス名とカテゴリ名を結合して復元する", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				insertProbeSet(ctx, t, drv)

				actual, err := repo.FindAllLowStock(ctx, probeLimit)
				require.NoError(t, err)

				found := findProbe(t, actual, below)
				require.NotNil(t, found)
				// SQL の結合エイリアス取り違え（status / category の名称入れ替わり）を検出するため、異なる 2 値で検証する。
				assert.Equal(t, mustParse(t, statusInStock), found.Status().ID())
				assert.Equal(t, "在庫あり", found.Status().Name())
				assert.Equal(t, mustParse(t, categoryElectronics), found.Category().ID())
				assert.Equal(t, "電子機器", found.Category().Name())
			})
		})

		t.Run("limitで取得件数が打ち切られる", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				insertProbeSet(ctx, t, drv)

				actual, err := repo.FindAllLowStock(ctx, 3)
				require.NoError(t, err)
				assert.Len(t, actual, 3)
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("キャンセル済みコンテキストではErrCanceledへ正規化して返す", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			actual, err := repo.FindAllLowStock(ctx, 10)
			require.ErrorIs(t, err, apperror.ErrCanceled)
			assert.Nil(t, actual)
		})

		t.Run("不正な行(価格が負数)が含まれるとデータ不整合としてErrInternalを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				insertNegativePriceLowStockProduct(ctx, t, drv, "ffffffff-0000-4000-8000-0000000000e1")

				actual, err := repo.FindAllLowStock(ctx, probeLimit)
				assert.Nil(t, actual)
				require.ErrorIs(t, err, apperror.ErrInternal)
				require.NotErrorIs(t, err, money.ErrNegativePrice)
			})
		})
	})
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("渡したドライバとinfra層トレーサーを保持した実装を返す", func(t *testing.T) {
			t.Parallel()

			testDB := testkit.NewTestDB(t)
			tf := observability.NewNoopTracerFactory(t)

			actual := New(testDB, tf)

			assert.Equal(t, &repository{db: testDB, tracer: tf.Infra()}, actual)
		})
	})
}

func Test_repository_FindByIDs(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	repo := &repository{tracer: observability.NewMockInfraLayerTracer(t), db: testDB}

	// ロックを取らないことは同一 tx では観測できないため、Test_findByIDsDoesNotWaitForRowLocks が担う。
	assertBatchProductFetchContract(
		t, testDB, testkit.NewTestTransactionRunner(t),
		"cccccccc-0000-4000-8002-", "FINDS", repo.FindByIDs,
	)
}

func Test_nilIfEmpty(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("値を持つスライスはそのまま返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, []int16{1, 2}, nilIfEmpty([]int16{1, 2}))
		})

		t.Run("非nilの空スライスはnilへ揃える", func(t *testing.T) {
			t.Parallel()

			assert.Nil(t, nilIfEmpty([]int16{}))
		})

		t.Run("nilはnilのまま返す", func(t *testing.T) {
			t.Parallel()

			assert.Nil(t, nilIfEmpty[int16](nil))
		})
	})
}

func Test_repository_FindAllList(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	lt := observability.NewMockInfraLayerTracer(t)
	txm := testkit.NewTestTransactionRunner(t)

	repo := &repository{tracer: lt, db: testDB}

	// seed より確実に新しい固定時刻。id の大小でタイブレークを検証するため tie ペアを含む。
	base := time.Date(2099, time.February, 1, 0, 0, 0, 0, time.UTC)
	tieHigh := "eeeeeeee-1111-4000-8000-000000000002"
	tieLow := "eeeeeeee-1111-4000-8000-000000000001"
	mid := "eeeeeeee-1111-4000-8000-000000000003"    // base-1h・未公開
	oldest := "eeeeeeee-1111-4000-8000-000000000004" // base-2h
	allKeyword := "findalllist-probe"

	insertProbeSet := func(ctx context.Context, t *testing.T, drv driver.DBTX) {
		t.Helper()
		insertProductAt(ctx, t, drv, tieHigh, allKeyword+"-A", ptr.To(base), base)
		insertProductAt(ctx, t, drv, tieLow, allKeyword+"-B", nil, base)
		insertProductAt(ctx, t, drv, mid, allKeyword+"-C", nil, base.Add(-time.Hour))
		insertProductAt(ctx, t, drv, oldest, allKeyword+"-D", ptr.To(base), base.Add(-2*time.Hour))
	}

	probeFilter := func(t *testing.T) domainproduct.SearchFilter {
		t.Helper()
		price := mustPriceFilter(t)
		quantity := int32(10)
		return domainproduct.SearchFilter{
			Keyword:     ptr.To(allKeyword),
			MinPrice:    &price,
			MaxPrice:    &price,
			MinQuantity: &quantity,
			MaxQuantity: &quantity,
		}
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("降順で先頭ページとafter境界がkeyset安定順に次ページを返し未公開行も含まれる", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				insertProbeSet(ctx, t, drv)

				// 先頭ページ(limit=2): 登録日時降順・id降順で tieHigh, tieLow。
				firstPage, err := repo.FindAllList(ctx, domainproduct.AllListParams{
					SearchFilter: probeFilter(t),
					Limit:        2,
					Ascending:    false,
				})
				require.NoError(t, err)
				require.Len(t, firstPage, 2)
				assert.Equal(t, mustParse(t, tieHigh), firstPage[0].ID())
				assert.Equal(t, mustParse(t, tieLow), firstPage[1].ID())
				// 2 件目は published_at=NULL であり、公開済み限定の一覧では現れない行。
				assert.Nil(t, firstPage[1].PublishedAt())

				// 次ページ: 直前ページ末尾 (created_at, id) を境界に mid, oldest。
				afterCreatedAt := firstPage[1].CreatedAt()
				afterID := firstPage[1].ID()
				nextPage, err := repo.FindAllList(ctx, domainproduct.AllListParams{
					SearchFilter:   probeFilter(t),
					Limit:          2,
					Ascending:      false,
					AfterCreatedAt: &afterCreatedAt,
					AfterID:        &afterID,
				})
				require.NoError(t, err)
				require.Len(t, nextPage, 2)
				assert.Equal(t, mustParse(t, mid), nextPage[0].ID())
				assert.Equal(t, mustParse(t, oldest), nextPage[1].ID())
			})
		})

		t.Run("昇順は降順の逆順で同じ母集団を返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				insertProbeSet(ctx, t, drv)

				asc, err := repo.FindAllList(ctx, domainproduct.AllListParams{
					SearchFilter: probeFilter(t),
					Limit:        4,
					Ascending:    true,
				})
				require.NoError(t, err)
				require.Len(t, asc, 4)
				assert.Equal(t, mustParse(t, oldest), asc[0].ID())
				assert.Equal(t, mustParse(t, mid), asc[1].ID())
				assert.Equal(t, mustParse(t, tieLow), asc[2].ID())
				assert.Equal(t, mustParse(t, tieHigh), asc[3].ID())
			})
		})

		t.Run("絞り込み条件に一致しない行は返さない", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				insertProbeSet(ctx, t, drv)

				filter := probeFilter(t)
				filter.Keyword = ptr.To(allKeyword + "-C")
				actual, err := repo.FindAllList(ctx, domainproduct.AllListParams{
					SearchFilter: filter,
					Limit:        10,
					Ascending:    false,
				})
				require.NoError(t, err)
				require.Len(t, actual, 1)
				assert.Equal(t, mustParse(t, mid), actual[0].ID())
			})
		})
	})
}

func Test_repository_CountAll(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	lt := observability.NewMockInfraLayerTracer(t)
	txm := testkit.NewTestTransactionRunner(t)

	repo := &repository{tracer: lt, db: testDB}

	base := time.Date(2099, time.March, 1, 0, 0, 0, 0, time.UTC)
	countKeyword := "countall-probe"

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未公開を含めて数え、公開済み限定の件数より多くなる", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				insertProductAt(ctx, t, drv, "eeeeeeee-2222-4000-8000-000000000001",
					countKeyword+"-A", ptr.To(base), base)
				insertProductAt(ctx, t, drv, "eeeeeeee-2222-4000-8000-000000000002",
					countKeyword+"-B", nil, base)

				price := mustPriceFilter(t)
				quantity := int32(10)
				filter := domainproduct.SearchFilter{
					Keyword:     ptr.To(countKeyword),
					MinPrice:    &price,
					MaxPrice:    &price,
					MinQuantity: &quantity,
					MaxQuantity: &quantity,
				}

				all, err := repo.CountAll(ctx, filter)
				require.NoError(t, err)
				assert.Equal(t, int64(2), all)

				published, err := repo.CountPublished(ctx, filter)
				require.NoError(t, err)
				assert.Equal(t, int64(1), published)
			})
		})

		t.Run("一致する行が無い場合は0を返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				actual, err := repo.CountAll(ctx, domainproduct.SearchFilter{
					Keyword: ptr.To("countall-no-such-keyword"),
				})
				require.NoError(t, err)
				assert.Equal(t, int64(0), actual)
			})
		})
	})
}
