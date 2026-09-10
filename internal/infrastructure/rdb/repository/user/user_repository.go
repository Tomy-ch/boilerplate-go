// Package user は、ユーザーリポジトリ（user.Repository）の RDB 実装を提供します。
package user

import (
	"context"
	"time"

	"go-boilerplate/internal/domain/user"
	"go-boilerplate/internal/infrastructure/rdb/driver"
	"go-boilerplate/internal/infrastructure/rdb/pgerror"
	"go-boilerplate/internal/infrastructure/rdb/sqlc"
	"go-boilerplate/internal/infrastructure/rdb/sqlc/gen"
	"go-boilerplate/internal/observability"
	"go-boilerplate/pkg/uuid"
)

type repository struct {
	db     driver.DatabaseDriver
	tracer observability.LayerTracer
}

// New は、user.Repository の RDB 実装を生成して返します。
func New(
	db driver.DatabaseDriver,
	tf observability.TracerFactory,
) user.Repository {
	return &repository{
		db:     db,
		tracer: tf.Infra(),
	}
}

// FindByActive は、アクティブ状態に基づいてユーザーの情報を取得します。
func (r *repository) FindByActive(ctx context.Context, active *bool, limit, offset int32) (user.Users, error) {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, r.db))

	switch {
	case active == nil:
		return fetchListUsersRows(ctx, db, &gen.ListUsersParams{
			OffsetParam: offset,
			LimitParam:  limit,
		})
	case *active:
		return fetchListUsersRowsByActive(ctx, db, &gen.ListActiveUsersParams{
			OffsetParam: offset,
			LimitParam:  limit,
		})
	default:
		return fetchListUsersRowsByDeleted(ctx, db, &gen.ListDeletedUsersParams{
			OffsetParam: offset,
			LimitParam:  limit,
		})
	}
}

// FindFeed は、(created_at DESC, id DESC) の keyset で取得します。
func (r *repository) FindFeed(ctx context.Context, after *user.FeedCursor, limit int32) (user.Users, error) {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, r.db))

	if after == nil {
		rows, err := db.ListUsersFeedFirst(ctx, limit)
		if err != nil {
			return nil, pgerror.NormalizeError(err)
		}
		return rowsToUsers(rows, func(r *gen.ListUsersFeedFirstRow) gen.Users { return r.Users })
	}

	rows, err := db.ListUsersFeedAfter(ctx, &gen.ListUsersFeedAfterParams{
		AfterCreatedAt: after.CreatedAt(),
		AfterID:        after.ID(),
		LimitParam:     limit,
	})
	if err != nil {
		return nil, pgerror.NormalizeError(err)
	}
	return rowsToUsers(rows, func(r *gen.ListUsersFeedAfterRow) gen.Users { return r.Users })
}

// SearchByKeyword は、buildLikeTokens が組み立てた ILIKE パターン列のいずれかに search_text が一致するユーザーを、
// active の値で 3 つの固定クエリ（全件 / アクティブのみ / 削除済みのみ）へ振り分けて取得します。
// keywords が空の場合は全件マッチのパターン（%）を用います。
func (r *repository) SearchByKeyword(ctx context.Context, keywords []string, active *bool, limit, offset int32) (user.Users, error) {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	tokens := buildLikeTokens(keywords)
	db := gen.New(driver.New(ctx, r.db))

	switch {
	case active == nil:
		rows, err := db.SearchUsers(ctx, &gen.SearchUsersParams{
			PatternsParam: tokens,
			LimitParam:    limit,
			OffsetParam:   offset,
		})
		if err != nil {
			return nil, pgerror.NormalizeError(err)
		}
		return rowsToUsers(rows, func(r *gen.SearchUsersRow) gen.Users { return r.Users })
	case *active:
		rows, err := db.SearchActiveUsers(ctx, &gen.SearchActiveUsersParams{
			PatternsParam: tokens,
			LimitParam:    limit,
			OffsetParam:   offset,
		})
		if err != nil {
			return nil, pgerror.NormalizeError(err)
		}
		return rowsToUsers(rows, func(r *gen.SearchActiveUsersRow) gen.Users { return r.Users })
	default:
		rows, err := db.SearchDeletedUsers(ctx, &gen.SearchDeletedUsersParams{
			PatternsParam: tokens,
			LimitParam:    limit,
			OffsetParam:   offset,
		})
		if err != nil {
			return nil, pgerror.NormalizeError(err)
		}
		return rowsToUsers(rows, func(r *gen.SearchDeletedUsersRow) gen.Users { return r.Users })
	}
}

