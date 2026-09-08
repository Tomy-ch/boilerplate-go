## Go言語のテスト関連のコマンド群
.PHONY: test ## CI用のテスト実行（キャッシュ無効）
.PHONY: test-cached ## ローカル用テスト実行（キャッシュ有効・pre-commit向け）
.PHONY: gen-test-repo ## テストの実行とテストレポートの生成
.PHONY: test-cover-ci ## CI用のカバレッジ付きテスト実行
.PHONY: cover-gate ## 総カバレッジが閾値以上か検証（CIゲート）
.PHONY: test-scripts ## CI用の scripts 配下ツールのテスト実行（キャッシュ無効）
.PHONY: test-scripts-cached ## ローカル用の scripts 配下ツールのテスト実行（キャッシュ有効・pre-commit向け）
.PHONY: cover-scripts ## scripts 配下ツールの総カバレッジを計測し、下限割れを警告する（失敗させない）
.PHONY: build-scripts ## scripts 配下ツールを scripts/bin/ へビルドする（手元で実バイナリを動かす用）
.PHONY: test-fails ## go test のログから失敗だけを抜き出す（LOG= でログ指定、LOG=- で標準入力）

# カバレッジ対象外パッケージ（test / test-cached / gen-test-repo / test-cover-ci で共有）
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
# （不変条件は .makefiles/database/pool.mk）。CI 用の test-cover-ci は CI 側で DB を用意するため対象外。
test: require-db-owner
	@$(GO_TEST_ENV) TGT_PKGS="$$(go list ./... | grep -Ev '$(GO_TEST_EXCLUDE)')"; \
	$$GOBP_NICE go test $$TGT_PKGS -race -cover -count=1 $$GO_TEST_P_FLAG

test-cached: require-db-owner
	@$(GO_TEST_ENV) TGT_PKGS="$$(go list ./... | grep -Ev '$(GO_TEST_EXCLUDE)')"; \
	$$GOBP_NICE go test $$TGT_PKGS -cover $$GO_TEST_P_FLAG

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

test-cover-ci:
	@$(GO_TEST_ENV) TGT_PKGS="$$(go list ./... | grep -Ev '$(GO_TEST_EXCLUDE)')"; \
	COVER_PKGS="$$(go list ./... \
		| grep -Ev '$(GO_TEST_EXCLUDE)' \
		| tr '\n' ',' \
		| sed 's/,$$//')"; \
	go test $$TGT_PKGS -race -coverpkg=$$COVER_PKGS -coverprofile=coverage.out -covermode=atomic -count=1

# scripts 配下の開発ツールは GO_TEST_EXCLUDE でカバレッジ母数から外れており、そのままでは
# test / test-cached のいずれにも乗らない。ツール自体がゲート（供給網ピン・lint）なので、
# 壊れ方が「静かに何も検査しなくなる」方向に出る。カバレッジ計測とは切り離して実行だけを足す。
test-scripts:
	@$(LOAD_BAND); $$GOBP_NICE go test ./scripts/... -race -count=1 $$GO_TEST_P_FLAG

test-scripts-cached:
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

# go test のログから、診断に要らない行を落として表示する。
#
# `-v` は付けていないので出力は 1 パッケージ 1 行になる。テスト対象は 266 パッケージあり、
# 1 件落ちたときでも通過分の `ok` 行が 265 行ぶら下がってくる。読む側（特にエージェント）は
# その全部をコンテキストへ載せることになるので、通過行と no test files 行だけを捨てる。
#
# 落とすのは「通った」ことしか言っていない行に限る。許可リストにして失敗行の形を列挙すると、
# 想定外の壊れ方（panic / build failed / race detector の報告）が黙って消える。捨てる側を
# 列挙するほうが、知らない出力は素通しされるぶん安全。元ログはディスクに残る。
#
# カバレッジ行は 3 つの形で出る。test-cover-ci / gen-test-repo は -coverpkg に対象パッケージを
# カンマ連結して渡すため（このリポジトリでは 13KB 超の 1 引数）、go test はカバレッジを報告する
# たびにその全リストを `of statements in ...` として行末へ複製する。
#   1. `ok  <pkg> <時間>  coverage: X% of statements in <全リスト>`  … 通過。ok で落ちる
#   2. `coverage: X% of statements in <全リスト>`                     … 失敗パッケージぶん。単独行
#   3. `<TAB><pkg><TAB><TAB>coverage: 0.0% of statements`             … テストの無いパッケージ
# 落ちたときに残るのは 2 と 3 なので、行頭の ok / ? だけを見ていると最大の行が素通りする。
# カバレッジしか言っていない行はまとめて捨てる。総量の判定は cover-gate が profile から行う
# ので、ここで数値を残す意味はない。
#
# 最初の sed は `gh run view --log` が各行へ付ける `<job>TAB<step>TAB<時刻> ` を剥がす。剥がさないと
# 行頭アンカーが外れて ok 行が残るうえ、接頭辞そのものが 1 行 50 バイト前後を占める（実測の CI
# ログでは絞り込み後の 553KB のうち 211KB）。時刻の手前の `[^0-9]*` は BOM を吸うため。ローカルの
# go test 出力にも TAB はあるが、2 つ目の TAB の後が日付にならないので当たらない。
#
# grep の -a は必須。ログに NUL が 1 バイトでも混ざると grep はバイナリとみなし、一致内容を
# 出さずに終わる。落ちた回ほど壊れた出力が混ざりやすく、そこで黙って空になるのが一番まずい。
#
# そのうえで、生き残った行に対しても `of statements in <リスト>` を畳む。捨て漏れが出ても
# 1 行が 13KB に膨らまないための保険で、削除ではなく省略なので判断材料は失われない。
#
# 絶対パスはリポジトリ相対へ縮める。testify の Error Trace はフルパスで出るため、worktree だと
# 1 行の大半がプレフィックスで埋まる。モジュール接頭辞（go-boilerplate/）のほうは残す。
# ok 行が消えた後に残るのは FAIL 数行で、そこはパッケージパスが失敗箇所の手掛かりになる。
#
# 残る行が無いときの報せは stderr へ出す。呼び出し側が `> file` で受けたときに空ファイルとなり、
# 「中身が無い＝失敗行が無い」の判定がそのまま効く。CI のコメント生成がこの形で読む。
#
# LOG= で対象を差し替える。既定は `make ai-test` が残すログ。CI のログにも同じ絞り込みが要る
# ので（`gh run view --log-failed` はステップ単位で、テストの全パッケージ行を含む）、
# LOG=- で標準入力を読む: gh run view --log-failed | make test-fails LOG=-
LOG ?= $(AI_LOG_DIR)/test.txt

test-fails:
	@out="$$(cat $(LOG) \
		| sed -E -e 's|^[^	]*	[^	]*	[^0-9]*[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:.]+Z ||' \
		| sed -e 's|$(CURDIR)/||g' -e 's|\(of statements\) in .*|\1 in ...|' \
		| grep -avE '^(ok|\?)[[:space:]]' \
		| grep -avE 'coverage: [0-9.]+% of statements')"; \
	if [ -z "$$out" ]; then echo "✅ 失敗行はありません（$(LOG)）" >&2; else printf '%s\n' "$$out"; fi
