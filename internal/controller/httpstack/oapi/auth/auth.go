// Package auth は認証に関連する設定を扱うパッケージです。
package auth

import (
	"context"
	"net/http"
	"strings"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/controller/ctxhelper"
	"go-boilerplate/internal/controller/error/response"
	authbd "go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/pkg/xerrors"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/labstack/echo/v5"
)

const prefixBearer = "Bearer "

// SchemeGroup は、Bearer 以外の securityScheme を担当する SchemeAuthenticator を集める fx group の名前です。
// 集めた認証器の選び方は NewAuthenticator を参照。
const SchemeGroup = "oapi.security.schemes"

const (
	// securitySchemeTypeHTTP は、OpenAPI の securityScheme type のうち HTTP 認証（Bearer を含む）を表す値です。
	securitySchemeTypeHTTP = "http"
	// securitySchemeBearer は、HTTP 認証の scheme のうち Bearer を表す値です。
	securitySchemeBearer = "bearer"
	// SchemeBearerRegistration は、内部ユーザーが未登録の主体を受け付ける securityScheme の名前です
	// （spec の securitySchemes のキー）。この名前を宣言した operation だけ、外部アイデンティティを
	// 内部ユーザーへ解決せずに通します。宣言できるのは内部ユーザーを自ら作る操作だけです。
	//nolint:gosec // G101: securityScheme の名前であって資格情報ではない
	SchemeBearerRegistration = "BearerAuthRegistration"
)

// SchemeAuthenticator は、Bearer 以外の 1 つの securityScheme を担当する認証器です。
// NewAuthenticator が選び、検証結果は request の context スロットへ書き込みます。
type SchemeAuthenticator interface {
	// Scheme は、この認証器が担当する securityScheme の名前（spec の securitySchemes のキー）を返します。
	Scheme() string
	// Authenticate は、input の資格情報を検証します。失敗は apperror の分類を持つエラーで返します。
	Authenticate(ctx context.Context, input *openapi3filter.AuthenticationInput) error
}

// NewAuthenticator は、認証用のOpenAPIオプションを返します。
// operation が宣言した securityScheme の名前が schemes のいずれかと一致すればその認証器へ委譲し、
// 一致しなければ Bearer として扱います。Bearer でもない scheme に認証器が無い場合は検証できないため拒否します。
// 同じ scheme を担当する認証器が 2 つあれば ErrDuplicateScheme を返します（起動時に結線の不具合として落とす）。
func NewAuthenticator(
	authenticator authbd.Authenticator,
	resolver authbd.IdentityResolver,
	schemes []SchemeAuthenticator,
) (openapi3filter.AuthenticationFunc, error) {
	byName := make(map[string]SchemeAuthenticator, len(schemes))
	for _, s := range schemes {
		if _, dup := byName[s.Scheme()]; dup {
			return nil, xerrors.Wrap(ErrDuplicateScheme, s.Scheme())
		}
		byName[s.Scheme()] = s
	}

	return func(ctx context.Context, input *openapi3filter.AuthenticationInput) error {
		req := input.RequestValidationInput.Request

		if s, ok := byName[input.SecuritySchemeName]; ok {
			if err := s.Authenticate(ctx, input); err != nil {
				failure := withHTTPStatus(err)
				//nolint:contextcheck // input が内包する request の context のスロットへ書き戻すため
				ctxhelper.SetAuthnFailure(req.Context(), failure)
				return failure
			}
			return nil
		}

		if !isBearerScheme(input.SecurityScheme) {
			failure := withHTTPStatus(xerrors.Wrap(ErrUnauthorizedSchemeUnsupported, input.SecuritySchemeName))
			//nolint:contextcheck // 同上
			ctxhelper.SetAuthnFailure(req.Context(), failure)
			return failure
		}

		resolveIdentity := input.SecuritySchemeName != SchemeBearerRegistration

		// OpenAPI バリデータが渡す context は context.Background() から組み立てられており、
		// スパン・deadline・キャンセルのいずれも持たない。認証は request の予算の内側で行う。
		//nolint:contextcheck // 引数の context ではなく input が内包する request の context を用いるため
		authn, err := authExtractor(req.Context(), req, authenticator, resolver, resolveIdentity)
		if err != nil {
			failure := withHTTPStatus(err)
			// 記録するのは資格情報が提示されたうえでの失敗だけ。未提示は失敗ではない。
			//nolint:contextcheck // input が内包する request の context のスロットへ書き戻すため
			ctxhelper.SetAuthnFailure(req.Context(), failure)
			return failure
		}
		if authn == nil {
			return withHTTPStatus(ErrUnauthorizedTokenNotProvided)
		}

		//nolint:contextcheck // input が内包する request の context のスロットへ書き戻すため
		if !ctxhelper.SetAuthn(req.Context(), *authn) {
			failure := withHTTPStatus(ErrAuthnSlotNotFound)
			// 認証は成立したが結果を運べていない。記録しなければ、認証済みの主体が匿名として通る。
			//nolint:contextcheck // 同上
			ctxhelper.SetAuthnFailure(req.Context(), failure)
			return failure
		}
		return nil
	}, nil
}

