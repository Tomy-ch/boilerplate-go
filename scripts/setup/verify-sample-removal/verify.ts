// サンプル削除の「過不足なし」を判定する規則。入口（./index.ts）はスナップショット読み込み・
// git / make / grep の起動・終了コードだけを担う。
//
// このモジュールは検証成功後、入口と一緒に消える（理由は selfDestructTargets）。

import path from "node:path";

/** 残留サンプル参照の検出条件。生成物とテストは CI で regen を省くため除外する。 */
export const DANGLING_PATTERN =
  "usercount|userpurge|productimagegc|withdrawalarchive|user_roles|prefecture|inquiry";
export const DANGLING_EXCLUDE = "_test\\.go|\\.gen\\.go";

/** サンプル削除を起動する make ターゲット。`.mk` のマーカー除去で消えるべきもの。 */
const SAMPLE_MAKE_TARGET = "setup-remove-sample-api";

/** 残留サンプル参照を洗い出す shell コマンド。ヒット無しでも非 0 で落ちないようにする。 */
export function buildDanglingCommand(): string {
  return `grep -rniE '${DANGLING_PATTERN}' internal/ cmd/ --include='*.go' | grep -vE '${DANGLING_EXCLUDE}' || true`;
}

/**
 * remove-sample-api が書き出したスナップショットから登録パスを取り出す。
 *
 * @throws JSON として読めない、または `registeredPaths` が配列でない・空の場合。
 */
export function parseSnapshot(json: string): string[] {
  const parsed: unknown = JSON.parse(json);
  const registeredPaths = (parsed as { registeredPaths?: unknown }).registeredPaths;

  if (!Array.isArray(registeredPaths) || registeredPaths.length === 0) {
    throw new Error("スナップショットの registeredPaths が空です");
  }

  return registeredPaths as string[];
}

/** `git status --porcelain` の出力から削除エントリの相対パスを取り出す。 */
export function parseDeletedPaths(porcelain: string): string[] {
  return porcelain
    .split("\n")
    .filter((line) => line.length > 3 && (line[0] === "D" || line[1] === "D"))
    .map((line) => line.slice(3));
}

/** 不足検出: 登録パスがまだ残っていれば「消えていない」。 */
export function findUnremovedPaths(
  registeredPaths: readonly string[],
  pathExists: (relativePath: string) => boolean,
): string[] {
  return registeredPaths
    .filter((relativePath) => pathExists(relativePath))
    .map((relativePath) => `未削除の登録パス: ${relativePath}`);
}

/** 過剰検出: 登録パスに含まれない削除は想定外（サンプル以外を巻き込んでいる）。 */
export function findUnregisteredDeletions(
  registeredPaths: readonly string[],
  deletedPaths: readonly string[],
): string[] {
  const isRegistered = (deletedPath: string): boolean =>
    registeredPaths.some(
      (registered) => deletedPath === registered || deletedPath.startsWith(`${registered}/`),
    );

  return deletedPaths
    .filter((deletedPath) => !isRegistered(deletedPath))
    .map((deletedPath) => `登録外の削除を検出: ${deletedPath}`);
}

/** 削除ツール自身の make ターゲットが `.mk` のマーカー除去で消えていることを確認する。 */
export function findLeftoverMakeTarget(makeHelpOutput: string): string[] {
  return makeHelpOutput.includes(SAMPLE_MAKE_TARGET)
    ? [`make ターゲット ${SAMPLE_MAKE_TARGET} が残っています`]
    : [];
}

/** 残留サンプル参照（実コード）の grep 結果を失敗メッセージへ変換する。 */
export function findDanglingReferences(danglingHits: string): string[] {
  const hits = danglingHits.trim();

  return hits === "" ? [] : [`残留サンプル参照:\n${hits}`];
}

/**
 * 孤立検出の対象外。利用者が自分の operation から参照するために置かれている汎用ブロックで、
 * 現時点でそれを参照しているのがサンプル API だけであるもの。
 *
 * @remarks
 * `openapi/components/schemas/README.md` はいずれも汎用の再利用ブロックとして Directory Contents に
 * 宣言しています（`errors/` は「one per HTTP status covering every `apperror` kind」、
 * ページネーションのメタデータは offset 版と cursor 版）。
 * 撤去すると参照するのは health 系が使う 405 だけになりますが、それは登録漏れではなく、
 * 利用者が使うために残す在庫です。参照ゼロという事実だけではこの在庫と登録漏れを区別できません。
 *
 * このリストは実装の都合ではなく「何を汎用ブロックとして残すか」という宣言なので、
 * 汎用ブロックを増やしたときは合わせて足す必要があり、放っておくとドリフトします。
 * 個別の登録漏れを黙らせる許可リストへ変質させないため、足すときは
 * schemas/README.md がその定義を汎用ブロックとして宣言していることを根拠にすること。
 */
