package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"go-boilerplate/pkg/xerrors"
)

// appServicesKey は、起動対象サービスを宣言する compose.mk の変数名です。
const appServicesKey = "APP_SERVICES"

var (
	// errViolation は、宣言された規則に反するサービスが見つかったことを表す。
	errViolation = xerrors.New("compose service declaration check failed")
	// errNoService は、compose ファイルがサービスを 1 つも宣言していないことを表す。
	errNoService = xerrors.New("compose file declares no service")
	// errNoAppServices = 起動対象の宣言が読めなかったことを表す。
	errNoAppServices = xerrors.New("no " + appServicesKey + " assignment found in")
	// errUnknownService は、起動対象に挙がっているサービスが compose に無いことを表す。
	errUnknownService = xerrors.New("service is listed in " + appServicesKey + " but absent from the compose file")
)

// service は、この検査が見る範囲のサービス宣言です。compose の他のキーは読みません。
type service struct {
	Healthcheck *healthcheck `yaml:"healthcheck"`
}

// healthcheck は、判定に使う healthcheck の宣言です。test が空なら宣言が無いのと変わりません。
type healthcheck struct {
	Test yaml.Node `yaml:"test"`
}

// composeFile は、compose ファイルのうち services だけを取り出す入れ物です。
type composeFile struct {
	Services map[string]service `yaml:"services"`
}

// loadServices は、compose ファイルからサービス宣言を読み出します。
//
// サービスが 0 件のときエラーにするのは、この検査が「何も見ていない」状態を成功と
// 報告しないためです。ファイルの場所を間違えても YAML としては正しく読めてしまいます。
func loadServices(path string) (map[string]service, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, xerrors.Wrap(err, "failed to read "+path)
	}

	var parsed composeFile
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return nil, xerrors.Wrap(err, "failed to parse "+path)
	}

	if len(parsed.Services) == 0 {
		return nil, xerrors.Wrap(errNoService, path)
	}

	return parsed.Services, nil
}

// loadAppServices は、compose.mk の APP_SERVICES 宣言から起動対象サービス名を読み出します。
//
// 対象の一覧をこのツールに持たないのは、両方に置くと片方だけを直したときに黙ってずれるためです。
// 宣言が見つからなければエラーにします。空の一覧で走らせると、検査対象ゼロのまま成功します。
func loadAppServices(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, xerrors.Wrap(err, "failed to read "+path)
	}

	for line := range strings.Lines(string(raw)) {
		value, ok := appServicesValue(line)
		if !ok {
			continue
		}

		names := strings.Fields(value)
		if len(names) == 0 {
			return nil, xerrors.Wrap(errNoAppServices, path)
		}

		return names, nil
	}

	return nil, xerrors.Wrap(errNoAppServices, path)
}

// appServicesValue は、行が APP_SERVICES の代入なら右辺を返します。
// `?=` と `=` と `:=` を受けるのは、make のどの代入でも宣言としては同じ意味を持つためです。
func appServicesValue(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)

	rest, found := strings.CutPrefix(trimmed, appServicesKey)
	if !found {
		return "", false
	}

	rest = strings.TrimLeft(rest, " \t")
	for _, op := range []string{"?=", ":=", "="} {
		if value, ok := strings.CutPrefix(rest, op); ok {
			return strings.TrimSpace(value), true
		}
	}

	return "", false
}

// checkHealthcheck は、起動対象サービスの healthcheck 宣言を検査し、違反の報告文を返します
// （違反が無ければ空文字）。
//
// 起動対象に挙がっているサービスが compose に無い場合は違反ではなくエラーにします。名前が
// ずれているのに「healthcheck を持つサービスは全部持っていた」と報告するのは、見ていないのと同じです。
func checkHealthcheck(appServices []string, services map[string]service) (string, error) {
	missing := make([]string, 0, len(appServices))

	for _, name := range appServices {
		svc, ok := services[name]
		if !ok {
			return "", xerrors.Wrap(errUnknownService, name)
		}

		if svc.Healthcheck == nil || svc.Healthcheck.Test.IsZero() {
			missing = append(missing, name)
		}
	}

	if len(missing) == 0 {
		return "", nil
	}

	sort.Strings(missing)

	return fmt.Sprintf(
		"app 層のサービスに healthcheck が宣言されていません: %s\n"+
			"  make serve は healthcheck を --wait で待って起動の成否を判定します"+
			"（.makefiles/app/server.mk の app-up）。宣言が無いサービスは、"+
			"コンテナが起動したことだけで成功と見なされます。",
		strings.Join(missing, " "),
	), nil
}