// isBearerScheme は、scheme を Bearer 経路で扱ってよいかを返します。
// 宣言が渡されない場合（nil）は Bearer とみなします — 既定の経路であり、spec 外からの呼び出しがこれに当たります。
func isBearerScheme(scheme *openapi3.SecurityScheme) bool {
	if scheme == nil {
		return true
	}
	return scheme.Type == securitySchemeTypeHTTP && strings.EqualFold(scheme.Scheme, securitySchemeBearer)
}

// withHTTPStatus は、認証フェーズのエラーへ apperror の分類に対応するステータスを持たせます（元のエラーは保持）。
// OpenAPI バリデータはステータスを解決できないエラーを 403 へ丸めるため、この段で持たせないと、
// 認可の判定を行っていないものが認可の結論として外へ出ます。
// メッセージは空のまま返します（本文は errorhandler が組み立てる）。
func withHTTPStatus(err error) error {
	var he *echo.HTTPError
	if xerrors.As(err, &he) {
		return err
	}
	return echo.NewHTTPError(response.NewHTTPErrorFromAppError(err).HTTPStatus, "").Wrap(err)
}

// authExtractor は、Bearer トークンを検証して内部ユーザーを解決した Authn を返します。
// トークンが無い場合は nil, nil を返します。
// resolveIdentity が false の場合は解決を行わず、UserID 未解決の Authn を返します。
func authExtractor(
	ctx context.Context,
	req *http.Request,
	authenticator authbd.Authenticator,
	resolver authbd.IdentityResolver,
	resolveIdentity bool,
) (*authbd.Authn, error) {
	scheme, token := extractBearerToken(req)
	if token == "" {
		//nolint:nilnil // トークン未提供を表す。呼び出し側で authn==nil を判定し未提供エラーへ変換するため意図的にnil,nilを返す
		return nil, nil
	}

	cred, err := authbd.NewCredential(scheme, token)
	if err != nil {
		return nil, err
	}

	// 資格情報についての結論だけを認証エラーへ寄せ、原因は外へ出さない。
	// 検証を遂行できなかった場合（鍵が引けない / 予算切れ）は結論ではないため、分類を保ったまま返す。
	authn, err := authenticator.Authenticate(ctx, cred)
	if err != nil {
		if xerrors.Is(err, apperror.ErrUnauthenticated) {
			return nil, ErrUnauthorizedInvalidToken
		}
		return nil, err
	}
	if authn == nil {
		//nolint:nilnil // Authenticate が nil,nil を返した場合は未提供として扱い、呼び出し側で ErrUnauthorizedTokenNotProvided へ変換する
		return nil, nil
	}

	if !resolveIdentity {
		return authn, nil
	}

	// 認証済みの外部アイデンティティ（issuer + subject）を内部ユーザーへ解決する。
	resolved, err := resolver.Resolve(ctx, authn)
	if err != nil {
		return nil, err
	}
	if resolved == nil {
		// resolver は成功時に非 nil の Authn を返す契約。防御的に未認証として扱う。
		return nil, authbd.ErrIdentityNotFound
	}

	return resolved, nil
}

// extractBearerToken は、Authorization ヘッダから Bearer トークンを抽出します（ヘッダ名・スキームは RFC 6750 により固定）。
// Authorization: Bearer <token> 形式のときだけ scheme=SchemeBearer と token を返し、
// それ以外は scheme/token とも空を返します。
func extractBearerToken(r *http.Request) (string, string) {
	raw := strings.TrimSpace(r.Header.Get(echo.HeaderAuthorization))
	if raw == "" {
		return "", ""
	}
	after, ok := strings.CutPrefix(raw, prefixBearer)
	if !ok {
		return "", ""
	}
	return authbd.SchemeBearer, strings.TrimSpace(after)
}
