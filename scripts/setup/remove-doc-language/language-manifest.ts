// 言語を畳む撤去で、機械規則に載らないものを宣言する manifest（データ定義）。
// 規則は doc-language.ts、計画は plan.ts、実行は index.ts を参照。
//
// 対訳ペアそのものは列挙しない。ペアは `<name>.md` と `<name>.ja.md` の隣接という規則で決まり、
// 422 組を書き並べても増減のたびに宣言が腐るだけである。ここに書くのは規則で決まらないもの
// —— 対訳の存在が主題であるがゆえに撤去ごと必要になるツールと、機械では畳めない散文だけ。

import { type Mode } from "./doc-language";

/** `--lang` が受け付ける値。 */
export const MODES: readonly (Mode | "both")[] = ["en", "ja", "both"];

/** 撤去のコミット件名。commitlint の type-enum に合わせる。 */
export const COMMIT_SUBJECTS: Readonly<Record<Mode | "both", string>> = {
  en: "Docs: 日本語の対訳を撤去し英語 1 本へ畳む",
  ja: "Docs: 英語正本を撤去し日本語 1 本へ畳む",
  both: "Docs: 英日の両方を残すと決め、言語選択のマーカーを解決する",
};

/**
 * 対訳ペアの存在そのものが主題で、畳んだ後は主語を失うもの。
 *
 * @remarks
 * `canonicalize-doc` は正本と対訳の組を作って同期するスキルです。組が無くなれば、この
 * スキルが何をするのかを言える人はいなくなります。散文を削って残す道もありますが、
 * 削り終えた後に残るのは「1 つの文書を 1 つの文書へ同期するスキル」で、それは機能ではありません。
 *
 * ポータルの言語フィルタも同じです。畳んだ後の `docs.json` は 1 言語しか持たないため、
 * 切り替え先の無い切り替えボタンだけが残ります。
 */
export const REMOVED_PATHS: readonly string[] = [
  // ポータルの日本語複製。manifest から作られる生成物だが、作られなくなるだけでは消えない。
  "docs/portal/guides/ja",
  ".claude/skills/canonicalize-doc",
  ".codex/skills/canonicalize-doc",
  "docs-viewer/src/lang-filter",
];

/**
 * 撤去し終えた後に自分自身を落とす対象。
 *
 * @remarks
 * 一度きりの操作なので、ツールと、それを呼ぶ make ターゲットの宣言と、それを叩く CI を
 * 一緒に落とします。作成先のリポジトリに残しても、叩けば「畳むものが無い」で失敗するだけです。
 * make ターゲット側の宣言とレシピは `doc-pair` マーカーで同時に落ちます。
 */
export const SELF_DESTRUCT_PATHS: readonly string[] = [
  ".github/workflows/doc-language-removal-check.yaml",
  "scripts/setup/remove-doc-language",
];

/** egress SSOT のパス。自消滅する workflow の宣言はここから落とす。 */
export const EGRESS_FILE = ".github/egress.toml";

/**
 * 自消滅する workflow が egress SSOT に持つジョブ宣言。
 *
 * @remarks
 * 落とす契機は言語の選択ではなく workflow の消滅なので、`both` を含む 3 モードで落とします。
 * マーカーでは表せません —— `both` はマーカーの中身を残す向きに解決するため、宣言だけが
 * 対応する workflow を失い、`make egress-check` が孤児として鳴きます。
 */
export const SELF_EGRESS_JOBS: readonly string[] = [
  "doc-language-removal-check.yaml:doc-language-removal-check",
];

/** 行は残したまま、文字列だけを差し替える宣言。 */
export type DocReplacement = {
  file: string;
  from: string;
  to: string;
  /**
   * この宣言が効くモード。省略すると両方で効く。
   *
   * @remarks
   * 大半の宣言は両モードで同じですが、`ja` では改名によって「同じパスの中身が別の言語になる」
   * ため、そのパスへ完全一致で書き込む宣言は言語ごとに別物になります。`remove-licensed-scanners`
   * が `.github/workflows/README.md` の英文を宣言しているのがその一例で、`ja` を選ぶと同じ
   * パスに日本語が入り、宣言は当たらなくなります。
   */
  mode?: Mode;
};

/**
 * 1 ファイル分の差し替えをまとめて宣言する。
 *
 * @remarks
 * 宣言は 170 件を超え、その大半が同じファイルに対する複数の差し替えです。1 件ずつ書くと
 * パスが同じ回数だけ繰り返され、打ち間違えても型では気づけません。ファイルを 1 度だけ書く形に
 * まとめます。`to` を省いた組は削除（空文字への差し替え）です。
 */
function forFile(
  file: string,
  edits: readonly (readonly [from: string, to?: string])[],
  mode?: Mode,
): DocReplacement[] {
  return edits.map(([from, to = ""]) => (mode === undefined ? { file, from, to } : { file, from, to, mode }));
}

/**
 * 表のセルのように、行ごと落とせない場所の差し替え。
 *
 * @remarks
 * マーカーは自分の行にコメントを置くため、表の中では使えません（コメント行がそこで表を終わらせる）。
 * 表全体を `replace` で包む道も、`tool-map` の表が既に HTML コメントを 1 つ持っているため塞がって
 * います —— HTML コメントの退避コメントに HTML コメントは入れられず、最初の `-->` が外側を閉じます。
 *
 * 完全一致にしているのは、本文が動いたときに空振りではなく停止させるためです。差し替え先が
 * 見つからなければ撤去は止まり、宣言を直すまで進みません。
 */
