package idempotency

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/controller/ctxhelper"
	"go-boilerplate/internal/usecase/boundary/auth"
	clocktest "go-boilerplate/internal/usecase/boundary/clock/testkit"
	idempotencybndry "go-boilerplate/internal/usecase/boundary/idempotency"
	mock_idempotency "go-boilerplate/internal/usecase/boundary/idempotency/mock"
	mock_tx "go-boilerplate/internal/usecase/boundary/tx/mock"
	idempotencyuc "go-boilerplate/internal/usecase/idempotency"
	mock_idempotencyuc "go-boilerplate/internal/usecase/idempotency/mock"
	"go-boilerplate/pkg/xerrors"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	fingerprintLen = 32
	// testPath は、ミドルウェア検証用の POST パスです。spy ハンドラに渡す任意値です。
	testPath = "/v1/resources"
	// sentinel は、後段ハンドラが呼ばれたことを示す戻り値です。
	sentinel = "SENTINEL"
	// testSubject は、冪等性スコープになる認証主体です（ADR-0063 (idempotency-scope-required)）。
	testSubject = "user-john-doe"
	// testOperationID は、operationID の伝播を追うための任意値です。
	testOperationID = "PostResources"
)

type spyRequest struct {
	Name string `json:"name"`
}

// strictHandlerFunc は、oapi-codegen 生成の gen.StrictHandlerFunc と同型のテスト用型です。
type strictHandlerFunc func(ec *echo.Context, request any) (any, error)

// newEcho は、テスト用の *echo.Context（POST testPath）を生成します。key 非空ならヘッダを付与し、
// withAuthn なら subject を持つ Authn を ctx に仕込みます。内部 UserID は解決しません
// （冪等性スコープは認証主体そのもので、内部ユーザーへの解決を要求しないため）。
func newEcho(t *testing.T, key string, withAuthn bool, subject string) *echo.Context {
	t.Helper()

	ctx := context.Background()
	if withAuthn {
		ctx = ctxhelper.WithAuthn(ctx)
		a, err := auth.New(subject, "test", nil, nil)
		require.NoError(t, err)
		ctxhelper.SetAuthn(ctx, *a)
	}
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, testPath, nil)
	if key != "" {
		req.Header.Set(headerName, key)
	}
	return echo.New().NewContext(req, httptest.NewRecorder())
}

func TestStrictMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ヘッダ無しは素通しし後段をそのまま呼ぶ", func(t *testing.T) {
			t.Parallel()

			called := false
			h := strictHandlerFunc(func(*echo.Context, any) (any, error) {
				called = true
				return sentinel, nil
			})
			ec := newEcho(t, "", true, "user-1")

			res, err := StrictMiddleware[strictHandlerFunc]()(h, "PostUsers")(ec, spyRequest{})

			require.NoError(t, err)
			assert.True(t, called)
			assert.Equal(t, sentinel, res)
		})
	})
}

