import { describe, expect, it } from "vitest";

import {
  PLATFORM_ONLY_SKILLS,
  allowlistLocation,
  asRepoPath,
  collectMakeTargets,
  compareHeadingStructure, // doc-pair:line
  eachLineOutsideFence,
  expandBraces,
  extractHeadings, // doc-pair:line
  extractInlineCode,
  extractMakeTargets,
  hasTranslationNote, // doc-pair:line
  makeTargetExists,
  onlyIn,
  parseFrontmatterKeys,
  literalParentDir,
  placeholderToRegExp,
  splitFrontmatter,
  extractNumberedSections,
  extractSectionRefs,
} from "./checks";

const ROOT_ENTRIES = new Set(["scripts", "docs", "internal", "pkg", "makefile"]);
const never = () => false;

function linesOutsideFence(content: string): string[] {
  return [...eachLineOutsideFence(content)].map(({ line }) => line);
}

describe("expandBraces", () => {
  describe("正常系", () => {
    it("波括弧を候補へ展開する", () => {
      expect(expandBraces("gen-{api,query}")).toEqual(["gen-api", "gen-query"]);
    });
    it("入れ子でない複数の波括弧を総当たりで展開する", () => {
      expect(expandBraces("{a,b}-{x,y}")).toEqual(["a-x", "a-y", "b-x", "b-y"]);
    });
    it("波括弧が無ければそのまま 1 件で返す", () => {
      expect(expandBraces("make go-lint")).toEqual(["make go-lint"]);
    });
  });
});

describe("literalParentDir", () => {
  describe("正常系", () => {
    it("最初のワイルドカードより手前を親として返す", () => {
      expect(literalParentDir("internal/domain/<aggregate>/error.go")).toBe("internal/domain");
      expect(literalParentDir("internal/domain/<aggregate>/mock/")).toBe("internal/domain");
    });

    it("`*` も `**` もワイルドカードとして扱う", () => {
      expect(literalParentDir("internal/domain/**/*.go")).toBe("internal/domain");
      expect(literalParentDir("pkg/uuid/*.go")).toBe("pkg/uuid");
    });

    // 途中セグメントの一部がプレースホルダでも、そのセグメントごと手前で切る。
    it("セグメント内の部分プレースホルダでもそのセグメントを含めない", () => {
      expect(literalParentDir("internal/domain/<aggregate>/<aggregate>_domain.go")).toBe(
        "internal/domain",
      );
    });
  });

  describe("異常系", () => {
    it("先頭セグメントがワイルドカードなら null", () => {
      expect(literalParentDir("<layer>/domain/x.go")).toBeNull();
    });

    it("ワイルドカードを含まないパスは null", () => {
      expect(literalParentDir("internal/domain/user/error.go")).toBeNull();
    });
  });
});

describe("placeholderToRegExp", () => {
  describe("正常系", () => {
    it("<name> を 1 セグメント分の穴として扱う", () => {
      const pattern = placeholderToRegExp("internal/domain/<name>", { segmentSeparator: true });

      expect(pattern.test("internal/domain/user")).toBe(true);
      expect(pattern.test("internal/domain/user/sub")).toBe(false);
    });
    it("** を任意階層として扱う", () => {
      const pattern = placeholderToRegExp("internal/**/README.md", { segmentSeparator: true });

      expect(pattern.test("internal/README.md")).toBe(true);
      expect(pattern.test("internal/domain/user/README.md")).toBe(true);
    });
    it("末尾の ** を任意階層として扱う", () => {
      const pattern = placeholderToRegExp("docs/portal/**", { segmentSeparator: true });

      expect(pattern.test("docs/portal/manifest.yaml")).toBe(true);
      expect(pattern.test("docs/portal/guides/a.md")).toBe(true);
      expect(pattern.test("docs/rules.md")).toBe(false);
    });
    it("区切りを持たない文脈では * を任意文字列にする", () => {
      const pattern = placeholderToRegExp("gen-*-oapi", { segmentSeparator: false });

      expect(pattern.test("gen-bundle-oapi")).toBe(true);
    });
    it("正規表現の特殊文字を字面として扱う", () => {
      const pattern = placeholderToRegExp("docs/adr/0001.md", { segmentSeparator: true });

      expect(pattern.test("docs/adr/0001Xmd")).toBe(false);
    });
  });

  describe("異常系", () => {
    it("閉じない < を穴と見なさず字面として扱う", () => {
      const pattern = placeholderToRegExp("docs/a<b.md", { segmentSeparator: true });

      expect(pattern.test("docs/a<b.md")).toBe(true);
      expect(pattern.test("docs/axb.md")).toBe(false);
    });
  });
});