export const DOC_REPLACEMENTS: readonly DocReplacement[] = [
  ...forFile(".claude/skills/tool-map/SKILL.md", [
    [" (skip `SKILL.ja.md` and other `*.ja.md` translation files)"],
    ["\n- `*.ja.md` translation files (they are not loaded as entries)."],
    ["\n- [ ] `*.ja.md` files excluded from the skills scan"],
  ]),
  ...forFile(".codex/skills/tool-map/SKILL.md", [
    [" (skip `SKILL.ja.md` and other `*.ja.md` translation files)"],
    ["\n- `*.ja.md` translation files (they are not loaded as entries)."],
    ["\n- [ ] `*.ja.md` files excluded from the skills scan"],
  ]),
  ...forFile(".github/workflows/README.md", [
    [" and its `README.ja.md` translation"],
    ["| `make md-lint` checks the pair, not the rows |", "| `make md-lint` does not check these rows |"],
  ]),
  ...forFile(".claude/skills/back-prop/SKILL.md", [
    [", minus `*.ja.md`"],
  ]),
  ...forFile(".codex/skills/back-prop/SKILL.md", [
    [", excluding `*.ja.md`"],
  ]),
  ...forFile(".claude/skills/commit/SKILL.md", [
    [", `*.ja.md`"],
  ]),
  ...forFile(".claude/skills/context-map/SKILL.md", [
    [" and its `.ja.md` pair"],
  ]),
  ...forFile(".codex/skills/context-map/SKILL.md", [
    [" and `docs/design/context-map.ja.md`"],
  ]),
  ...forFile(".claude/skills/glossary/SKILL.md", [
    ["、`.ja.md` ペアの作成"],
  ]),
  ...forFile(".codex/skills/glossary/SKILL.md", [
    [" Do not create a `.ja.md` pair for the glossary: this spec tree uses one Japanese file with English headings."],
  ]),
  ...forFile(".claude/skills/go-upgrade/SKILL.md", [
    [" / `README.ja.md`"],
    [" / `docker/README.ja.md`"],
    [" / `docker/server/README.ja.md`"],
    [" / `docker/tools/README.ja.md`"],
  ]),
  ...forFile(".codex/skills/go-upgrade/SKILL.md", [
    [" / `README.ja.md`"],
    [" / `docker/README.ja.md`"],
    [" / `docker/server/README.ja.md`"],
    [" / `docker/tools/README.ja.md`"],
  ]),
  ...forFile(".claude/skills/new-env/SKILL.md", [
    [", `env/README.ja.md`"],
    [" and `README.ja.md`"],
    [", env/README.ja.md"],
    [" then `env/README.ja.md`"],
    [" and `env/README.ja.md`"],
    ["; the skill translates for the other"],
    ["\nResolution rules:\n\n- If only Japanese provided → skill writes Japanese to `env/README.ja.md` row, then translates to English for `env/README.md` row.\n- If only English provided → reverse direction.\n- If both provided → use as-is, no translation.\n- Translations are kept short and direct (single-line, technical register matching surrounding rows). If the description is non-trivial or domain-specific, surface the proposed translation in the Step 2 plan summary for user review before writing.\n"],
  ]),
  ...forFile(".codex/skills/new-env/SKILL.md", [
    [", `env/README.ja.md`"],
    [" and `README.ja.md`"],
    [", env/README.ja.md"],
    [" then `env/README.ja.md`"],
    [" and `env/README.ja.md`"],
    ["; the skill translates for the other"],
    ["\nResolution rules:\n\n- If only Japanese provided → skill writes Japanese to `env/README.ja.md` row, then translates to English for `env/README.md` row.\n- If only English provided → reverse direction.\n- If both provided → use as-is, no translation.\n- Translations are kept short and direct (single-line, technical register matching surrounding rows). If the description is non-trivial or domain-specific, surface the proposed translation in the Step 2 plan summary for user review before writing.\n"],
  ]),
  ...forFile(".claude/skills/portal-manifest-sync/SKILL.md", [
    [" / `README.ja.md`"],
    ["\n- Japanese: `docs/portal/guides/ja/<flat-hyphenated-name>.ja.md`"],
    ["\n  - src: foo/bar/README.ja.md"],
    ["1. Read the English README content (and the `*.ja.md` sibling for completeness check; sibling existence already established via Step 2 preflight).", "1. Read the README content."],
  ]),
  ...forFile(".codex/skills/portal-manifest-sync/SKILL.md", [
    [" / `README.ja.md`"],
    ["\n- Japanese: `docs/portal/guides/ja/<flat-hyphenated-name>.ja.md`"],
    ["\n  - src: foo/bar/README.ja.md"],
    ["1. Read the English README content (and the `*.ja.md` sibling for completeness check; sibling existence already established via Step 2 preflight).", "1. Read the README content."],
  ]),
  ...forFile(".claude/skills/readme-review/SKILL.md", [
    ["\n- Cross-reference to translation (`README.ja.md`) — its existence and sync convention compliance"],
  ]),
  ...forFile(".codex/skills/readme-review/SKILL.md", [
    ["\n- Cross-reference to translation (`README.ja.md`) — its existence and sync convention compliance"],
  ]),
  ...forFile(".claude/skills/repo-ops/SKILL.md", [
    ["-g '!**/*.ja.md' "],
    ["Most of the Markdown in this tree is either a Japanese mirror you must not read or generated output\nthat lags the code, so a naive repo-wide search buries the one file that actually decides the answer.\nOf roughly 1,000 tracked `*.md`, **over 40% are `*.ja.md` translations** and **72 are generated\n`docs/portal/guides/**` copies of READMEs**; `docs/godoc/**` adds ~1,800 files and\n`docs/db-schema/**` ~400.", "Much of the Markdown in this tree is generated output that lags the code, so a naive repo-wide\nsearch buries the one file that actually decides the answer. Of the tracked `*.md`, **72 are\ngenerated `docs/portal/guides/**` copies of READMEs**; `docs/godoc/**` adds ~1,800 files and\n`docs/db-schema/**` ~400."],
    ["are *tracked*, so they need these explicit globs. Hitting a `*.ja.md` is still useful as a\n**locator** (it proves the topic is documented); read the English original beside it, per\n`AGENTS.md`'s rule never to read `*.ja.md`.", "are *tracked*, so they need these explicit globs."],
  ]),
  ...forFile(".codex/skills/repo-ops/SKILL.md", [
    ["-g '!**/*.ja.md' "],
    ["Most of the Markdown in this tree is either a Japanese mirror you must not read or generated output\nthat lags the code, so a naive repo-wide search buries the one file that actually decides the answer.\nOf roughly 1,000 tracked `*.md`, **over 40% are `*.ja.md` translations** and **72 are generated\n`docs/portal/guides/**` copies of READMEs**; `docs/godoc/**` adds ~1,800 files and\n`docs/db-schema/**` ~400.", "Much of the Markdown in this tree is generated output that lags the code, so a naive repo-wide\nsearch buries the one file that actually decides the answer. Of the tracked `*.md`, **72 are\ngenerated `docs/portal/guides/**` copies of READMEs**; `docs/godoc/**` adds ~1,800 files and\n`docs/db-schema/**` ~400."],
    ["are *tracked*, so they need these explicit globs. Hitting a `*.ja.md` is still useful as a\n**locator** (it proves the topic is documented); read the English original beside it, per\n`AGENTS.md`'s rule never to read `*.ja.md`.", "are *tracked*, so they need these explicit globs."],
  ]),
  ...forFile(".claude/skills/repo-truth/SKILL.md", [
    ["adversarial on top of that — 495 `*.ja.md` mirrors that must not be read and 144 generated copies", "adversarial on top of that — 144 generated copies"],
    ["1,000 tracked `*.md`, over 40% are `*.ja.md` translations that `AGENTS.md` forbids reading, and\n`docs/portal/**` / `docs/godoc/**` / `docs/db-schema/**` / `docs/openapi/**` / `docs/coverage/**` are\ngenerated copies that lag their sources.", "the tracked `*.md`, `docs/portal/**` / `docs/godoc/**` / `docs/db-schema/**` / `docs/openapi/**` /\n`docs/coverage/**` are generated copies that lag their sources."],
    ["Two things `repo-ops` section 0 establishes that this skill must not soften:\n\n- **Never read a `*.ja.md`.** Hitting one is still useful as a *locator* — it proves the topic is\n  documented — but read the English original beside it.\n- **Precedence when sources disagree**", "One thing `repo-ops` section 0 establishes that this skill must not soften:\n\n- **Precedence when sources disagree**"],
    ["- ❌ Read or cite a `*.ja.md`, or cite generated output (`docs/portal/**`, `docs/godoc/**`,\n  `docs/db-schema/**`, `docs/openapi/**`, `docs/coverage/**`) as authority.", "- ❌ Cite generated output (`docs/portal/**`, `docs/godoc/**`, `docs/db-schema/**`,\n  `docs/openapi/**`, `docs/coverage/**`) as authority."],
    ["- [ ] No `*.ja.md` read; generated trees excluded from search.", "- [ ] Generated trees excluded from search."],
  ]),
  ...forFile(".claude/skills/resolve-merge/SKILL.md", [
    ["- ❌ Hand-edit or side-pick any generated artifact, lockfile, or `.ja.md`.", "- ❌ Hand-edit or side-pick any generated artifact or lockfile."],
  ]),
  ...forFile(".codex/skills/repo-truth/SKILL.md", [
    ["It also carries 495 `*.ja.md` mirrors and 144 generated copies that must not be treated", "It also carries 144 generated copies that must not be treated"],
    ["Use keyword search only after the concern-owning indexes, as a final net. Never read a `*.ja.md`.", "Use keyword search only after the concern-owning indexes, as a final net."],
    ["- Do NOT read or cite `*.ja.md` or generated documentation as authority.", "- Do NOT cite generated documentation as authority."],
    ["- [ ] No `*.ja.md` read and generated trees excluded.", "- [ ] Generated trees excluded."],
  ]),
  ...forFile(".claude/skills/repo-truth/SKILL.ja.md", [
    ["`repo-ops` §0 が定めていて、このスキルが緩めてはならないことが 2 つある:\n\n- **`*.ja.md` を決して読まない。** ヒットすること自体は*位置特定*として有用だが —— その話題が文書化されている\n  証拠になる —— 読むのは隣にある英語原本である。\n- **出典が食い違ったときの優先順位**", "`repo-ops` §0 が定めていて、このスキルが緩めてはならないことが 1 つある:\n\n- **出典が食い違ったときの優先順位**"],
    ["- ❌ `*.ja.md` を読む・引用する、生成物（`docs/portal/**`、`docs/godoc/**`、`docs/db-schema/**`、\n  `docs/openapi/**`、`docs/coverage/**`）を権威として引用する。", "- ❌ 生成物（`docs/portal/**`、`docs/godoc/**`、`docs/db-schema/**`、`docs/openapi/**`、\n  `docs/coverage/**`）を権威として引用する。"],
    ["- [ ] `*.ja.md` を読んでいない。生成ツリーを検索から除外した。", "- [ ] 生成ツリーを検索から除外した。"],
  ]),
  ...forFile(".claude/skills/resolve-merge/SKILL.ja.md", [
    ["- ❌ 生成物・lockfile・`.ja.md` を手で編集する、片側を選ぶ。", "- ❌ 生成物・lockfile を手で編集する、片側を選ぶ。"],
  ]),
  ...forFile(".claude/skills/how-to/SKILL.ja.md", [
    ["- ❌ 生成物・lockfile・`.ja.md` を手で編集する、片側を選ぶ。", "- ❌ 生成物・lockfile を手で編集する、片側を選ぶ。"],
  ]),
  ...forFile(".codex/skills/repo-truth/SKILL.ja.md", [
    ["- `*.ja.md` や生成文書を権威として読み、引用しません。", "- 生成文書を権威として読み、引用しません。"],
    ["- [ ] `*.ja.md` を読まず、生成ツリーを除外した。", "- [ ] 生成ツリーを除外した。"],
  ]),
  ...forFile(".claude/skills/tools-upgrade/SKILL.md", [
    [", `docker/**/README.ja.md`"],
  ]),
  ...forFile(".codex/skills/tools-upgrade/SKILL.md", [
    [", `docker/**/README.ja.md`"],
  ]),
  ...forFile("docs/adr/0000-record-architecture-decisions.md", [
    [" (each ADR also needs its `.ja.md` translation)"],
  ]),
  ...forFile("scripts/README.md", [
    [", translation pairs (`SKILL.ja.md` exists, carries no frontmatter, opens with a sync note, and its heading-level sequence matches `SKILL.md`)"],
    [" Codex-side `SKILL.ja.md` is optional, so it is\nchecked as a translation pair only when present."],
  ]),
  ...forFile(".claude/skills/comment-sweep/SKILL.md", [
    ["   Whichever is chosen, the English canonical file and its `.ja.md` translation — plus the log table in\n   `docs/adr/README.md` and `docs/adr/README.ja.md` — are updated in the same change.", "   Whichever is chosen, the file and the log table in `docs/adr/README.md` are updated in the same\n   change."],
  ]),
  ...forFile("AGENTS.md", [
    ["**Documentation scope for agents** — the canonical sources are the English `README.md` and\n`docs/**/*.md`. **Never read `*.ja.md` files: they are human-facing Japanese translations of\nthose canonical sources — read the canonical English original instead.** Also ignore the\ndocumentation-portal UI assets:\n\n```txt\n**/*.ja.md\ndocs/portal/**\n```", "**Documentation scope for agents** — the canonical sources are `README.md` and `docs/**/*.md`.\nIgnore the documentation-portal UI assets:\n\n```txt\ndocs/portal/**\n```"],
  ]),
  ...forFile("docs/get-started/setup-repository.md", [
    ["1. Rewrite the contents of README.md and README.ja.md according to your project; replace or remove\n   the repository-specific branch-rule exception in the maintainer-policy section.\n2. If your project keeps its documentation in a single language, you may collapse the pair — for\n   example by replacing README.md with the contents of README.ja.md.", "1. Rewrite the contents of README.md according to your project; replace or remove the\n   repository-specific branch-rule exception in the maintainer-policy section."],
    ["3. Rewrite the contents of [openapi.yaml](../../openapi/openapi.yaml) according to your project.", "2. Rewrite the contents of [openapi.yaml](../../openapi/openapi.yaml) according to your project."],
  ]),
  ...forFile("docs/maintenance/docs-structure.md", [
    ["## 2. Japanese Documents\n\nA Japanese document sits **beside its English canonical**, named `<name>.ja.md`. The suffix is what\nseparates the languages; there is no separate directory, and the generator splits them by suffix.\n\nThese files are displayed in the **Architecture (Japanese)** section.\n\n## 3. Section Documents", "## 2. Section Documents"],
    ["## 4. Japanese Section Documents\n\nA section's Japanese documents live in that same section directory, as `<name>.ja.md`. Nothing else\nis needed — the generator finds them by suffix and files them under:\n\n```txt\nProject (Japanese)\n```\n\n## 5. Reserved Directories", "## 3. Reserved Directories"],
    ["\ndocs/security/auth.ja.md"],
    ["\nSecurity (Japanese)"],
    ["\n|docs/*.ja.md|Japanese|"],
    ["\n|docs/<section>/*.ja.md|Japanese セクション|"],
  ]),
  ...forFile("docs/maintenance/portal-manifest.md", [
    ["  # Japanese\n  - src: <source path>.ja.md\n    dst: docs/portal/guides/ja/<flat-name>.ja.md\n  - ...\n", "  - ...\n"],
    [" / `.ja.md`"],
    ["\n| `docs/*.ja.md` | items of section `architecture` (lang: ja) | Japanese counterparts |"],
    ["\n| `docs/<dir>/*.ja.md` | items of section `<dir>` (lang: ja) | Japanese counterparts |"],
    ["     # Japanese\n     - src: internal/controller/<new-package>/README.ja.md\n       dst: docs/portal/guides/ja/controller-<new-package>.ja.md\n"],
  ]),
  ...forFile(".claude/README.md", [
    ["- **`skill-lint` does not check it.** The repository's skill conventions — frontmatter, the\n  `SKILL.ja.md` pair, references that resolve — assume a skill this repository writes.", "- **`skill-lint` does not check it.** The repository's skill conventions — frontmatter and\n  references that resolve — assume a skill this repository writes."],
  ]),
  ...forFile(".claude/skills/comment-sweep/SKILL.ja.md", [
    [" と `docs/adr/README.ja.md`"],
  ]),
  ...forFile(".codex/skills/comment-sweep/SKILL.ja.md", [
    ["英語の正本・隣の `.ja.md`・英日両方の ADR ログ表を揃えて更新する", "正本と ADR ログ表を揃えて更新する"],
  ]),
  ...forFile(".claude/skills/context-map/SKILL.ja.md", [
    [" と隣の `.ja.md`"],
  ]),
  ...forFile(".claude/skills/glossary/SKILL.ja.md", [
    ["で、**`.ja.md`\nペアを持たない**——作らないこと、`canonicalize-doc` へ繋がないこと。", "である。"],
    ["、`.ja.md` ペアの作成"],
  ]),
  ...forFile(".codex/skills/glossary/SKILL.ja.md", [
    ["用語集には `.ja.md` ペアを作りません。"],
  ]),
  ...forFile(".claude/skills/commit/SKILL.ja.md", [
    ["、`*.ja.md`"],
  ]),
  ...forFile(".claude/skills/go-upgrade/SKILL.ja.md", [
    [" / `docker/README.ja.md`"],
    [" / `docker/server/README.ja.md`"],
    [" / `docker/tools/README.ja.md`"],
  ]),
  ...forFile(".claude/skills/manage-skill/SKILL.ja.md", [
    [" / `SKILL.ja.md`"],
  ]),
  ...forFile(".claude/skills/new-env/SKILL.ja.md", [
    [", `env/README.ja.md`"],
    ["\n- 日本語のみ供与 → `env/README.ja.md` に日本語そのまま、`env/README.md` に英訳を記入"],
    [", env/README.ja.md"],
    [" → `env/README.ja.md`"],
  ]),
  ...forFile(".claude/skills/portal-manifest-sync/SKILL.ja.md", [
    [" / `README.ja.md`"],
    ["\n- 日本語: `docs/portal/guides/ja/<flat-hyphenated-name>.ja.md`"],
    ["1. 英語 README を読み込む（`*.ja.md` sibling は Step 2 プリフライトで存在保証済み）", "1. README を読み込む"],
  ]),
  ...forFile(".claude/skills/repo-ops/SKILL.ja.md", [
    ["-g '!**/*.ja.md' "],
  ]),
  ...forFile(".claude/skills/tool-map/SKILL.ja.md", [
    ["\n- [ ] skills スキャンから `*.ja.md` を除外した"],
  ]),
  ...forFile(".claude/skills/tools-upgrade/SKILL.ja.md", [
    [", `docker/**/README.ja.md`"],
  ]),
  ...forFile(".codex/skills/go-upgrade/SKILL.ja.md", [
    [" / `docker/README.ja.md`"],
    [" / `docker/server/README.ja.md`"],
    [" / `docker/tools/README.ja.md`"],
  ]),
  ...forFile(".codex/skills/manage-skill/SKILL.ja.md", [
    [" / `SKILL.ja.md`"],
  ]),
  ...forFile(".codex/skills/new-env/SKILL.ja.md", [
    [", `env/README.ja.md`"],
    ["\n- 日本語のみ供与 → `env/README.ja.md` に日本語そのまま、`env/README.md` に英訳を記入"],
    [", env/README.ja.md"],
    [" → `env/README.ja.md`"],
  ]),
  ...forFile(".codex/skills/portal-manifest-sync/SKILL.ja.md", [
    [" / `README.ja.md`"],
    ["\n- 日本語: `docs/portal/guides/ja/<flat-hyphenated-name>.ja.md`"],
    ["1. 英語 README を読み込む（`*.ja.md` sibling は Step 2 プリフライトで存在保証済み）", "1. README を読み込む"],
  ]),
  ...forFile(".codex/skills/repo-ops/SKILL.ja.md", [
    ["-g '!**/*.ja.md' "],
  ]),
  ...forFile(".codex/skills/tool-map/SKILL.ja.md", [
    ["\n- [ ] skills スキャンから `*.ja.md` を除外した"],
  ]),
  ...forFile(".codex/skills/tools-upgrade/SKILL.ja.md", [
    [", `docker/**/README.ja.md`"],
  ]),
  ...forFile(".claude/skills/sync-ai/SKILL.ja.md", [
    ["、`sh -n` を通し、`SKILL.md` / `SKILL.ja.md` の見出し数が一致していることを確かめる", "、`sh -n` を通す"],
  ]),
  ...forFile(".codex/skills/sync-ai/SKILL.ja.md", [
    ["さらに `sh -n` を実行し、`SKILL.md` / `SKILL.ja.md` の見出し数が一致することも確認する。", "さらに `sh -n` を実行する。"],
  ]),
  ...forFile(".codex/skills/back-prop/SKILL.ja.md", [
    [" から `*.ja.md` を除いた"],
  ]),
  ...forFile("docs/maintenance/docs-structure.ja.md", [
    ["\n|docs/*.ja.md|Japanese|"],
    ["\n|docs/<section>/*.ja.md|Japanese セクション|"],
  ]),
  ...forFile("docs/maintenance/portal-manifest.ja.md", [
    ["\n| `docs/*.ja.md` | section `architecture` の item (lang: ja) | 日本語版 |"],
    ["\n| `docs/<dir>/*.ja.md` | section `<dir>` の item (lang: ja) | 日本語版 |"],
    [" (`.md` / `.ja.md`)", " `.md`"],
  ]),
  ...forFile(".claude/skills/back-prop/SKILL.ja.md", [
    ["、`docs/architecture.md`。`*.ja.md` は除く）", "、`docs/architecture.md`）"],
  ]),
  ...forFile(".agents/closed-loop/skill-meta.yaml", [
    ["    opportunity_predicate: \"対訳ペアの片側だけが変更された（`X.md` と `X.ja.md` の一方のみ）\"\n"],
  ]),
  ...forFile(".github/workflows/licensed-scanners-removal-check.yaml", [
    ["      - '.github/workflows/README.ja.md'\n"],
  ]),
  ...forFile(".github/workflows/sync-versions-check.yaml", [
    ["      - 'docker/**/README.ja.md'\n"],
  ]),
  ...forFile(".github/workflows/setup-scripts-check.yaml", [
    ["grep -ril 'boilerplate' README.md README.ja.md", "grep -ril 'boilerplate' README.md"],
  ]),
  ...forFile(".gitleaksignore", [
    ["env/README.ja.md:generic-api-key:258\n"],
    ["env/README.ja.md:generic-api-key:217\n"],
    // 週次の履歴スキャンが拾う分。上の 2 件と違い過去の版の行番号なので、畳んで行が動いても
    // 番号は動かない。落とすのは番号のためではなく、名指している対訳がその名前で存在しなくなるため。
    ["env/README.ja.md:generic-api-key:219\n"],
    ["env/README.ja.md:generic-api-key:245\n"],
  ]),
  // gitleaks のフィンガープリントは行番号を含むので、畳んで行が動けば無視が外れて検出が復活する。
  // 残る側がどちらの言語かで行数が違うため、モードごとに宣言する。
  ...forFile(
    ".gitleaksignore",
    [
      ["env/README.md:generic-api-key:260", "env/README.md:generic-api-key:259"],
      ["env/README.md:generic-api-key:219", "env/README.md:generic-api-key:218"],
    ],
    "en",
  ),
  ...forFile(
    ".gitleaksignore",
    [
      ["env/README.md:generic-api-key:260", "env/README.md:generic-api-key:257"],
      ["env/README.md:generic-api-key:219", "env/README.md:generic-api-key:216"],
    ],
    "ja",
  ),
  ...forFile(".graphifyignore", [
    ["*.ja.md\n**/*.ja.md\n"],
  ]),
  ...forFile("scripts/graphify-pending/main.go", [
    ["//\t*.ja.md         ファイル名のパターン", "//\t*.gen.sql       ファイル名のパターン"],
    ["// matchesGlob は `*.ja.md` のような、", "// matchesGlob は `*.gen.sql` のような、"],
  ]),
  ...forFile("scripts/graphify-pending/main_test.go", [
    ["\t\t\tfiles := manifest(`{\"docs/rules.ja.md\":{\"semantic_hash\":\"stale\"},\"docs/rules.md\":{\"semantic_hash\":\"stale\"}}`)\n\t\t\tfiles[\"docs/rules.ja.md\"] = body\n\t\t\tfiles[\"docs/rules.md\"] = body\n\t\t\tfiles[\"ignore\"] = \"*.ja.md\\n**/*.ja.md\\n\"", "\t\t\tfiles := manifest(`{\"docs/a.gen.sql\":{\"semantic_hash\":\"stale\"},\"docs/rules.md\":{\"semantic_hash\":\"stale\"}}`)\n\t\t\tfiles[\"docs/a.gen.sql\"] = body\n\t\t\tfiles[\"docs/rules.md\"] = body\n\t\t\tfiles[\"ignore\"] = \"*.gen.sql\\n**/*.gen.sql\\n\""],
    ["\t\t\"拡張子パターンは深い階層にも当たる\":       {\"docs/adr/0001.ja.md\", \"*.ja.md\", true},\n\t\t\"拡張子パターンは正本に当たらない\":        {\"docs/adr/0001.md\", \"*.ja.md\", false},", "\t\t\"拡張子パターンは深い階層にも当たる\":       {\"docs/adr/0001.gen.sql\", \"*.gen.sql\", true},\n\t\t\"拡張子パターンは他の綴りに当たらない\":      {\"docs/adr/0001.md\", \"*.gen.sql\", false},"],
    ["\t\t\tassert.True(t, matchesGlob(\"rules.ja.md\", \"*.ja.md\"))", "\t\t\tassert.True(t, matchesGlob(\"rules.gen.sql\", \"*.gen.sql\"))"],
  ]),
  ...forFile("AGENTS.ja.md", [
    ["**エージェントにとってのドキュメント範囲** —— 正典は英語の `README.md` と `docs/**/*.md` です。\n**`*.ja.md` ファイルは決して読まないでください。これらは正典の人間向け日本語訳であり、\n正典である英語の原文を読んでください。** ドキュメントポータルの UI アセットも無視します:\n\n```txt\n**/*.ja.md\ndocs/portal/**\n```", "**エージェントにとってのドキュメント範囲** —— 正典は `README.md` と `docs/**/*.md` です。\nドキュメントポータルの UI アセットは無視します:\n\n```txt\ndocs/portal/**\n```"],
  ]),
  ...forFile("scripts/setup/remove-licensed-scanners/scanner-manifest.ts", [
    ["      {\n        file: README_JA,\n        block:\n          \"|SonarQube Cloud Scan|`sonarqube.yaml`|SonarQube Cloud による一次ソースの解析。結果は Web API から読み戻して SARIF へ変換する（**Sonar の品質ゲートでブロックする**。issue の一覧は報告専用。`SONAR_TOKEN` が必要。[資格情報を要するスキャナの撤去](#資格情報を要するスキャナの撤去)を参照）|\\n\",\n      },\n"],
    ["      {\n        file: README_JA,\n        block:\n          \"| SonarQube Cloud | Go / TypeScript / `sonar-project.properties` 変更 PR | 同上 | \\u2014 （下記参照） |\\n\",\n      },\n"],
    ["      {\n        file: README_JA,\n        block:\n          \"| `sonarqube.yaml` `sonarqube` | 15 | ベンダー側の解析キューが最大 10 分待つため。テストとカバレッジのゲートはそれぞれの所有ワークフローで実行する |\\n\",\n      },\n"],
    ["      {\n        file: README_JA,\n        block:\n          \"\\nSonarQube Cloud にスロットはありません。無料プランは organization ごとに 1 ブランチしか解析せず他は拒否するため、定期実行のブランチ解析はここでは成功し得ません。ベンダーが例外としている pull request と、code scanning のベースラインを保つための保護ブランチへの push でだけ走ります。`05:00` は SonarQube Cloud が手放したスロットで、空いたままです。次に増える定期実行がそこを取ります。\\n\",\n      },\n"],
    ["      { file: README_JA, fragment: \" + `sonarqube.yaml`（SonarQube Cloud） **(gate, 品質ゲート)**\" },\n"],
    ["      {\n        file: README_JA,\n        fragment:\n          \"SonarQube Cloud はもっと単純な理由で未接続です。通知の元になる定期実行を持っていません。\",\n      },\n"],
    ["      {\n        file: README_JA,\n        block:\n          \"|CodeQL Scan|`code-ql.yaml`|`security-extended` スイートでの CodeQL 解析。言語ごとに matrix を分け、`go` / `javascript-typescript`（docs-viewer / scripts）/ `actions`（ワークフロー定義そのもの）を対象とする|\\n\",\n      },\n"],
    ["      {\n        file: README_JA,\n        block:\n          \"| CodeQL | Go / TypeScript / Actions 定義の変更 PR | 同上 | 週次 |\\n\",\n      },\n"],
    ["      {\n        file: README_JA,\n        block:\n          \"| `code-ql.yaml` `codeql` | 30 | 上限は matrix の最も遅い leg に掛かるが、`go` 以外の leg には完了実行が無く実測できない。加えて `security-extended` は従前の値を測ったスイートより大きい |\\n\",\n      },\n"],
    ["      {\n        file: README_JA,\n        block:\n          \"|Licensed Scanners Removal Check|`licensed-scanners-removal-check.yaml`|ライセンス条件付きのスキャナを撤去し、その後も `make pin-actions-check` / `make egress-check` / Markdown lint が緑であることを検証する。いずれも「どの workflow も参照しないエントリ」で落ちるため、取りこぼしは撤去を走らせたリポジトリでしか現れない|\\n\",\n      },\n"],
    ["      {\n        file: README_JA,\n        block:\n          \"\\n`自前の Go ソース` と `自前の TypeScript ソース` の行にはベンダーホスト型のスキャナも乗っています。Sonar はこの表で唯一「ルール単位で担当 1 つ」から意図的に外れています。品質ゲートは静的解析・重複と Sonar 自身の issue 分類をまとめて判定し、カバレッジの閾値は Go / TypeScript のテストワークフローがそれぞれ担います。両者が認識する検出で PR が 2 回赤くなり得ますが、それを受け入れているのは、ベンダーの判定を捨てると「スキャンは報告するが run はそのままマージされる」状態になるためです。\\n\",\n      },\n"],
    ["      { file: README_JA, fragment: \"、`01:15` CodeQL\" },\n"],
    ["      { file: README_JA, fragment: \"`code-ql.yaml`（`javascript-typescript` レグ）+ \" },\n"],
    ["      { file: README_JA, fragment: \"+ `code-ql.yaml`（`actions` レグ）\" },\n"],
    ["      { file: README_JA, heading: \"#### 資格情報を要するスキャナの撤去\" },\n"],
    ["const README_JA = \".github/workflows/README.ja.md\";\n"],
  ], "en"),
  ...forFile("scripts/setup/remove-licensed-scanners/scanner-manifest.ts", [
    ["      {\n        file: README_EN,\n        block:\n          \"|SonarQube Cloud Scan|`sonarqube.yaml`|SonarQube Cloud analysis of first-party source, read back over the Web API and converted to SARIF (**gates on Sonar's quality gate**, issue list report-only; needs `SONAR_TOKEN`, see [Removing the credential-bearing scanners](#removing-the-credential-bearing-scanners))|\\n\",\n      },\n"],
    ["      {\n        file: README_EN,\n        block:\n          \"| SonarQube Cloud | Go / TypeScript / `sonar-project.properties`-change PRs | same as above | \\u2014 (see below) |\\n\",\n      },\n"],
    ["      {\n        file: README_EN,\n        block:\n          \"| `sonarqube.yaml` `sonarqube` | 15 | vendor-side analysis can queue for up to 10 minutes; test and coverage gates run in their owning workflows |\\n\",\n      },\n"],
    ["      {\n        file: README_EN,\n        block:\n          \"\\nSonarQube Cloud has no slot. Its free plan analyzes one branch per organization and refuses every other one, so a scheduled branch analysis cannot succeed here — it runs on pull requests, where the vendor exempts that limit, and on a push to a protected branch to keep the code-scanning baseline. `05:00` is the slot it gave up and stays free; the next scheduled workflow takes it.\\n\",\n      },\n"],
    ["      { file: README_EN, fragment: \" + `sonarqube.yaml` (SonarQube Cloud) **(gate, quality gate)**\" },\n"],
    ["      {\n        file: README_EN,\n        fragment:\n          \" SonarQube Cloud is unconnected for a plainer one: it has no scheduled run to notify from.\",\n      },\n"],
    ["      {\n        file: README_EN,\n        block:\n          \"|CodeQL Scan|`code-ql.yaml`|CodeQL analysis on the `security-extended` suite, one matrix leg per language: `go`, `javascript-typescript` (docs-viewer / scripts) and `actions` (the workflow definitions themselves)|\\n\",\n      },\n"],
    ["      {\n        file: README_EN,\n        block:\n          \"| CodeQL | Go / TypeScript / Actions-definition-change PRs | same as above | weekly |\\n\",\n      },\n"],
    ["      {\n        file: README_EN,\n        block:\n          \"|Licensed Scanners Removal Check|`licensed-scanners-removal-check.yaml`|Remove the licence-conditional scanners and verify `make pin-actions-check` / `make egress-check` / Markdown lint stay green afterwards — each of those fails on an entry no workflow references, so a missed one surfaces only in the repository that ran the removal|\\n\",\n      },\n"],
    ["      {\n        file: README_EN,\n        block:\n          \"| `code-ql.yaml` `codeql` | 30 | the limit covers whichever matrix leg is slowest, and no leg but `go` has a completed run to measure; `security-extended` is also a larger suite than the one the previous value was measured against |\\n\",\n      },\n"],
    ["      {\n        file: README_EN,\n        block:\n          \"\\nThe `First-party Go source` and `First-party TypeScript source` rows carry the vendor-hosted scanner as well. Sonar is the one deliberate departure from \\\"one owner per rule\\\" in this table. Its quality gate judges static analysis and duplication alongside its own issue taxonomy, while the Go and TypeScript test workflows own coverage thresholds. A finding both engines recognize can still turn a pull request red twice; that is accepted because discarding the vendor's verdict entirely would leave the scan reporting into a run that merged regardless.\\n\",\n      },\n"],
    ["      { file: README_EN, fragment: \", `01:15` CodeQL\" },\n"],
    ["      { file: README_EN, fragment: \"`code-ql.yaml` (`javascript-typescript` leg) + \" },\n"],
    ["      { file: README_EN, fragment: \" + `code-ql.yaml` (`actions` leg)\" },\n"],
    ["      { file: README_EN, heading: \"#### Removing the credential-bearing scanners\" },\n"],
    ["const README_EN = \".github/workflows/README.md\";\n"],
    ["const README_JA = \".github/workflows/README.ja.md\";", "const README_JA = \".github/workflows/README.md\";"],
  ], "ja"),
  ...forFile(".claude/skills/manage-skill/SKILL.md", [
    [" + mandatory `SKILL.ja.md` translation pair"],
    ["ALWAYS use it before hand-editing a `SKILL.md` or `SKILL.ja.md`.", "ALWAYS use it before hand-editing a `SKILL.md`."],
    ["those have `sync-readme` / `canonicalize-doc` / `back-prop`", "those have `sync-readme` / `back-prop`"],
  ]),
  ...forFile(".codex/skills/manage-skill/SKILL.md", [
    [" + mandatory `SKILL.ja.md` translation pair"],
    ["ALWAYS use it before hand-editing a `SKILL.md` or `SKILL.ja.md`.", "ALWAYS use it before hand-editing a `SKILL.md`."],
    ["those have `sync-readme` / `canonicalize-doc` / `back-prop`", "those have `sync-readme` / `back-prop`"],
  ]),
  ...forFile(".claude/skills/sync-readme/SKILL.md", [
    ["### 6. Chain into `canonicalize-doc` to sync the translation\n\nAfter the canonical README is written:\n\n1. Check whether a sibling translation file exists (e.g., `README.ja.md` next to the updated `README.md`).\n2. If it does, invoke the `canonicalize-doc` skill via the Skill tool with:\n    - source path: the canonical README that was just updated\n    - direction: `translation-from-canonical` (or `sync-both` with the canonical as source of truth, if the translation already exists)\n3. If no translation file exists, skip this step and report that the canonical was updated standalone.\n\nThe chained `canonicalize-doc` call will perform its own `AskUserQuestion` confirmation; that is expected and not redundant — it lets the user veto the translation sync if needed.\n\n"],
    ["### 7. Verify with Markdown Lint", "### 6. Verify with Markdown Lint"],
    ["### 8. Final verification", "### 7. Final verification"],
    ["After writing the canonical README (and after `canonicalize-doc` has produced any translation), run:", "After writing the canonical README, run:"],
  ]),
  ...forFile(".codex/skills/sync-readme/SKILL.md", [
    ["### 6. Chain into `canonicalize-doc` to sync the translation\n\nAfter the canonical README is written:\n\n1. Check whether a sibling translation file exists (e.g., `README.ja.md` next to the updated `README.md`).\n2. If it does, invoke the `canonicalize-doc` skill via the Skill tool with:\n    - source path: the canonical README that was just updated\n    - direction: `translation-from-canonical` (or `sync-both` with the canonical as source of truth, if the translation already exists)\n3. If no translation file exists, skip this step and report that the canonical was updated standalone.\n\nThe chained `canonicalize-doc` call will perform its own `ask the user explicitly` confirmation; that is expected and not redundant — it lets the user veto the translation sync if needed.\n\n"],
    ["### 7. Verify with Markdown Lint", "### 6. Verify with Markdown Lint"],
    ["### 8. Final verification", "### 7. Final verification"],
    ["After writing the canonical README (and after `canonicalize-doc` has produced any translation), run:", "After writing the canonical README, run:"],
  ]),
  ...forFile("docs/get-started/setup-repository.ja.md", [
    ["1. [README.md](../../README.md), [README.ja.md](../../README.ja.md) の内容をプロジェクトに合わせて書き換え、メンテナ方針節にあるこのリポジトリ固有のブランチ規則の例外は置き換えるか削除してください。\n2. ドキュメントを 1 言語に絞るなら、対を畳んでも構いません（例えば [README.md](../../README.md) を [README.ja.md](../../README.ja.md) の内容で置き換える）。\n    - [gen-docs-json.ts](../../scripts/portal/gen-docs-json.ts) と、それが生成元にする [manifest.yaml](../portal/manifest.yaml) はどちらも README.md を参照しているため、完全に置換する場合はこれらのスクリプトも書き換える必要があります。\n    - portal の UI も En / Jp の切り替えを持つので、同じ手当てが要ります。\n3. [openapi.yaml](../../openapi/openapi.yaml) の内容をプロジェクトに合わせて書き換えてください。", "1. [README.md](../../README.md) の内容をプロジェクトに合わせて書き換え、メンテナ方針節にあるこのリポジトリ固有のブランチ規則の例外は置き換えるか削除してください。\n2. [openapi.yaml](../../openapi/openapi.yaml) の内容をプロジェクトに合わせて書き換えてください。"],
  ]),
  ...forFile(".claude/skills/sync-readme/SKILL.ja.md", [
    ["### 6. `canonicalize-doc` をチェーンして翻訳を同期\n\ncanonical README の書き込み完了後:\n\n1. 兄弟の翻訳ファイル（例: 更新した `README.md` の隣の `README.ja.md`）の有無を確認する。\n2. 存在する場合、Skill ツールで `canonicalize-doc` を起動する。引数は:\n    - source パス: 今回更新した canonical README\n    - direction: `translation-from-canonical`（翻訳が既に存在するなら `sync-both`、source of truth は canonical）\n3. 翻訳ファイルが存在しない場合は本ステップをスキップし、canonical のみ更新した旨を報告する。\n\nチェーンで呼び出された `canonicalize-doc` 自身が改めて `AskUserQuestion` で確認を行うのは期待される動作（冗長ではない）。ユーザーが翻訳同期を veto できる余地を残すため。\n\n"],
    ["### 7. Markdown Lint による検証", "### 6. Markdown Lint による検証"],
    ["### 8. 最終検証", "### 7. 最終検証"],
  ]),
  ...forFile(".codex/skills/sync-readme/SKILL.ja.md", [
    ["### 6. `canonicalize-doc` をチェーンして翻訳を同期\n\ncanonical README の書き込み完了後:\n\n1. 兄弟の翻訳ファイル（例: 更新した `README.md` の隣の `README.ja.md`）の有無を確認する。\n2. 存在する場合、Skill ツールで `canonicalize-doc` を起動する。引数は:\n    - source パス: 今回更新した canonical README\n    - direction: `translation-from-canonical`（翻訳が既に存在するなら `sync-both`、source of truth は canonical）\n3. 翻訳ファイルが存在しない場合は本ステップをスキップし、canonical のみ更新した旨を報告する。\n\nチェーンで呼び出された `canonicalize-doc` 自身が改めて `ask the user explicitly` で確認を行うのは期待される動作（冗長ではない）。ユーザーが翻訳同期を veto できる余地を残すため。\n\n"],
    ["### 7. Markdown Lint による検証", "### 6. Markdown Lint による検証"],
    ["### 8. 最終検証", "### 7. 最終検証"],
  ]),
  ...forFile(".claude/README.ja.md", [
    ["- **英語が canonical。** skill / README 本文は命令形の英語で書き、対になる `*.ja.md` は `canonicalize-doc`\n  skill で同期する人間向け参考訳です。ユーザーへの実行時出力は引き続き `CLAUDE.md` に従います（日本語）。\n"],
  ]),
  ...forFile("docs-viewer/src/portal-app/portal-app.tsx", [
    ["            <ToggleGroupNative aria-label=\"表示言語\">\n              <ToggleGroupNativeItem\n                checked={lang === \"EN\"}\n                name=\"lang\"\n                onChange={selectEnglish}\n                value=\"EN\"\n              >\n                EN\n              </ToggleGroupNativeItem>\n              <ToggleGroupNativeItem\n                checked={lang === \"JA\"}\n                name=\"lang\"\n                onChange={selectJapanese}\n                value=\"JA\"\n              >\n                JA\n              </ToggleGroupNativeItem>\n            </ToggleGroupNative>\n"],
  ]),
];

