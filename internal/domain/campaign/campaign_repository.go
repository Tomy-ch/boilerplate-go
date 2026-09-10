//go:generate mockgen -source=$GOFILE -destination=mock/mock_$GOFILE.gen.go -package=mock_$GOPACKAGE
package campaign

import (
	"context"
	"time"

	"go-boilerplate/pkg/uuid"
)

// ListParams は、キャンペーン一覧取得の取得範囲を表すクエリ条件です。
type ListParams struct {
	// Limit は、取得件数の上限です。
	Limit int32
	// Offset は、読み飛ばす件数です。
	Offset int32
}

// ClaimCountParams は、受け取り枚数を数える対象を表すクエリ条件です。
// 同型の識別子が 2 つ並ぶため構造体で受けます（基準は docs/rules.md の Function Signature Rules）。
type ClaimCountParams struct {
	// CampaignID は、数える対象のキャンペーンです。
	CampaignID uuid.UUID
	// UserID は、数える対象の利用者です。
	UserID uuid.UUID
}

// Repository は、キャンペーンの永続化を担うインターフェースです。
type Repository interface {
	// Create は、定義したキャンペーンを 1 件永続化します。対象は [New] が生成した、
	// まだ 1 枚も配っていないキャンペーンです。呼び出し元のトランザクションがあればそれに参加します。
	// コードが既に使われている場合は Conflict を返します。
	Create(ctx context.Context, c *Campaign) error
	// FindList は、キャンペーンを配布期間の開始が新しい順で返します。停止済みも含みます。
	FindList(ctx context.Context, params ListParams) (Campaigns, error)
	// CountAll は、キャンペーンの総件数を返します。
	CountAll(ctx context.Context) (int, error)
	// LockByCode は、受け取りのために正規化済みコードからキャンペーンを取得します。
	// 存在しない場合は NotFound を返します。同一キャンペーンへの並行更新は、先行する更新が
	// 終わるまで待機したうえで最新の状態を取得します。
	//
	// 配布期間・停止・上限では絞りません（理由は docs/spec/domain/campaign.md の
	// Repository Methods > LockByCode と ADR-0036 (ordered-pessimistic-row-locks) の決定 5 を参照）。
	LockByCode(ctx context.Context, code Code) (*Campaign, error)
	// LockByID は、停止のために ID からキャンペーンを取得します。
	// 絞り込みを置かない理由は [Repository.LockByCode] と同じです。
	LockByID(ctx context.Context, id uuid.UUID) (*Campaign, error)
	// CountClaims は、指定利用者が指定キャンペーンから既に受け取った枚数を返します。
	// 1 人あたり上限の判定に用いるため、キャンペーン行のロックを取ったあとで呼びます。
	CountClaims(ctx context.Context, params ClaimCountParams) (int, error)
	// RecordClaim は、受け取りを 1 件記録します。発行済み枚数の加算と受け取り記録の挿入を
	// 1 つの呼び出しで行い、片方だけが残る状態を作りません。
	// 他の書き手が先に総枚数上限を埋めていた場合は ErrIssuedConcurrently を返します。
	RecordClaim(ctx context.Context, claim *Claim) error
	// UpdateSuspended は、キャンペーンを停止済みにします。対象は [Repository.LockByID] で取得し
	// [Campaign.Suspend] で検証済みです。既に停止済みだった場合は ErrAlreadySuspended を返します。
	UpdateSuspended(ctx context.Context, id uuid.UUID, suspendedAt time.Time) error
}