// buildLikeTokens は、キーワードを部分一致の LIKE パターンへ変換します。空の場合は全件マッチの ["%"] を返します。
func buildLikeTokens(keywords []string) []string {
	if len(keywords) == 0 {
		return []string{"%"}
	}
	tokens := make([]string, len(keywords))
	for i, kw := range keywords {
		escaped := sqlc.EscapeForLike(kw, sqlc.DefaultLikeEscapeChar)
		tokens[i] = sqlc.WrapContainsLikePattern(escaped)
	}
	return tokens
}

// rowToUser は、sqlc が返す Users 行をドメインエンティティへ変換します。
func rowToUser(u gen.Users) (*user.User, error) {
	entity, err := user.New(u.ID, user.Attributes{
		FirstName:    u.FirstName,
		LastName:     u.LastName,
		Email:        u.Email,
		Phone:        u.Phone,
		PrefectureID: u.PrefectureID,
		City:         u.City,
		Street:       u.Street,
		Building:     u.Building,
		PostalCode:   u.PostalCode,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
		DeletedAt:    u.DeletedAt,
	})
	if err != nil {
		return nil, pgerror.NormalizeReconstructError(err)
	}
	return entity, nil
}

// rowsToUsers は、行スライスをドメインエンティティ列へ変換します。
func rowsToUsers[T any](rows []T, extract func(T) gen.Users) (user.Users, error) {
	users := make(user.Users, len(rows))
	for i, row := range rows {
		u, err := rowToUser(extract(row))
		if err != nil {
			return nil, err
		}
		users[i] = u
	}
	return users, nil
}

// fetchListUsersRows は、ユーザーの情報を取得します。
func fetchListUsersRows(
	ctx context.Context, db *gen.Queries, params *gen.ListUsersParams,
) (user.Users, error) {
	rows, err := db.ListUsers(ctx, params)
	if err != nil {
		return nil, pgerror.NormalizeError(err)
	}
	return rowsToUsers(rows, func(r *gen.ListUsersRow) gen.Users { return r.Users })
}

// fetchListUsersRowsByActive は、アクティブ状態に基づいてユーザーの情報を取得します。
func fetchListUsersRowsByActive(
	ctx context.Context, db *gen.Queries, params *gen.ListActiveUsersParams,
) (user.Users, error) {
	rows, err := db.ListActiveUsers(ctx, params)
	if err != nil {
		return nil, pgerror.NormalizeError(err)
	}
	return rowsToUsers(rows, func(r *gen.ListActiveUsersRow) gen.Users { return r.Users })
}

// fetchListUsersRowsByDeleted は、削除されたユーザーの情報を取得します。
func fetchListUsersRowsByDeleted(
	ctx context.Context, db *gen.Queries, params *gen.ListDeletedUsersParams,
) (user.Users, error) {
	rows, err := db.ListDeletedUsers(ctx, params)
	if err != nil {
		return nil, pgerror.NormalizeError(err)
	}
	return rowsToUsers(rows, func(r *gen.ListDeletedUsersRow) gen.Users { return r.Users })
}

func (r *repository) Create(ctx context.Context, u *user.User) error {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, r.db))
	err := db.CreateUser(ctx, &gen.CreateUserParams{
		ID:           u.ID(),
		FirstName:    u.FirstName(),
		LastName:     u.LastName(),
		Email:        u.Email(),
		Phone:        u.Phone(),
		PrefectureID: u.PrefectureID(),
		City:         u.City(),
		Street:       u.Street(),
		Building:     u.Building(),
		PostalCode:   u.PostalCode(),
		CreatedAt:    u.CreatedAt(),
		UpdatedAt:    u.UpdatedAt(),
	})
	if err != nil {
		return pgerror.NormalizeError(err)
	}
	return nil
}

// FindByID は、IDから単一ユーザーを取得します。存在しない場合は NotFound に正規化したエラーを返します。
func (r *repository) FindByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, r.db))
	row, err := db.GetUserByID(ctx, id)
	if err != nil {
		return nil, pgerror.NormalizeError(err)
	}

	return rowToUser(row.Users)
}