/**
 * ツールと共に死ぬ記述のうち、マーカーを置けないもの。3 モードとも落とす。
 *
 * @remarks
 * `sonar-project.properties` の重複除外は 1 行に複数のパスが並ぶ値で、そのうちの 1 つだけを
 * 落とすため行を囲むマーカーでは表せません。しかもこの行は `sample-api` の差し替えブロックの
 * 内側にあり、退避側にも同じパスが載っています。完全一致で両方から抜くのが最も壊れにくい形です。
 */
export const TOOL_REPLACEMENTS: readonly DocReplacement[] = [
  ...forFile("sonar-project.properties", [
    ["# language-manifest.ts は撤去の宣言台帳で、1 エントリが「ファイル・置換前・置換後」の同じ形を\n# 取る。重なるのはその形式であって判断ではない。まとめる抽象（両環境を 1 行で書く等）は入れられる\n# が、そうすると「どちらの AI 環境に対する宣言か」が読み取れなくなり、完全一致で照合する台帳と\n# しての性質を損なう。行の重複だけを外し、静的解析の他の観点は対象に残す。\n"],
    ["scripts/setup/remove-doc-language/language-manifest.ts,"],
  ]),
];

/**
 * 畳んだ後も残ってよい、対訳を名指す綴り（前後の空白を除いた完全一致）。
 *
 * @remarks
 * `.ja.md` で終わる綴りがすべて対訳を指すわけではありません。`tool-map` が書き出す
 * `TOOL_MAP.ja.md` は、このリポジトリのドキュメントではなくスキル自身の出力ファイルで、
 * 言語を畳んだ後もそのスキルは日本語版の地図を書き出せます。
 *
 * 除外は宣言でしか表せません。撤去は「消える名前を指す言及」を止めるので、止めてはいけない
 * ものはここで名指す必要があります —— `doc-ref-lint` の翻訳除外と同じ形です。
 */
