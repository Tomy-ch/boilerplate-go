//go:generate mockgen -source=$GOFILE -destination=mock/mock_$GOFILE.gen.go -package=mock_$GOPACKAGE

// Package user は、ユーザーに関するユースケースを提供します。
package user

import (
	"context"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/domain/coupon"
	"go-boilerplate/internal/domain/prefecture"
	"go-boilerplate/internal/domain/purchase"
	"go-boilerplate/internal/domain/service/membership"
	"go-boilerplate/internal/domain/user"
	"go-boilerplate/internal/observability"
	authbd "go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/internal/usecase/boundary/authz"
	"go-boilerplate/internal/usecase/boundary/clock"
	outboxbndry "go-boilerplate/internal/usecase/boundary/outbox"
	"go-boilerplate/internal/usecase/boundary/tx"
	"go-boilerplate/internal/usecase/outbox"
	"go-boilerplate/internal/usecase/tools/paging"
	"go-boilerplate/internal/usecase/user/event"
	"go-boilerplate/pkg/ptr"
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"
)

// aggregateType は、outbox の集約種別です。
const aggregateType = "user"

// 既存ユーザーが参照する prefecture を解決できない参照整合性破れ（サーバ側データ不整合）を表します。
var errOrphanPrefecture = xerrors.Wrap(apperror.ErrInternal, "prefecture not found for user")

// UserView は、ユーザー取得結果の出力 DTO を表します。
type UserView struct {
	ID             uuid.UUID
	FirstName      string
	LastName       string
	Email          string
	Phone          string
	PostalCode     string
	PrefectureName string
	City           string
	Street         string
	Building       *string
	DeletedAt      *time.Time
}

// UserListView は、一覧取得結果（ページ分の一覧と総件数）を表します。
type UserListView struct {
	Items []UserView
	Total int64
}

// UserFeedView は、ユーザーフィード（cursor ページネーション）の取得結果を表します。
type UserFeedView struct {
	// Items は、現在ページのユーザー一覧です。
	Items []UserView
	// NextCursor は、次ページ取得用の不透明カーソルです。最終ページの場合は nil です。
	NextCursor *string
}

// UpdateProfileParams は、ユーザープロフィール更新の入力（可変フィールド）を表します。
type UpdateProfileParams struct {
	FirstName      string
	LastName       string
	Email          string
	Phone          string
	PostalCode     string
	PrefectureName string
	City           string
	Street         string
	Building       *string
}

// CreateParamsDTO は、ユーザー作成に必要なパラメータを表します。
// 内部ユーザー ID は登録時に採番するため受け取りません。呼出元が渡すのは、認証済みの主体を
// 指す外部アイデンティティ（Issuer + Subject）と本人の属性だけです。
type CreateParamsDTO struct {
	UpdateProfileParams

	// Issuer は、認証したトークンの発行者です。
	Issuer string
	// Subject は、認証したトークンの主体です。
	Subject string
}

// PatchParamsDTO は、ユーザー部分更新（PATCH）に必要なパラメータを表します。nil のフィールドは更新しません。
type PatchParamsDTO struct {
	FirstName      *string
	LastName       *string
	Email          *string
	Phone          *string
	PostalCode     *string
	PrefectureName *string
	City           *string
	Street         *string
	Building       *string
}

type usecase struct {
	tracer       observability.LayerTracer
	txm          tx.Manager
	clock        clock.Clock
	authorizer   authz.Authorizer
	userRepo     user.Repository
	userLock     user.LockRepository
	pftRepo      prefecture.Repository
	purchaseRepo purchase.Repository
	couponRepo   coupon.Repository
	identities   authbd.IdentityRegistrar
	emit         outbox.EmitUsecase
}