describe("eachLineOutsideFence", () => {
  describe("正常系", () => {
    it("フェンスの中身を返さない", () => {
      expect(linesOutsideFence(["外", "```sh", "中", "```", "外2"].join("\n"))).toEqual(["外", "外2"]);
    });
    it("Markdown を含む Markdown で内側のフェンスに閉じさせない", () => {
      const content = ["````markdown", "```json", "{}", "```", "````", "外"].join("\n");

      expect(linesOutsideFence(content)).toEqual(["外"]);
    });
    it("チルダのフェンスも扱う", () => {
      expect(linesOutsideFence(["~~~", "中", "~~~", "外"].join("\n"))).toEqual(["外"]);
    });
    it("行番号を 1 始まりで返す", () => {
      expect([...eachLineOutsideFence("a\nb")]).toEqual([
        { line: "a", lineNo: 1 },
        { line: "b", lineNo: 2 },
      ]);
    });
  });
});

describe("extractInlineCode", () => {
  describe("正常系", () => {
    it("スパンの中身を前後の空白を落として返す", () => {
      expect(extractInlineCode("実行は `make go-lint` です")).toEqual(["make go-lint"]);
    });
    it("1 行の複数スパンを順に返す", () => {
      expect(extractInlineCode("`a` と `b`")).toEqual(["a", "b"]);
    });
    it("スパンの無い行は空を返す", () => {
      expect(extractInlineCode("ただの散文")).toEqual([]);
    });
  });
});

describe("splitFrontmatter", () => {
  describe("正常系", () => {
    it("--- で囲まれたブロックを切り出す", () => {
      expect(splitFrontmatter("---\nname: x\n---\n本文")).toEqual({ lines: ["name: x"], endLine: 3 });
    });
  });

  describe("異常系", () => {
    it("先頭が --- でなければ null を返す", () => {
      expect(splitFrontmatter("# 見出し\n---\n")).toBeNull();
    });
    it("閉じられていない frontmatter は null を返す", () => {
      expect(splitFrontmatter("---\nname: x\n")).toBeNull();
    });
  });
});

describe("parseFrontmatterKeys", () => {
  describe("正常系", () => {
    it("キーと値を取り出す", () => {
      expect(parseFrontmatterKeys(["name: commit", "model: sonnet"])).toEqual(
        new Map([
          ["name", "commit"],
          ["model", "sonnet"],
        ]),
      );
    });
    it("折り畳みスカラの後続行を連結して値にする", () => {
      const keys = parseFrontmatterKeys(["description: >-", "  1 行目", "  2 行目", "name: x"]);

      expect(keys.get("description")).toBe("1 行目 2 行目");
    });
  });

  describe("異常系", () => {
    it("キーの形をしない行を無視する", () => {
      expect(parseFrontmatterKeys(["  - list item"])).toEqual(new Map());
    });
  });
});

// doc-pair:begin
describe("extractHeadings", () => {
  describe("正常系", () => {
    it("レベルとテキストと行番号を取り出す", () => {
      expect(extractHeadings("# A\n\n## B")).toEqual([
        { level: 1, text: "A", lineNo: 1 },
        { level: 2, text: "B", lineNo: 3 },
      ]);
    });
    it("フェンス内の # を見出しにしない", () => {
      expect(extractHeadings("```sh\n# コメント\n```")).toEqual([]);
    });
  });
});