func Test_handle(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ヘッダ無しは素通しし後段をそのまま呼ぶ", func(t *testing.T) {
			t.Parallel()
			called := false
			next := NextFunc(func(*echo.Context, any) (any, error) {
				called = true
				return sentinel, nil
			})
			ec := newEcho(t, "", true, "user-1")

			res, err := Middleware()(next, "PostUsers")(ec, spyRequest{})

			require.NoError(t, err)
			assert.True(t, called)
			assert.Equal(t, sentinel, res)
		})

		t.Run("空白のみのキーは素通しし後段をそのまま呼ぶ", func(t *testing.T) {
			t.Parallel()
			called := false
			next := NextFunc(func(*echo.Context, any) (any, error) {
				called = true
				return sentinel, nil
			})
			// 空白のみのキーは handle 内の TrimSpace で空になり、ヘッダ無し相当として素通しされる
			// （validateKey は呼ばれない）。
			ec := newEcho(t, "   ", true, "user-1")

			res, err := Middleware()(next, "PostUsers")(ec, spyRequest{})

			require.NoError(t, err)
			assert.True(t, called)
			assert.Equal(t, sentinel, res)
		})

		t.Run("認証が無ければ冪等性は発動せず素通しする", func(t *testing.T) {
			t.Parallel()
			called := false
			next := NextFunc(func(*echo.Context, any) (any, error) {
				called = true
				return sentinel, nil
			})
			ec := newEcho(t, "key-1", false, "")

			res, err := Middleware()(next, "PostUsers")(ec, spyRequest{})

			require.NoError(t, err)
			assert.True(t, called)
			assert.Equal(t, sentinel, res)
		})

		t.Run("内部UserIDが未解決でも認証済みなら冪等性が発動する", func(t *testing.T) {
			t.Parallel()
			called := false
			next := NextFunc(func(*echo.Context, any) (any, error) {
				called = true
				return sentinel, nil
			})
			const unresolved = "user-not-yet-registered"
			ec := newEcho(t, "key-unresolved", true, unresolved)

			res, err := Middleware()(next, "PostUsers")(ec, spyRequest{})
			require.NoError(t, err)
			require.True(t, called)
			assert.Equal(t, sentinel, res, "後段の戻り値がそのまま透過されること")

			ctrl := gomock.NewController(t)
			store := mock_idempotency.NewMockStore(ctrl)
			txm := mock_tx.NewMockManager(ctrl)
			clk := clocktest.NewMockClock(t, time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))
			txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) },
			)

			var got idempotencybndry.ClaimParams
			store.EXPECT().Claim(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, p idempotencybndry.ClaimParams) (bool, error) {
					got = p
					return true, nil
				},
			)
			store.EXPECT().Complete(gomock.Any(), gomock.Any()).Return(nil)

			deps := idempotencyuc.Deps{Txm: txm, Store: store, Clock: clk}
			_, _, runErr := idempotencyuc.Run(ec.Request().Context(), deps, 201,
				func(context.Context) (string, error) { return "ok", nil })
			require.NoError(t, runErr)

			assert.Equal(t, unresolved, got.Scope, "内部 UserID の解決を待たずにスコープが定まること")
		})

		t.Run("有効キーと認証があれば Request を ctx に載せて後段へ渡す", func(t *testing.T) {
			t.Parallel()
			called := false
			next := NextFunc(func(*echo.Context, any) (any, error) {
				called = true
				return sentinel, nil
			})
			ec := newEcho(t, "key-abc", true, testSubject)

			res, err := Middleware()(next, "PostUsers")(ec, spyRequest{Name: "alice"})
			require.NoError(t, err)
			require.True(t, called)
			assert.Equal(t, sentinel, res, "後段の戻り値がそのまま透過されること")

			// middleware は ec.SetRequest で stash 済み。その ctx を Run に渡し Claim の引数を検証する。
			gotCtx := ec.Request().Context()
			ctrl := gomock.NewController(t)
			store := mock_idempotency.NewMockStore(ctrl)
			txm := mock_tx.NewMockManager(ctrl)
			clk := clocktest.NewMockClock(t, time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))
			txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) },
			)

			var got idempotencybndry.ClaimParams
			store.EXPECT().Claim(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, p idempotencybndry.ClaimParams) (bool, error) {
					got = p
					return true, nil
				},
			)
			store.EXPECT().Complete(gomock.Any(), gomock.Any()).Return(nil)

			deps := idempotencyuc.Deps{Txm: txm, Store: store, Clock: clk}
			_, _, runErr := idempotencyuc.Run(gotCtx, deps, 201,
				func(context.Context) (string, error) { return "ok", nil })
			require.NoError(t, runErr)

			assert.Equal(t, testSubject, got.Scope)
			assert.Equal(t, "key-abc", got.Key)
			assert.Equal(t, http.MethodPost, got.Method)
			assert.Equal(t, testPath, got.Path)
			assert.Len(t, got.Fingerprint, fingerprintLen)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("255文字超のキーは400で後段を呼ばない", func(t *testing.T) {
			t.Parallel()
			called := false
			next := NextFunc(func(*echo.Context, any) (any, error) {
				called = true
				return sentinel, nil
			})
			ec := newEcho(t, strings.Repeat("a", 256), true, "user-1")

			_, err := Middleware()(next, "PostUsers")(ec, spyRequest{})

			require.ErrorIs(t, err, apperror.ErrInvalidArgument)
			assert.False(t, called)
		})

		t.Run("非印字ASCIIを含むキーは400で後段を呼ばない", func(t *testing.T) {
			t.Parallel()
			called := false
			next := NextFunc(func(*echo.Context, any) (any, error) {
				called = true
				return sentinel, nil
			})
			ec := newEcho(t, "key with space", true, "user-1")

			_, err := Middleware()(next, "PostUsers")(ec, spyRequest{})

			require.ErrorIs(t, err, apperror.ErrInvalidArgument)
			assert.False(t, called)
		})

		t.Run("指紋生成に失敗するリクエストは500で後段を呼ばない(fail-closed)", func(t *testing.T) {
			t.Parallel()
			called := false
			next := NextFunc(func(*echo.Context, any) (any, error) {
				called = true
				return sentinel, nil
			})
			ec := newEcho(t, "key-1", true, testSubject)

			// chan は json.Marshal できず、弱い指紋を作らずエラーになる。
			_, err := Middleware()(next, "PostUsers")(ec, make(chan int))

			require.ErrorIs(t, err, apperror.ErrInternal)
			assert.False(t, called)
		})
	})
}

func Test_validateKey(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()
		t.Run("印字可能ASCIIは通る", func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validateKey("Idem-Key_123.~"))
		})

		t.Run("255文字ちょうどは通る", func(t *testing.T) {
			t.Parallel()
			// 上限 maxKeyLength=255 ちょうどは通過する（256 文字の異常系と対の境界）。
			require.NoError(t, validateKey(strings.Repeat("a", 255)))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("長すぎ", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateKey(strings.Repeat("x", 256)), apperror.ErrInvalidArgument)
		})

		t.Run("空白(0x20)", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateKey("a b"), apperror.ErrInvalidArgument)
		})

		t.Run("制御文字(0x1f)", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateKey("a\x1fb"), apperror.ErrInvalidArgument)
		})

		t.Run("DEL(0x7f)", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateKey("a\x7fb"), apperror.ErrInvalidArgument)
		})

		t.Run("マルチバイト", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateKey("あ"), apperror.ErrInvalidArgument)
		})
	})
}

