package core

import (
	"context"

	"go-boilerplate/internal/config"
	"go-boilerplate/internal/controller/httpstack/oapi/auth"
	"go-boilerplate/internal/infrastructure/auth/jwt"
	"go-boilerplate/internal/infrastructure/auth/local"
	"go-boilerplate/internal/infrastructure/httpclient"

	// sample-api:replace-begin
	"go-boilerplate/internal/infrastructure/auth/useridentity"
	// sample-api:replace-with
	// = "go-boilerplate/internal/infrastructure/auth/identity"
	// sample-api:replace-end
	"go-boilerplate/internal/logging"
	"go-boilerplate/internal/observability"

	authbd "go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/internal/usecase/boundary/clock"

	"go-boilerplate/pkg/xerrors"

	"go.uber.org/fx"
)

// callerSkipCount は、ロギングラッパーが追加するフレーム数を補正するためのスキップ数です。
const callerSkipCount = 1

// accessTokenType は、access token に期待する RFC 9068 の typ ヘッダ値です。
// 根拠と ID Token 誤用拒否の扱いは docs/design/auth.md を参照してください。
const accessTokenType = "at+jwt"

// errNoAuthenticatorForEnv は、現在の環境に対応する Authenticator が無く配線に失敗した場合のエラーです。
var errNoAuthenticatorForEnv = xerrors.New("no authenticator configured for environment")

// authenticatorParams は、provideAuthenticator の依存を集約する fx パラメータです。
type authenticatorParams struct {
	fx.In

	AppCfg      *config.ApplicationConfig
	AuthCfg     *config.AuthConfig
	EndpointCfg *config.EndpointConfig
	Clock       clock.Clock
	Logger      logging.Logger
	HTTPClient  httpclient.Client
	TracerFtry  observability.TracerFactory
	Lifecycle   fx.Lifecycle
}

// AuthnModule は、認証関連の依存関係（Authenticator・IdentityResolver・Auth コントローラ）を提供するfxモジュールを返します。
func AuthnModule() fx.Option {
	return fx.Module(
		"core.authn",
		fx.Provide(
			provideAuthenticator,
			// sample-api:replace-begin
			useridentity.New,
			// sample-api:replace-with
			// = identity.New,
			// sample-api:replace-end
			fx.Annotate(auth.NewAuthenticator, fx.ParamTags("", "", `group:"`+auth.SchemeGroup+`"`)),
		),
	)
}

// provideAuthenticator は、環境に対応した Authenticator を選んで返します。
// 対応する case が無い環境は誤った Authenticator を配線しないよう起動エラーにします（fail-closed）。
func provideAuthenticator(p authenticatorParams) (authbd.Authenticator, error) {
	logger := p.Logger.Named("core.authn").CallerSkip(callerSkipCount)

	switch p.AppCfg.Env() {
	case config.EnvCI, config.EnvTest:
		logger.Warn(
			context.Background(),
			"Local authenticator wired: authentication is stubbed (non-production only)",
			logging.String("env", p.AppCfg.Env()),
		)

		return local.New(), nil
	case config.EnvLocal, config.EnvDast, config.EnvDevelopment:
		return provideJWKSAuthenticator(p, logger)
	default:
		logger.Error(
			context.Background(),
			"No authenticator configured for the current environment",
			logging.String("env", p.AppCfg.Env()),
		)

		return nil, xerrors.Wrap(errNoAuthenticatorForEnv, p.AppCfg.Env())
	}
}

// allowInsecureJWKSURL は、指定環境で JWKS URL に非 https を許すかを返します。
// 疑似 provider（http）へ繋ぐ環境だけが対象で、それ以外は https を強制します。
func allowInsecureJWKSURL(env string) bool {
	switch env {
	case config.EnvLocal, config.EnvDast:
		return true
	default:
		return false
	}
}

// provideJWKSAuthenticator は、AUTH_* 設定から JWKS backed の JWT authenticator を構築します。
// 取得はリクエストから切り離して走るため、停止時に取り残さないよう完了待ちを lifecycle へ登録します（遅延取得）。
func provideJWKSAuthenticator(p authenticatorParams, logger logging.Logger) (authbd.Authenticator, error) {
	authenticator, err := jwt.NewJWKS(jwt.JWKSParams{
		Issuer:             p.AuthCfg.Issuer(),
		Audience:           p.AuthCfg.Audience(),
		AllowedAlgs:        p.AuthCfg.AllowedAlgorithms(),
		Leeway:             p.AuthCfg.ClockSkew(),
		ExpectedType:       accessTokenType,
		Clock:              p.Clock,
		JWKSURL:            p.EndpointCfg.JWKS(),
		CacheTTL:           p.AuthCfg.JWKSCacheTTL(),
		DiscoveryTTL:       p.AuthCfg.DiscoveryTTL(),
		UnknownKidCooldown: p.AuthCfg.UnknownKidCooldown(),
		AllowInsecureURL:   allowInsecureJWKSURL(p.AppCfg.Env()),
		TracerFactory:      p.TracerFtry,
	}, p.HTTPClient)
	if err != nil {
		logger.Error(
			context.Background(),
			"Failed to wire JWKS JWT authenticator",
			logging.String("env", p.AppCfg.Env()),
			logging.Error("error", err),
		)

		return nil, err
	}

	if d, ok := authenticator.(jwt.Drainer); ok {
		p.Lifecycle.Append(fx.Hook{OnStop: d.Drain})
	}

	logger.Info(
		context.Background(),
		"JWKS JWT authenticator wired",
		logging.String("env", p.AppCfg.Env()),
		logging.String("issuer", p.AuthCfg.Issuer()),
	)

	return authenticator, nil
}