describe("compareHeadingStructure", () => {
  describe("正常系", () => {
    it("レベル列が一致すれば null を返す", () => {
      expect(compareHeadingStructure(extractHeadings("# A\n## B"), extractHeadings("# あ\n## い"))).toBeNull();
    });
    it("見出しテキストの違いはずれと見なさない（翻訳で変わる）", () => {
      expect(compareHeadingStructure(extractHeadings("# Overview"), extractHeadings("# 概要"))).toBeNull();
    });
  });

  describe("異常系", () => {
    it("対訳に見出しが足りなければ最初の欠落位置を返す", () => {
      const mismatch = compareHeadingStructure(extractHeadings("# A\n## B"), extractHeadings("# あ"));

      expect(mismatch).toMatchObject({ index: 1, translation: null });
      expect(mismatch?.canonical).toMatchObject({ level: 2, text: "B" });
    });
    it("対訳に余分な見出しがあれば最初の余りを返す", () => {
      const mismatch = compareHeadingStructure(extractHeadings("# A"), extractHeadings("# あ\n## い"));

      expect(mismatch).toMatchObject({ index: 1, canonical: null });
    });
    it("レベルの違いをずれとして返す", () => {
      const mismatch = compareHeadingStructure(extractHeadings("## A"), extractHeadings("### あ"));

      expect(mismatch?.index).toBe(0);
    });
  });
});

describe("hasTranslationNote", () => {
  describe("正常系", () => {
    it("冒頭の引用行が canonical を指していれば通す", () => {
      expect(hasTranslationNote("\n> これは SKILL.md の翻訳です\n", "SKILL.md")).toBe(true);
    });
  });

  describe("異常系", () => {
    it("引用行でなければ注記と見なさない", () => {
      expect(hasTranslationNote("SKILL.md の翻訳です", "SKILL.md")).toBe(false);
    });
    it("canonical のファイル名を含まなければ注記と見なさない", () => {
      expect(hasTranslationNote("> 翻訳です", "SKILL.md")).toBe(false);
    });
    it("空のファイルを注記ありと見なさない", () => {
      expect(hasTranslationNote("", "SKILL.md")).toBe(false);
    });
  });
});

// doc-pair:end
describe("onlyIn", () => {
  describe("正常系", () => {
    it("片側にしかない名前を返す", () => {
      expect(onlyIn(["a", "b"], ["b"])).toEqual(["a"]);
    });
  });
});

describe("collectMakeTargets", () => {
  describe("正常系", () => {
    it(".PHONY 行のターゲットを索引へ入れる", () => {
      const index = collectMakeTargets([".PHONY: lint ## Lint を実行"]);

      expect([...index.exact]).toEqual(["lint"]);
    });
    it("ルール行のターゲットを索引へ入れる", () => {
      const index = collectMakeTargets(["serve: build", "\techo hi"]);

      expect([...index.exact]).toEqual(["serve"]);
    });
    it("% を含むターゲットは完全一致ではなくパターンとして持つ", () => {
      const index = collectMakeTargets(["db-migrate-up-%:"]);

      expect([...index.exact]).toEqual([]);
      expect(index.patterns.map((re) => re.source)).toEqual(["^db-migrate-up-.+$"]);
    });
    it("複数の内容を 1 つの索引へまとめる", () => {
      const index = collectMakeTargets([".PHONY: gen-api", ".PHONY: gen-query"]);

      expect([...index.exact].sort()).toEqual(["gen-api", "gen-query"]);
    });
  });

  describe("異常系", () => {
    it("変数代入を索引へ入れない", () => {
      expect([...collectMakeTargets(["GODOC_OUT := docs/godoc"]).exact]).toEqual([]);
    });
    it("特殊ターゲット（. 始まり）を索引へ入れない", () => {
      expect([...collectMakeTargets([".PHONY: lint"]).exact]).toEqual(["lint"]);
      expect([...collectMakeTargets([".SILENT:"]).exact]).toEqual([]);
    });
    it("レシピ行のコマンドを索引へ入れない", () => {
      expect([...collectMakeTargets(["build:", "\techo: hi"]).exact]).toEqual(["build"]);
    });
  });
});