// Usecase は、ユーザーに関するユースケースを定義します。
type Usecase interface {
	// ListUsers は、認可を確認したうえでユーザー一覧を取得します。他ユーザーを列挙する操作のため
	// admin ロールを持つ主体のみ許可し、拒否された場合は authz.ErrForbidden を返します。
	ListUsers(ctx context.Context, authn *authbd.Authn, active *bool, page *paging.Page) ([]UserView, error)
	// ListUsersWithTotal は、認可を確認したうえでユーザー一覧と総件数をまとめて取得します。
	// 認可条件は ListUsers と同じく admin 限定です。
	ListUsersWithTotal(ctx context.Context, authn *authbd.Authn, active *bool, page *paging.Page) (*UserListView, error)
	// ListUsersFeed は、認可を確認したうえで未削除ユーザーを作成日時の降順（cursor ページネーション）で
	// 取得します。認可条件は ListUsers と同じく admin 限定です。
	ListUsersFeed(ctx context.Context, authn *authbd.Authn, cursor *paging.Cursor) (*UserFeedView, error)
	// CreateUser は、認証済みの主体を内部ユーザーとして登録します。ユーザーの作成・外部アイデンティティの
	// 結び付け・ウェルカムクーポンの発行を単一トランザクションで行います。
	// 既に登録済みの主体からの呼び出しは apperror.ErrConflict を返し、何も残しません。
	CreateUser(ctx context.Context, dto *CreateParamsDTO) (UserView, error)
	// CountUsers は、ユーザーの総件数を返します。件数のみを返し個々のユーザーを開示しないため、
	// 認可を要求しません。
	CountUsers(ctx context.Context, active *bool) (int64, error)
	// GetUser は、認可を確認したうえで ID から単一ユーザーを取得します。
	// 認可が拒否された場合は authz.ErrForbidden（apperror.ErrPermissionDenied をラップ）を返します。
	GetUser(ctx context.Context, authn *authbd.Authn, id uuid.UUID) (UserView, error)
	// UpdateUser は、認可を確認したうえでユーザーのプロフィールを全更新します。
	// 認可が拒否された場合は authz.ErrForbidden（apperror.ErrPermissionDenied をラップ）を返します。
	UpdateUser(ctx context.Context, authn *authbd.Authn, id uuid.UUID, dto *UpdateProfileParams) (UserView, error)
	// UpdateUserPartially は、認可を確認したうえでユーザーを部分更新します。指定されたフィールドのみを
	// 反映し、未指定 / null は据え置きます（値のクリアは非対応で、クリアには全更新の UpdateUser を使います）。
	// 認可が拒否された場合は authz.ErrForbidden（apperror.ErrPermissionDenied をラップ）を返します。
	UpdateUserPartially(ctx context.Context, authn *authbd.Authn, id uuid.UUID, dto *PatchParamsDTO) (UserView, error)
	// DeleteUser は、認可を確認したうえでユーザーを退会させます。ユーザーを論理削除し、
	// 同一トランザクションで退会イベントを発行します（配信は非同期・結果整合。ADR-0054 (transactional-outbox)）。
	// 進行中の購入が残っている場合は apperror.ErrConflict を返し、退会させません。
	// 認可が拒否された場合は authz.ErrForbidden（apperror.ErrPermissionDenied をラップ）を返します。
	DeleteUser(ctx context.Context, authn *authbd.Authn, id uuid.UUID) error
}

// New は、ユーザーに関するユースケースを初期化します。
func New(
	tf observability.TracerFactory,
	txm tx.Manager,
	clock clock.Clock,
	authorizer authz.Authorizer,
	userRepo user.Repository,
	userLock user.LockRepository,
	prefectureRepo prefecture.Repository,
	purchaseRepo purchase.Repository,
	couponRepo coupon.Repository,
	identities authbd.IdentityRegistrar,
	emit outbox.EmitUsecase,
) Usecase {
	return &usecase{
		tracer:       tf.Usecase(),
		txm:          txm,
		clock:        clock,
		authorizer:   authorizer,
		userRepo:     userRepo,
		userLock:     userLock,
		pftRepo:      prefectureRepo,
		purchaseRepo: purchaseRepo,
		couponRepo:   couponRepo,
		identities:   identities,
		emit:         emit,
	}
}

func (u *usecase) ListUsers(ctx context.Context, authn *authbd.Authn, active *bool, page *paging.Page) ([]UserView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	if err := u.authorizeUserCollection(ctx, authn); err != nil {
		return nil, err
	}

	return u.listUsers(ctx, active, page)
}

