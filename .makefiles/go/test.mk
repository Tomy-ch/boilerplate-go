## Go言語のテスト関連のコマンド群
.PHONY: go-test ## CI用のテスト実行（キャッシュ無効）
.PHONY: go-test-cached ## ローカル用テスト実行（キャッシュ有効・pre-commit向け）
.PHONY: go-test-arch ## アーキテクチャテストだけを実行（DB 不要・約 1 秒）
.PHONY: gen-test-repo ## テストの実行とテストレポートの生成
.PHONY: go-test-cover-ci ## CI用のカバレッジ付きテスト実行
.PHONY: cover-gate ## 総カバレッジが閾値以上か検証（CIゲート）
.PHONY: go-test-scripts ## CI用の scripts 配下ツールのテスト実行（キャッシュ無効）
.PHONY: go-test-scripts-cached ## ローカル用の scripts 配下ツールのテスト実行（キャッシュ有効・pre-commit向け）
.PHONY: cover-scripts ## scripts 配下ツールの総カバレッジを計測し、下限割れを警告する（失敗させない）
.PHONY: build-scripts ## scripts 配下ツールを scripts/bin/ へビルドする（手元で実バイナリを動かす用）
.PHONY: go-test-fails ## go test のログから失敗だけを抜き出す（LOG= でログ指定、LOG=- で標準入力）

# カバレッジ対象外パッケージ（test / go-test-cached / gen-test-repo / go-test-cover-ci で共有）
#
# node_modules を外すのは、npm が展開する依存ツリーに Go 実装を同梱するパッケージがあり
# （`flatted` の golang/）、`go list ./...` がそれを本体のパッケージとして数えてしまうため。
# 第三者のコードなので母数に入れるとカバレッジが理由なく動く。
GO_TEST_EXCLUDE := /(gen|cmd|mock|apperror|scripts|node_modules)(/|$$)

# カバレッジゲートの下限（docs/rules.md の 90% フロア）。対象は boilerplate 本体
# （internal / pkg）で、GO_TEST_EXCLUDE が scripts を母数から外している。
COVERAGE_THRESHOLD := 90

# scripts/ 配下の開発ツールの下限。本体とは別の数値を別のプロファイルに対して張る。
# ここが本体と同じ 1 本のゲートに乗ると、出荷物と無関係なツールの劣化がマージを止める。
SCRIPTS_COVERAGE_THRESHOLD := 95

# DB を使うテストはテスト用 DB の seed を fixture として読む。その seed の issuer はスロットのポートに
# 追従する（.makefiles/database/seed.mk）ため、host 実行の go test にも同じ値を渡す。
GO_TEST_ENV = $(LOAD_BAND); $(LOAD_SLOT); $(DB_SLOT_ENV); export AUTH_ISSUER; \
	if [ -n "$$GO_TEST_LOAD_ENV" ]; then export $$GO_TEST_LOAD_ENV; fi;

# ホスト実行の go test は DB_NAME_TEST を見て接続先を決める（internal/config/config_testing_mock.go）。
# 未設定なら共有 test へ落ちるため、スロット未取得の worktree では require-db-owner で止める
# （不変条件は .makefiles/database/pool.mk）。CI 用の go-test-cover-ci は CI 側で DB を用意するため対象外。
go-test: require-db-owner
	@$(GO_TEST_ENV) TGT_PKGS="$$(go list ./... | grep -Ev '$(GO_TEST_EXCLUDE)')"; \
	$$GOBP_NICE go test $$TGT_PKGS -race -cover -count=1 $$GO_TEST_P_FLAG

go-test-cached: require-db-owner
	@$(GO_TEST_ENV) TGT_PKGS="$$(go list ./... | grep -Ev '$(GO_TEST_EXCLUDE)')"; \
	$$GOBP_NICE go test $$TGT_PKGS -cover $$GO_TEST_P_FLAG

# 実装中の反復用。DB を要さず約 1 秒で終わるので、go-lint-fast と対で checkpoint に回せる。
# depguard が持てない検査（集約間 import の隔離など）はここが受け持つ。
go-test-arch:
	@$(LOAD_BAND); $$GOBP_NICE go test ./internal/architest/... -count=1

gen-test-repo: require-db-owner
	@echo "🔄 テストを実行し、レポートを生成します..."
	go clean -testcache
	rm -f docs/coverage/coverage.out
	$(GO_TEST_ENV) TGT_PKGS="$$(go list ./... | grep -Ev '$(GO_TEST_EXCLUDE)')"; \
	COVER_PKGS="$$(go list ./... \
		| grep -Ev '$(GO_TEST_EXCLUDE)' \
		| tr '\n' ',' \
		| sed 's/,$$//')"; \
	go test $$TGT_PKGS -coverpkg=$$COVER_PKGS -coverprofile=docs/coverage/coverage.out -covermode=set
	go tool cover -html=docs/coverage/coverage.out -o docs/coverage/index.html
	rm -f docs/coverage/coverage.out
	@echo "✅ テストレポートの生成が完了しました。"