describe("extractMakeTargets", () => {
  describe("正常系", () => {
    it("make に続くターゲット名を取り出す", () => {
      expect(extractMakeTargets("make go-lint")).toEqual(["go-lint"]);
    });
    it("複数ターゲットを取り出す", () => {
      expect(extractMakeTargets("make go-lint test")).toEqual(["go-lint", "test"]);
    });
    it("オプションを読み飛ばす", () => {
      expect(extractMakeTargets("make -s gate-heavy-skip")).toEqual(["gate-heavy-skip"]);
    });
    it("変数代入以降は引数ではないので打ち切る", () => {
      expect(extractMakeTargets("make db-init DB=local")).toEqual(["db-init"]);
    });
  });

  describe("異常系", () => {
    it("make で始まらないスパンは対象外", () => {
      expect(extractMakeTargets("go test ./...")).toEqual([]);
    });
    it("make を接頭辞に持つだけの語に反応しない", () => {
      expect(extractMakeTargets("makefile")).toEqual([]);
    });
    it("シェル演算子以降を引数と見なさない", () => {
      expect(extractMakeTargets("make go-lint 2>&1")).toEqual(["go-lint"]);
    });
  });
});

describe("asRepoPath", () => {
  describe("正常系", () => {
    it("ルート直下エントリから始まるファイル参照を返す", () => {
      expect(asRepoPath("scripts/skill-lint/index.ts", ROOT_ENTRIES, never)).toBe("scripts/skill-lint/index.ts");
    });
    it("先頭の ./ を落とす", () => {
      expect(asRepoPath("./docs/rules.md", ROOT_ENTRIES, never)).toBe("docs/rules.md");
    });
    it("末尾 / のディレクトリ参照を返す", () => {
      expect(asRepoPath("internal/domain/", ROOT_ENTRIES, never)).toBe("internal/domain");
    });
  });

  describe("異常系", () => {
    it("区切りを持たない参照を対象外にする", () => {
      expect(asRepoPath("SKILL.md", ROOT_ENTRIES, never)).toBeNull();
    });
    it("ルート直下に無いエントリから始まる参照を対象外にする", () => {
      expect(asRepoPath("vendor/x/y.go", ROOT_ENTRIES, never)).toBeNull();
    });
    it("Go の import パス（拡張子なし）を対象外にする", () => {
      expect(asRepoPath("internal/domain/user", ROOT_ENTRIES, never)).toBeNull();
    });
    it("省略記法（...）を対象外にする", () => {
      expect(asRepoPath("internal/.../README.md", ROOT_ENTRIES, never)).toBeNull();
    });
    it("空白やシェル記号を含む記述を対象外にする", () => {
      expect(asRepoPath("scripts/a b.ts", ROOT_ENTRIES, never)).toBeNull();
      expect(asRepoPath("scripts/$(name).ts", ROOT_ENTRIES, never)).toBeNull();
    });
    it("パッケージパス + Go シンボルを対象外にする", () => {
      const exists = (relativePath: string) => relativePath === "pkg/ptr";

      expect(asRepoPath("pkg/ptr.Copy", ROOT_ENTRIES, exists)).toBeNull();
    });
    it("同形でもパッケージが実在しなければパス参照として扱う", () => {
      expect(asRepoPath("pkg/ptr.Copy", ROOT_ENTRIES, never)).toBe("pkg/ptr.Copy");
    });
  });
});

describe("PLATFORM_ONLY_SKILLS", () => {
  describe("正常系", () => {
    it("全ての登録が理由を持つ", () => {
      for (const [, reason] of PLATFORM_ONLY_SKILLS) {
        expect(reason.trim()).not.toBe("");
      }
    });
  });
});

describe("allowlistLocation", () => {
  describe("正常系", () => {
    it("allowlist を持つファイル自身を指す", () => {
      expect(allowlistLocation(process.cwd()).endsWith("checks.ts")).toBe(true);
    });
  });
});

