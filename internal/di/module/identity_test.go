package module

import (
	"testing"

	authbd "go-boilerplate/internal/usecase/boundary/auth"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

func Test_identityModule(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("IdentityRegistrar が解決できる", func(t *testing.T) {
			t.Parallel()

			// 型を要求しないと fx.ValidateApp はコンストラクタを検査しないため、Populate で名指しする。
			var registrar authbd.IdentityRegistrar
			validateGraph(t, append(commonDeps(), identityModule(), fx.Populate(&registrar))...)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未配線では IdentityRegistrar が解決できずグラフ検証に失敗する", func(t *testing.T) {
			t.Parallel()

			var registrar authbd.IdentityRegistrar
			opts := append(commonDeps(), fx.Populate(&registrar), fx.NopLogger)
			require.Error(t, fx.ValidateApp(opts...))
		})
	})
}