go-test-cover-ci:
	@$(GO_TEST_ENV) TGT_PKGS="$$(go list ./... | grep -Ev '$(GO_TEST_EXCLUDE)')"; \
	COVER_PKGS="$$(go list ./... \
		| grep -Ev '$(GO_TEST_EXCLUDE)' \
		| tr '\n' ',' \
		| sed 's/,$$//')"; \
	go test $$TGT_PKGS -race -coverpkg=$$COVER_PKGS -coverprofile=coverage.out -covermode=atomic -count=1

# scripts 配下の開発ツールは GO_TEST_EXCLUDE でカバレッジ母数から外れており、そのままでは
# test / go-test-cached のいずれにも乗らない。ツール自体がゲート（供給網ピン・lint）なので、
# 壊れ方が「静かに何も検査しなくなる」方向に出る。カバレッジ計測とは切り離して実行だけを足す。
go-test-scripts:
	@$(LOAD_BAND); $$GOBP_NICE go test ./scripts/... -race -count=1 $$GO_TEST_P_FLAG

go-test-scripts-cached:
	@$(LOAD_BAND); $$GOBP_NICE go test ./scripts/... $$GO_TEST_P_FLAG

# 出力先を scripts/bin/ に固定する。`go build ./scripts/<tool>` をリポジトリ直下で叩くと
# パッケージ名の実行ファイルがルートへ落ち、追跡対象外のまま数十 MB 残る。-o の付け忘れが
# 起きないようターゲットへ寄せ、置き場所ごと .gitignore で無視する。
build-scripts:
	@echo "🧰 scripts 配下のツールを scripts/bin/ へビルドします..."
	@go build -o scripts/bin/ ./scripts/...
	@echo "✅ ビルドが完了しました（scripts/bin/）。"

# 判定は scripts/cover-gate（テストの当たる Go 側）が持つ。ここが渡すのはしきい値だけで、
# 下限値そのものは docs/rules.md に紐づく設定なので make 側に残す。
cover-gate:
	@go run ./scripts/cover-gate -profile coverage.out -threshold $(COVERAGE_THRESHOLD)

# scripts 配下の計測は本体とはプロファイルを分ける。合流させると片方の劣化がもう片方の
# 合否を動かすため、下限割れは -warn で警告に留める（CI では GITHUB_ACTIONS が立つので
# ::warning:: アノテーションとして差分ビューに出る）。
#
# go test の出力は捨てないこと。-coverprofile はファイルへ書くため、標準出力に残るのは
# サマリ行と、失敗したときのテスト名・差分だけで、後者はこのターゲットが落ちた理由を示す
# 唯一の手掛かりになる。
cover-scripts:
	@$(LOAD_BAND); $$GOBP_NICE go test ./scripts/... -coverprofile=coverage-scripts.out -covermode=atomic -count=1 $$GO_TEST_P_FLAG
	@go run ./scripts/cover-gate -profile coverage-scripts.out -threshold $(SCRIPTS_COVERAGE_THRESHOLD) \
		-warn $(if $(GITHUB_ACTIONS),-github,)
	@rm -f coverage-scripts.out

# go test のログから、成功しか報告していない行を落とす。何を落とすか・なぜ捨てて安全かは
# .makefiles/README.md の go-test-fails 行が所管する。ここに残すのは各段の前提だけ。
#
# カバレッジ行は 3 つの形で出る（-coverpkg へ渡した全リストが報告ごとに行末へ複製される）。
#   1. `ok  <pkg> <時間>  coverage: X% ... in <全リスト>`   … 通過。ok で落ちる
#   2. `coverage: X% ... in <全リスト>`                      … 失敗パッケージぶん。単独行
#   3. `<TAB><pkg><TAB><TAB>coverage: 0.0% of statements`   … テストの無いパッケージ
# 落ちたときに残るのは 2 と 3 なので、行頭の ok / ? だけを見ていると最大の行が素通りする。
#
# 1 つ目の sed は `gh run view --log` の行接頭辞を剥がす。剥がさないと行頭アンカーが外れる。
# 時刻手前の `[^0-9]*` は BOM を吸うため。ローカルの go test 出力は 2 つ目の TAB の後が日付に
# ならないので当たらない。`of statements in <リスト>` の畳み込みは捨て漏れたときの保険で、
# 絶対パスはリポジトリ相対へ縮めるが、モジュール接頭辞は失敗箇所の手掛かりなので残す。
#
# grep の -a は必須。NUL が 1 バイト混ざると grep はバイナリとみなし、何も出さずに終わる。
# 失敗行が無いときの報せは stderr へ出す。呼び出し側が `> file` で受けて空判定できるように。
LOG ?= $(AI_LOG_DIR)/test.txt

go-test-fails:
	@out="$$(cat $(LOG) \
		| sed -E -e 's|^[^	]*	[^	]*	[^0-9]*[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:.]+Z ||' \
		| sed -e 's|$(CURDIR)/||g' -e 's|\(of statements\) in .*|\1 in ...|' \
		| grep -avE '^(ok|\?)[[:space:]]' \
		| grep -avE 'coverage: [0-9.]+% of statements')"; \
	if [ -z "$$out" ]; then echo "✅ 失敗行はありません（$(LOG)）" >&2; else printf '%s\n' "$$out"; fi