describe("makeTargetExists", () => {
  describe("正常系", () => {
    it("索引にある名前を実在と見なす", () => {
      expect(makeTargetExists("lint", collectMakeTargets([".PHONY: lint"]))).toBe(true);
    });
    it("パターンルールに当てはまる名前を実在と見なす", () => {
      const index = collectMakeTargets(["db-migrate-up-%:"]);

      expect(makeTargetExists("db-migrate-up-local", index)).toBe(true);
    });
    it("参照側のプレースホルダに当てはまる実ターゲットがあれば実在と見なす", () => {
      const index = collectMakeTargets([".PHONY: gen-bundle-oapi"]);

      expect(makeTargetExists("gen-*-oapi", index)).toBe(true);
    });
    it("参照側のプレースホルダに当てはまるパターンルールがあれば実在と見なす", () => {
      const index = collectMakeTargets(["db-migrate-up-%:"]);

      expect(makeTargetExists("db-migrate-*", index)).toBe(true);
    });
    it("波括弧の列挙は全件が実在して初めて実在と見なす", () => {
      const index = collectMakeTargets([".PHONY: gen-api gen-query"]);

      expect(makeTargetExists("gen-{api,query}", index)).toBe(true);
    });
  });

  describe("異常系", () => {
    it("索引に無い名前を実在と見なさない", () => {
      expect(makeTargetExists("ghost", collectMakeTargets([".PHONY: lint"]))).toBe(false);
    });
    it("波括弧の 1 つでも欠ければ実在と見なさない", () => {
      const index = collectMakeTargets([".PHONY: gen-api"]);

      expect(makeTargetExists("gen-{api,query}", index)).toBe(false);
    });
  });
});

describe("extractNumberedSections", () => {
  describe("正常系", () => {
    it("H2 以下の番号付き見出しから節番号を拾う", () => {
      const content = ["# 題", "## 0. 序", "### 3. 小節", "## 21. 本題"].join("\n");

      expect([...extractNumberedSections(content)].sort((a, b) => a - b)).toEqual([0, 3, 21]);
    });

    it("連番の外に採番された節も拾う", () => {
      expect([...extractNumberedSections("## 99. 削除される節")]).toEqual([99]);
    });
  });

  describe("異常系", () => {
    it("H1 は文書題であって節ではない", () => {
      expect([...extractNumberedSections("# 1. 題")]).toEqual([]);
    });

    it("番号を持たない見出しは節として数えない", () => {
      expect([...extractNumberedSections("## Contract")]).toEqual([]);
    });

    it("フェンスの中の見出しは節として数えない", () => {
      const content = ["```md", "## 7. 出力例", "```"].join("\n");

      expect([...extractNumberedSections(content)]).toEqual([]);
    });
  });
});

describe("extractSectionRefs", () => {
  describe("正常系", () => {
    it("§ 記法を拾う", () => {
      expect(extractSectionRefs("`repo-ops` §19 を見よ")).toEqual([
        { skill: "repo-ops", section: 19 },
      ]);
    });

    it("section 記法を拾う", () => {
      expect(extractSectionRefs("see `repo-ops` section 0")).toEqual([
        { skill: "repo-ops", section: 0 },
      ]);
    });

    it("日本語の助詞を挟んだ形も拾う", () => {
      expect(extractSectionRefs("`repo-ops` の §21")).toEqual([{ skill: "repo-ops", section: 21 }]);
    });

    it("1 行に複数あればすべて拾う", () => {
      expect(extractSectionRefs("`a` §1 と `b` §2")).toEqual([
        { skill: "a", section: 1 },
        { skill: "b", section: 2 },
      ]);
    });
  });

  describe("異常系", () => {
    it("code span に入っていない名前は拾わない", () => {
      expect(extractSectionRefs("RFC 7235 section 3")).toEqual([]);
    });

    it("節番号を伴わないスキル名は拾わない", () => {
      expect(extractSectionRefs("`repo-ops` を読む")).toEqual([]);
    });
  });
});