export const ALLOWED_MENTIONS: readonly string[] = [
  "| `--output-path` | any relative path | `./TOOL_MAP.md` (en) or `./TOOL_MAP.ja.md` (ja) — only used if `--output=file` |",
  "| `--output-path` | 任意の相対パス | `./TOOL_MAP.md`（en）または `./TOOL_MAP.ja.md`（ja）— `--output=file` のときのみ使用 |",
];

/**
 * 機械では畳めず、行ごと落とすと宣言した行（前後の空白を除いた完全一致）。
 *
 * @remarks
 * ここに載るのは「対訳規約を説明している散文」のうち、行として独立していて、消しても前後の
 * 文が成立するものだけです。文の途中で対訳に触れている行——`manage-skill` / `sync-ai` の
 * 手順のように、対訳を含む節を別の言い回しへ差し替える必要があるもの——はここではなく、
 * 本文側の `doc-pair:replace-begin` / `replace-with` / `replace-end` が持ちます。
 *
 * 宣言は最小に保ってください。完全一致がなぜ fail-closed になるかは DOC_REPLACEMENTS の @remarks。
 */
export const DECLARED_LINES: readonly string[] = [
  "| skill | `SKILL.md` | `SKILL.ja.md` |",
  "- `SKILL.ja.md` (translation, Japanese):",
  "- Claude Code skill files: `SKILL.md` / `SKILL.ja.md` (under `.claude/skills/<name>/`)",
  "- If creating a `SKILL.md`, ensure it contains the one-line pointer to `SKILL.ja.md`.",
  "- If creating a `SKILL.ja.md`, ensure the leading blockquote sync note is present.",
];