func mustFingerprint(t *testing.T, method, path string, request any) []byte {
	t.Helper()
	fp, err := fingerprint(method, path, request)
	require.NoError(t, err)
	return fp
}

func Test_fingerprint(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()
		base := mustFingerprint(t, http.MethodPost, testPath, spyRequest{Name: "alice"})

		t.Run("SHA-256の32バイトを返す", func(t *testing.T) {
			t.Parallel()
			assert.Len(t, base, fingerprintLen)
		})

		t.Run("同一入力は同一指紋(決定的)", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, base, mustFingerprint(t, http.MethodPost, testPath, spyRequest{Name: "alice"}))
		})

		t.Run("リクエストボディが異なれば指紋も異なる", func(t *testing.T) {
			t.Parallel()
			assert.NotEqual(t, base, mustFingerprint(t, http.MethodPost, testPath, spyRequest{Name: "bob"}))
		})

		t.Run("methodが異なれば指紋も異なる", func(t *testing.T) {
			t.Parallel()
			assert.NotEqual(t, base, mustFingerprint(t, http.MethodPut, testPath, spyRequest{Name: "alice"}))
		})

		t.Run("pathが異なれば指紋も異なる", func(t *testing.T) {
			t.Parallel()
			assert.NotEqual(t, base, mustFingerprint(t, http.MethodPost, "/v1/orders", spyRequest{Name: "alice"}))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("marshal不能な値はエラーを返す(fail-closed)", func(t *testing.T) {
			t.Parallel()
			_, err := fingerprint(http.MethodPost, testPath, make(chan int))
			require.Error(t, err)
		})
	})
}

func TestMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("捕捉した next と operationID を handle へ渡す", func(t *testing.T) {
			t.Parallel()
			called := false
			next := NextFunc(func(*echo.Context, any) (any, error) {
				called = true
				return sentinel, nil
			})
			ec := newEcho(t, "key-mw", true, testSubject)

			res, err := Middleware()(next, testOperationID)(ec, spyRequest{Name: "alice"})
			require.NoError(t, err)
			assert.True(t, called)
			assert.Equal(t, sentinel, res)

			// operationID は ctx に stash された Request 経由でしか観測できないため、Run に渡して
			// o11y カウンタへ届いたラベルを見る。
			ctrl := gomock.NewController(t)
			store := mock_idempotency.NewMockStore(ctrl)
			txm := mock_tx.NewMockManager(ctrl)
			clk := clocktest.NewMockClock(t, time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))
			metrics := mock_idempotencyuc.NewMockMetrics(ctrl)
			txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) },
			)
			store.EXPECT().Claim(gomock.Any(), gomock.Any()).Return(true, nil)
			store.EXPECT().Complete(gomock.Any(), gomock.Any()).Return(nil)

			var gotOperationID string
			metrics.EXPECT().IncMiss(gomock.Any(), gomock.Any()).Do(
				func(_ context.Context, operationID string) { gotOperationID = operationID },
			)

			deps := idempotencyuc.Deps{Txm: txm, Store: store, Clock: clk, Metrics: metrics}
			_, _, runErr := idempotencyuc.Run(ec.Request().Context(), deps, 201,
				func(context.Context) (string, error) { return "ok", nil })
			require.NoError(t, runErr)

			assert.Equal(t, testOperationID, gotOperationID)
		})

		t.Run("同一factoryから生成したwrapperはそれぞれの後段を呼び分ける", func(t *testing.T) {
			t.Parallel()

			mw := Middleware()
			firstCalled, secondCalled := false, false
			first := mw(NextFunc(func(*echo.Context, any) (any, error) {
				firstCalled = true
				return "FIRST", nil
			}), "PostResources")
			second := mw(NextFunc(func(*echo.Context, any) (any, error) {
				secondCalled = true
				return "SECOND", nil
			}), "PutResources")

			gotFirst, err := first(newEcho(t, "key-1", true, testSubject), spyRequest{})
			require.NoError(t, err)
			gotSecond, err := second(newEcho(t, "key-2", true, testSubject), spyRequest{})
			require.NoError(t, err)

			assert.Equal(t, "FIRST", gotFirst)
			assert.Equal(t, "SECOND", gotSecond)
			assert.True(t, firstCalled)
			assert.True(t, secondCalled)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("後段が返したerrorをそのまま透過する", func(t *testing.T) {
			t.Parallel()

			boom := xerrors.New("handler boom")
			next := NextFunc(func(*echo.Context, any) (any, error) { return nil, boom })

			_, err := Middleware()(next, "PostResources")(newEcho(t, "key-1", true, testSubject), spyRequest{})

			require.ErrorIs(t, err, boom)
		})
	})
}
