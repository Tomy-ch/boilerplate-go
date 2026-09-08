// Package idempotency は、冪等性の入り口 middleware（oapi-codegen StrictMiddleware スロット用）を提供します。
package idempotency

import (
	"crypto/sha256"
	"encoding/json"
	"strings"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/controller/ctxhelper"
	idempotencyuc "go-boilerplate/internal/usecase/idempotency"
	"go-boilerplate/pkg/xerrors"

	"github.com/labstack/echo/v5"
)

// headerName は、冪等性キーのリクエストヘッダ名です。
const headerName = "Idempotency-Key"

// maxKeyLength は、Idempotency-Key ヘッダの最大バイト長です。
const maxKeyLength = 255

// NextFunc は、StrictHandlerFunc と構造的に同一の handler 呼び出しシグネチャです。
type NextFunc func(ctx *echo.Context, request any) (any, error)

// Middleware は、冪等性入り口を StrictMiddleware の構造的シグネチャで返します。
func Middleware() func(next NextFunc, operationID string) NextFunc {
	return func(next NextFunc, operationID string) NextFunc {
		return func(ec *echo.Context, request any) (any, error) {
			return handle(ec, request, operationID, next)
		}
	}
}

// StrictMiddleware は、Middleware() を oapi-codegen のパッケージ固有 StrictMiddlewareFunc 形へ
// 適合させたアダプタを返します。
func StrictMiddleware[H ~func(ec *echo.Context, request any) (any, error)]() func(f H, operationID string) H {
	core := Middleware()
	return func(f H, operationID string) H {
		return H(core(NextFunc(f), operationID))
	}
}

func handle(ec *echo.Context, request any, operationID string, next NextFunc) (any, error) {
	r := ec.Request()
	key := strings.TrimSpace(r.Header.Get(headerName))
	if key == "" {
		// ヘッダ無し = 非冪等（既存挙動のまま素通し）。
		return next(ec, request)
	}
	if err := validateKey(key); err != nil {
		return nil, err
	}

	// 未認証なら安全側で素通しする。
	authn, ok := ctxhelper.GetAuthn(r.Context())
	if !ok {
		return next(ec, request)
	}

	fp, err := fingerprint(r.Method, r.URL.Path, request)
	if err != nil {
		return nil, xerrors.Join(apperror.ErrInternal, xerrors.Wrap(err, "failed to fingerprint idempotent request"))
	}

	reqCtx := idempotencyuc.WithRequest(r.Context(), idempotencyuc.Request{
		// スコープは認証主体そのもの（ADR-0063 (idempotency-scope-required)）。内部ユーザー ID ではない。
		Scope:       authn.Subject(),
		Key:         key,
		Fingerprint: fp,
		Method:      r.Method,
		Path:        r.URL.Path,
		OperationID: operationID,
	})
	ec.SetRequest(r.WithContext(reqCtx))

	return next(ec, request)
}

// validateKey は、Idempotency-Key の健全性を検証します（≤255 / 印字可能 ASCII）。違反は 400。
func validateKey(key string) error {
	if len(key) > maxKeyLength {
		return xerrors.Wrap(apperror.ErrInvalidArgument, "Idempotency-Key must be 255 characters or fewer")
	}
	for _, c := range []byte(key) {
		if c < 0x21 || c > 0x7E {
			return xerrors.Wrap(apperror.ErrInvalidArgument, "Idempotency-Key must contain only printable ASCII characters")
		}
	}
	return nil
}

// fingerprint は、method + path + typed request の決定的 marshal を SHA-256 した指紋を返します。
// request の marshal に失敗した場合は弱い指紋を作らずエラーを返します（fail-closed）。
func fingerprint(method, path string, request any) ([]byte, error) {
	b, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	h.Write([]byte(method))
	h.Write([]byte{'\n'})
	h.Write([]byte(path))
	h.Write([]byte{'\n'})
	h.Write(b)
	return h.Sum(nil), nil
}