func (u *usecase) CreateUser(ctx context.Context, dto *CreateParamsDTO) (UserView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	now := u.clock.Now()

	userID, err := uuid.New()
	if err != nil {
		return UserView{}, xerrors.Wrap(err, "failed to generate user id")
	}

	var (
		userEntity *user.User
		pftName    string
	)
	err = u.txm.Do(ctx, func(ctx context.Context) error {
		pftDomain, err := u.pftRepo.FindByName(ctx, dto.PrefectureName)
		if err != nil {
			return err
		}
		pftName = pftDomain.Name()

		userEntity, err = user.New(userID, user.Attributes{
			FirstName:    dto.FirstName,
			LastName:     dto.LastName,
			Email:        dto.Email,
			Phone:        dto.Phone,
			PrefectureID: pftDomain.ID(),
			City:         dto.City,
			Street:       dto.Street,
			Building:     dto.Building,
			PostalCode:   dto.PostalCode,
			CreatedAt:    now,
			UpdatedAt:    now,
		})
		if err != nil {
			return err
		}

		if err = u.userRepo.Create(ctx, userEntity); err != nil {
			return err
		}

		if err = u.identities.Register(ctx, userID, dto.Issuer, dto.Subject); err != nil {
			return err
		}

		welcome, err := newWelcomeCoupon(userID, now)
		if err != nil {
			return err
		}

		return u.couponRepo.Create(ctx, welcome)
	})
	if err != nil {
		return UserView{}, err
	}

	return toUserView(userEntity, pftName), nil
}

func (u *usecase) CountUsers(ctx context.Context, active *bool) (int64, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()
	return u.userRepo.CountByActive(ctx, active)
}

func (u *usecase) ListUsersWithTotal(ctx context.Context, authn *authbd.Authn, active *bool, page *paging.Page) (*UserListView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	if err := u.authorizeUserCollection(ctx, authn); err != nil {
		return nil, err
	}

	items, err := u.listUsers(ctx, active, page)
	if err != nil {
		return nil, err
	}
	total, err := u.CountUsers(ctx, active)
	if err != nil {
		return nil, err
	}
	return &UserListView{Items: items, Total: total}, nil
}

func (u *usecase) ListUsersFeed(ctx context.Context, authn *authbd.Authn, cursor *paging.Cursor) (*UserFeedView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	if err := u.authorizeUserCollection(ctx, authn); err != nil {
		return nil, err
	}

	if cursor == nil {
		return nil, xerrors.Wrap(apperror.ErrInvalidArgument, "cursor must not be nil")
	}

	after, err := decodeFeedCursor(cursor)
	if err != nil {
		return nil, err
	}

	us, err := u.userRepo.FindFeed(ctx, after, cursor.Limit32()+1)
	if err != nil {
		return nil, err
	}

	limit := cursor.Limit()
	hasNext := len(us) > limit
	if hasNext {
		us = us[:limit]
	}

	items, err := u.toUserViews(ctx, us)
	if err != nil {
		return nil, err
	}

	var nextCursor *string
	if hasNext && len(us) > 0 {
		encoded := encodeFeedCursor(us[len(us)-1])
		nextCursor = &encoded
	}

	return &UserFeedView{Items: items, NextCursor: nextCursor}, nil
}

func (u *usecase) GetUser(ctx context.Context, authn *authbd.Authn, id uuid.UUID) (UserView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	if err := u.authorizeUserAccess(ctx, authn, authz.ActionUserGet, id); err != nil {
		return UserView{}, err
	}

	userEntity, err := u.userRepo.FindByID(ctx, id)
	if err != nil {
		return UserView{}, err
	}

	pftDomain, err := u.pftRepo.FindByID(ctx, userEntity.PrefectureID())
	if err != nil {
		if xerrors.Is(err, apperror.ErrNotFound) {
			return UserView{}, errOrphanPrefecture
		}
		return UserView{}, err
	}

	return toUserView(userEntity, pftDomain.Name()), nil
}

func (u *usecase) UpdateUser(ctx context.Context, authn *authbd.Authn, id uuid.UUID, dto *UpdateProfileParams) (UserView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	if err := u.authorizeUserAccess(ctx, authn, authz.ActionUserUpdate, id); err != nil {
		return UserView{}, err
	}

	now := u.clock.Now()

	var (
		userEntity *user.User
		pftName    string
	)
	err := u.txm.Do(ctx, func(ctx context.Context) error {
		var err error
		userEntity, err = u.userRepo.FindByID(ctx, id)
		if err != nil {
			return err
		}

		pftDomain, err := u.pftRepo.FindByName(ctx, dto.PrefectureName)
		if err != nil {
			return err
		}
		pftName = pftDomain.Name()

		if err = userEntity.UpdateProfile(user.Profile{
			FirstName:    dto.FirstName,
			LastName:     dto.LastName,
			Email:        dto.Email,
			Phone:        dto.Phone,
			PrefectureID: pftDomain.ID(),
			City:         dto.City,
			Street:       dto.Street,
			Building:     dto.Building,
			PostalCode:   dto.PostalCode,
		}, now); err != nil {
			return err
		}

		return u.userRepo.Update(ctx, userEntity)
	})
	if err != nil {
		return UserView{}, err
	}

	return toUserView(userEntity, pftName), nil
}