export const ORPHAN_EXCLUDED_PATHS: readonly string[] = [
  "openapi/components/schemas/BearerAuth.yaml",
  "openapi/components/schemas/BearerAuthRegistration.yaml",
  "openapi/components/schemas/errors/",
  "openapi/components/schemas/PaginationMetadataResponse.yaml",
  "openapi/components/schemas/CursorPaginationMetadataResponse.yaml",
];

/** 外部ファイルを指す `$ref` の値を YAML テキストから取り出す。fragment だけの参照は同一ファイル内なので除く。 */
export function extractFileRefs(yamlText: string): string[] {
  const refs: string[] = [];

  for (const match of yamlText.matchAll(/\$ref:\s*["']?([^"'\s]+)/g)) {
    const target = match[1].split("#")[0];
    if (target !== "") {
      refs.push(target);
    }
  }

  return refs;
}

/** `$ref` の値を、参照元ファイルの位置から解決してリポジトリ相対パスにする。 */
export function resolveRef(fromPath: string, ref: string): string {
  return path.posix.normalize(path.posix.join(path.posix.dirname(fromPath), ref));
}

/** entrypoint から `$ref` を辿って到達できるファイルの集合を返す。 */
export function reachableFiles(
  entrypoint: string,
  refsOf: (relativePath: string) => readonly string[],
): Set<string> {
  const reachable = new Set<string>([entrypoint]);
  const queue: string[] = [entrypoint];

  while (queue.length > 0) {
    const current = queue.shift() as string;
    for (const ref of refsOf(current)) {
      const target = resolveRef(current, ref);
      if (!reachable.has(target)) {
        reachable.add(target);
        queue.push(target);
      }
    }
  }

  return reachable;
}

/**
 * 孤立検出: 撤去後も残っているのに、正本 spec から `$ref` で辿れなくなった定義。
 *
 * @remarks
 * 撤去前の到達可否は見ません。撤去前から参照されていない定義は ORPHAN_EXCLUDED_PATHS が
 * 宣言する在庫か、さもなくばそれ自体が報告に値する死んだ定義であり、どちらの扱いも
 * 撤去後の状態だけで決まるためです。
 *
 * この向きは findUnremovedPaths（登録したのに消えていない）と
 * findUnregisteredDeletions（登録していないのに消えた）のどちらも捉えません。
 * マニフェストがディレクトリ単位で消す傘から、ファイル単位で列挙する場所へ定義を移すと、
 * 登録は不要なままに見えて撤去後だけ孤立します。
 */
export function findOrphanedComponents(
  survivingComponents: readonly string[],
  reachableAfter: ReadonlySet<string>,
): string[] {
  const isExcluded = (relativePath: string): boolean =>
    ORPHAN_EXCLUDED_PATHS.some(
      (excluded) => relativePath === excluded || relativePath.startsWith(excluded),
    );

  return survivingComponents
    .filter((relativePath) => !isExcluded(relativePath) && !reachableAfter.has(relativePath))
    .map((relativePath) => `撤去後に孤立した定義: ${relativePath}（撤去対象への登録漏れ）`);
}

export type VerificationInput = {
  registeredPaths: readonly string[];
  pathExists: (relativePath: string) => boolean;
  gitStatusPorcelain: string;
  makeHelpOutput: string;
  danglingHits: string;
  survivingComponents: readonly string[];
  reachableAfter: ReadonlySet<string>;
};

/** 5 種の検査をすべて走らせ、失敗メッセージを 1 本の配列にまとめる。 */
export function collectFailures(input: VerificationInput): string[] {
  return [
    ...findUnremovedPaths(input.registeredPaths, input.pathExists),
    ...findUnregisteredDeletions(input.registeredPaths, parseDeletedPaths(input.gitStatusPorcelain)),
    ...findLeftoverMakeTarget(input.makeHelpOutput),
    ...findDanglingReferences(input.danglingHits),
    ...findOrphanedComponents(input.survivingComponents, input.reachableAfter),
  ];
}

/**
 * 検証成功後に消す対象を返す。
 *
 * @remarks
 * このツールはサンプル削除の最終地点なので、自身のディレクトリごと消えます。ファイルを 1 本ずつ
 * 挙げていると、判定モジュールやそのテストを足したときに列挙から漏れ、消えたはずの検証ツールの
 * 一部だけが利用者のリポジトリへ居座ります。スナップショットは 1 階層上の共有位置にあるため別に挙げます。
 */
export function selfDestructTargets(selfDir: string, snapshotPath: string): string[] {
  return [snapshotPath, selfDir];
}