// Update は、ユーザーの mutable フィールドと updatedAt / deletedAt を更新します。
// 影響行数 0 は対象不存在として pgerror.NormalizeExecResult が NotFound へ正規化します。
func (r *repository) Update(ctx context.Context, u *user.User) error {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, r.db))
	rows, err := db.UpdateUser(ctx, &gen.UpdateUserParams{
		FirstName:    u.FirstName(),
		LastName:     u.LastName(),
		Email:        u.Email(),
		Phone:        u.Phone(),
		PrefectureID: u.PrefectureID(),
		City:         u.City(),
		Street:       u.Street(),
		Building:     u.Building(),
		PostalCode:   u.PostalCode(),
		UpdatedAt:    u.UpdatedAt(),
		DeletedAt:    u.DeletedAt(),
		ID:           u.ID(),
	})
	return pgerror.NormalizeExecResult(rows, err)
}

// CountByActive は、アクティブ状態に基づいてユーザーの総件数を返します。
func (r *repository) CountByActive(ctx context.Context, active *bool) (int64, error) {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, r.db))

	var (
		count int64
		err   error
	)

	switch {
	case active == nil:
		count, err = db.CountUsers(ctx)
	case *active:
		count, err = db.CountActiveUsers(ctx)
	default:
		count, err = db.CountDeletedUsers(ctx)
	}

	if err != nil {
		return 0, pgerror.NormalizeError(err)
	}

	return count, nil
}

// FindDeletedBefore は、deleted_at が cutoff より古い users の ID を id 昇順で最大 limit 件返します。
// afterID=nil なら先頭から、それ以外は afterID より大きい id を返します。
func (r *repository) FindDeletedBefore(ctx context.Context, cutoff time.Time, afterID *uuid.UUID, limit int32) ([]uuid.UUID, error) {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, r.db))

	var (
		ids []uuid.UUID
		err error
	)
	if afterID == nil {
		ids, err = db.ListPurgeCandidateUserIDsFirst(ctx, &gen.ListPurgeCandidateUserIDsFirstParams{
			Cutoff:     &cutoff,
			LimitParam: limit,
		})
	} else {
		ids, err = db.ListPurgeCandidateUserIDsAfter(ctx, &gen.ListPurgeCandidateUserIDsAfterParams{
			Cutoff:     &cutoff,
			AfterID:    *afterID,
			LimitParam: limit,
		})
	}
	if err != nil {
		return nil, pgerror.NormalizeError(err)
	}
	return ids, nil
}

// PurgeByIDs は、coupons → user_identities → user_roles → users の順に削除し、users の削除件数を返します。
// 子行を先に消すことで FK 違反を避けます。
// 4 クエリとも deleted_at IS NOT NULL を条件に持つため、論理削除されていないユーザーの ID を渡しても
// そのユーザーは子行を含めて一切削除されず、返る件数が ids の件数を下回ります。
func (r *repository) PurgeByIDs(ctx context.Context, ids []uuid.UUID) (int64, error) {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	if len(ids) == 0 {
		return 0, nil
	}

	db := gen.New(driver.New(ctx, r.db))
	if err := db.DeleteCampaignClaimsByUserIDs(ctx, ids); err != nil {
		return 0, pgerror.NormalizeError(err)
	}
	if err := db.DeleteCouponsByUserIDs(ctx, ids); err != nil {
		return 0, pgerror.NormalizeError(err)
	}
	if err := db.DeleteUserIdentitiesByUserIDs(ctx, ids); err != nil {
		return 0, pgerror.NormalizeError(err)
	}
	if err := db.DeleteUserRolesByUserIDs(ctx, ids); err != nil {
		return 0, pgerror.NormalizeError(err)
	}
	purged, err := db.DeleteUsersByIDs(ctx, ids)
	if err != nil {
		return 0, pgerror.NormalizeError(err)
	}
	return purged, nil
}

func (r *repository) CountByKeyword(ctx context.Context, keywords []string, active *bool) (int64, error) {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	tokens := buildLikeTokens(keywords)
	db := gen.New(driver.New(ctx, r.db))

	var (
		count int64
		err   error
	)

	switch {
	case active == nil:
		count, err = db.CountSearchUsers(ctx, tokens)
	case *active:
		count, err = db.CountSearchActiveUsers(ctx, tokens)
	default:
		count, err = db.CountSearchDeletedUsers(ctx, tokens)
	}

	if err != nil {
		return 0, pgerror.NormalizeError(err)
	}

	return count, nil
}