func (u *usecase) UpdateUserPartially(ctx context.Context, authn *authbd.Authn, id uuid.UUID, dto *PatchParamsDTO) (UserView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	if err := u.authorizeUserAccess(ctx, authn, authz.ActionUserUpdate, id); err != nil {
		return UserView{}, err
	}

	now := u.clock.Now()

	var (
		userEntity *user.User
		pftName    string
	)
	err := u.txm.Do(ctx, func(ctx context.Context) error {
		var err error
		userEntity, err = u.userRepo.FindByID(ctx, id)
		if err != nil {
			return err
		}

		prefectureID := userEntity.PrefectureID()
		pftDomain, err := u.resolvePatchPrefecture(ctx, dto.PrefectureName, prefectureID)
		if err != nil {
			return err
		}
		prefectureID = pftDomain.ID()
		pftName = pftDomain.Name()

		building := userEntity.Building()
		if dto.Building != nil {
			building = dto.Building
		}

		if err = userEntity.UpdateProfile(user.Profile{
			FirstName:    ptr.Deref(dto.FirstName, userEntity.FirstName()),
			LastName:     ptr.Deref(dto.LastName, userEntity.LastName()),
			Email:        ptr.Deref(dto.Email, userEntity.Email()),
			Phone:        ptr.Deref(dto.Phone, userEntity.Phone()),
			PrefectureID: prefectureID,
			City:         ptr.Deref(dto.City, userEntity.City()),
			Street:       ptr.Deref(dto.Street, userEntity.Street()),
			Building:     building,
			PostalCode:   ptr.Deref(dto.PostalCode, userEntity.PostalCode()),
		}, now); err != nil {
			return err
		}
		return u.userRepo.Update(ctx, userEntity)
	})
	if err != nil {
		return UserView{}, err
	}

	return toUserView(userEntity, pftName), nil
}

func (u *usecase) DeleteUser(ctx context.Context, authn *authbd.Authn, id uuid.UUID) error {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	if err := u.authorizeUserAccess(ctx, authn, authz.ActionUserDelete, id); err != nil {
		return err
	}

	now := u.clock.Now()
	// 論理削除と退会イベントの発行を単一 tx にまとめ、退会だけが成立してイベントが失われることを防ぐ。
	// 退会を拒む条件（進行中の購入）も同じ tx で判定し、拒否時は論理削除もイベントも残さない。
	return u.txm.Do(ctx, func(ctx context.Context) error {
		// 進行中購入の判定より前に排他ロックを取ることが、購入作成（共有ロック）との直列化の成立条件。
		// 判定より後だと「判定通過 → 購入の成立 → 退会の確定」の順序を止められない。
		userEntity, err := u.userLock.LockByID(ctx, id)
		if err != nil {
			return err
		}

		statuses, err := u.purchaseRepo.FindStatusesByUserID(ctx, id)
		if err != nil {
			return err
		}
		if err = membership.EnsureWithdrawable(userEntity, statuses); err != nil {
			return err
		}

		if err = userEntity.MarkAsDeleted(now); err != nil {
			return err
		}
		if err = u.userRepo.Update(ctx, userEntity); err != nil {
			return err
		}

		payload, err := event.BuildWithdrawn(userEntity)
		if err != nil {
			return err
		}
		_, err = u.emit.Emit(ctx, outbox.EmitInput{
			AggregateType: aggregateType,
			AggregateID:   id.String(),
			EventType:     event.TypeWithdrawn,
			Payload:       payload,
			Channel:       outboxbndry.ChannelHTTP,
		})
		return err
	})
}

