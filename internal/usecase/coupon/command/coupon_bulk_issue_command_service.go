//go:generate mockgen -source=$GOFILE -destination=mock/mock_$GOFILE.gen.go -package=mock_$GOPACKAGE

// Package command は、販促クーポン一括発行の書き込み操作（CommandService）のインターフェースを
// 定義します（ADR-0032 (lightweight-cqrs)）。実装は infra 層に置き、渡された ctx のトランザクションに
// 参加します。
//
// 所在が usecase 層なのは、CommandService がトランザクションの道具であり、所有者はトランザクションを
// 開く側だからです。パッケージ名 command はワークフローの名であって集約の名ではありません。
package command

import (
	"context"
	"time"

	"go-boilerplate/internal/domain/coupon"
)

// IssuePromotionalCouponsParams は、販促クーポンを一括発行するための入力です。
// ExpiresAt と IssuedAt が同型のため構造体で受けます（docs/rules.md の Function Signature Rules）。
type IssuePromotionalCouponsParams struct {
	// Scope は、発行するクーポンの適用範囲です。検証済みの値オブジェクトを受け取ります。
	// どの範囲を配るかは業務の判断なので、決めるのも組み立てるのも呼び出し側です。
	Scope coupon.Scope
	// Discount は、発行するクーポンの値引きです。全員に同じ条件で配ります。
	Discount coupon.Discount
	// ExpiresAt は、発行するクーポンの有効期限です。
	ExpiresAt time.Time
	// IssuedAt は、発行日時です。
	IssuedAt time.Time
}

// IssuePromotionalCouponsResult は、一括発行の結果です。
type IssuePromotionalCouponsResult struct {
	// RecipientCount は、受給対象になった利用者の数です。退会済みは含みません。
	RecipientCount int64
	// IssuedCouponCount は、実際に発行した枚数です。受給者 1 人につき 1 枚のため RecipientCount と一致します。
	IssuedCouponCount int64
}

// CommandService は、販促クーポンの一括発行を定義します。
//
// 載せてよい書き込みの基準と、強制する条件がドメイン不変条件からの導出でなければならない規律は
// ADR-0032 (lightweight-cqrs) の Eligibility / Derivation を参照。
type CommandService interface {
	// IssuePromotionalCoupons は、退会していないすべての利用者へ、同一条件のクーポンを 1 枚ずつ
	// 発行します。渡された ctx のトランザクション内で実行します。
	//
	// 集約ではなく発行条件を受け取る理由と、往復数が母集団に比例しない根拠は
	// ADR-0114 (predicate-defined-set-writes-on-commandservice) を参照。原子性ではなく受給者を
	// 識別子で名指しできないことがこの構造の理由です。
	// 個々の Coupon は受給者を読んだあとにドメインのコンストラクタを通して組み立てます。
	// 発行枚数の上限は強制しません。呼び出し側が書き込み前に判定します
	// （docs/spec/usecase/coupon.md の Command Service を参照）。
	IssuePromotionalCoupons(ctx context.Context, params IssuePromotionalCouponsParams) (IssuePromotionalCouponsResult, error)
}