// listUsers は、認可済みの呼出元に対してユーザー一覧を取得します。
func (u *usecase) listUsers(ctx context.Context, active *bool, page *paging.Page) ([]UserView, error) {
	if page == nil {
		return nil, xerrors.Wrap(apperror.ErrInvalidArgument, "page must not be nil")
	}

	us, err := u.userRepo.FindByActive(ctx, active, page.Limit32(), page.Offset32())
	if err != nil {
		return nil, err
	}

	return u.toUserViews(ctx, us)
}

// authorizeUserAccess は、認証主体 authn が対象ユーザー（所有者 = id）への action を実行してよいか判定します。
// リソースの所有者を対象ユーザー（id）とすることで、所有権モデルでは呼出元が自分自身のみを操作できます。
// authn が nil の場合は、認可判定以前に呼出元を特定できないため apperror.ErrUnauthenticated を返します。
func (u *usecase) authorizeUserAccess(ctx context.Context, authn *authbd.Authn, action authz.Action, id uuid.UUID) error {
	if authn == nil {
		return apperror.ErrUnauthenticated
	}
	return u.authorizer.Authorize(ctx, authn, action, authz.NewResource("user", &id))
}

// authorizeUserCollection は、認証主体 authn がユーザーの列挙を行ってよいか判定します。
// 所有者を持たないリソース（ownerID = nil）として問い合わせるため所有者フォールバックが成立せず、
// admin ロールを持つ主体だけが許可されます。
// authn が nil の場合は、認可判定以前に呼出元を特定できないため apperror.ErrUnauthenticated を返します。
func (u *usecase) authorizeUserCollection(ctx context.Context, authn *authbd.Authn) error {
	if authn == nil {
		return apperror.ErrUnauthenticated
	}
	return u.authorizer.Authorize(ctx, authn, authz.ActionUserList, authz.NewResource("user", nil))
}

// toUserViews は、ユーザーエンティティ列を UserView の DTO 列へ変換します。
// いずれかのユーザーが参照する都道府県を解決できない場合は参照整合性破れ（errOrphanPrefecture）を返します。
func (u *usecase) toUserViews(ctx context.Context, us user.Users) ([]UserView, error) {
	_, prefectureMap, err := observability.RunWithSpan(
		ctx, u.tracer, "usecase", "user", "prefectureMap", func(ctx context.Context) (map[uuid.UUID]*prefecture.Prefecture, error) {
			pids := make([]uuid.UUID, len(us))
			for i, ue := range us {
				pids[i] = ue.PrefectureID()
			}

			ps, pftErr := u.pftRepo.FindByIDs(ctx, pids)
			if pftErr != nil {
				return nil, pftErr
			}

			prefectureMap := make(map[uuid.UUID]*prefecture.Prefecture, len(ps))
			for _, p := range ps {
				prefectureMap[p.ID()] = p
			}

			return prefectureMap, nil
		},
	)
	if err != nil {
		return nil, err
	}

	dtos := make([]UserView, len(us))
	for i, ue := range us {
		p, ok := prefectureMap[ue.PrefectureID()]
		if !ok {
			return nil, errOrphanPrefecture
		}
		dtos[i] = toUserView(ue, p.Name())
	}

	return dtos, nil
}

// resolvePatchPrefecture は、部分更新時の都道府県を解決します。
// 名前指定があれば名前で解決（入力エラーは伝播）、なければ既存 ID で解決します（未解決は参照整合性破れ）。
func (u *usecase) resolvePatchPrefecture(ctx context.Context, name *string, currentID uuid.UUID) (*prefecture.Prefecture, error) {
	if name != nil {
		return u.pftRepo.FindByName(ctx, *name)
	}
	pftDomain, err := u.pftRepo.FindByID(ctx, currentID)
	if xerrors.Is(err, apperror.ErrNotFound) {
		return nil, errOrphanPrefecture
	}
	if err != nil {
		return nil, err
	}
	return pftDomain, nil
}

// toUserView は、ユーザーエンティティと都道府県名から DTO を構築します。
func toUserView(u *user.User, prefectureName string) UserView {
	return UserView{
		ID:             u.ID(),
		FirstName:      u.FirstName(),
		LastName:       u.LastName(),
		Email:          u.Email(),
		Phone:          u.Phone(),
		PostalCode:     u.PostalCode(),
		PrefectureName: prefectureName,
		City:           u.City(),
		Street:         u.Street(),
		Building:       u.Building(),
		DeletedAt:      u.DeletedAt(),
	}
}
