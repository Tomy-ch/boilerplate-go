---
status: accepted
date: 2026-07-24
deciders: [maintainers]
tags: [architecture]
---

# ADR-0039: 集約横断の値オブジェクトはキュレートされたドメイン lexicon に置く

## ステータス

accepted

## 背景

<!-- sample-api:replace-begin -->
`money.Price`（[ADR-0038](0038-two-scale-quantity-model.ja.md)）の導入により、レイヤールールの欠落が表面化した。`Price` はビジネス上の意味を持つ値オブジェクト（非負性、最小単位への換算）であり、**複数の集約**で共有される。
<!-- sample-api:replace-with -->
<!-- = 決済スケールの値オブジェクト（[ADR-0038](0038-two-scale-quantity-model.ja.md)）の導入により、レイヤールールの欠落が表面化した。それ自身がビジネス上の意味を持ち、**複数の集約**で共有される。 -->
<!-- sample-api:replace-end -->
<!-- 撤去後にこの箇所へ自分の例を置くための指針。
     目的: 「複数の集約から使われる」が抽象のままだと、入場基準を満たす実例が示せない。
     意義: 効くのは利用者が 2 つ以上あることで、型そのものの複雑さではない。
     書き方: その値オブジェクトを使う集約側のフィールドを 2 つ以上挙げる。 -->
<!-- sample-api:begin -->
サンプルでの利用者は `product.price` と `purchase_details.unit_price` / `purchases.*_amount`。
<!-- sample-api:end -->
したがって単一の集約パッケージに置くことはできず（他の集約がそこへ手を伸ばすか、VO を重複させることになる）、`pkg/` にも置けない（`pkg/` はビジネスロジックを禁じ、コンテキスト非依存を保たなければならない）。

既存のルール——「ドメイン層に許可された `internal/` 依存は `internal/apperror` のみ」——は、*ドメイン集約をまたいで*共有される値オブジェクトを想定していなかった。文字どおり読めば自然な配置を禁じてしまう。加えて depguard のルールは domain→domain について `lax` だったため、*任意の*集約横断 import（ある集約が別の集約へ直接手を伸ばす形）をサイレントに許してしまう、潜在的な結合の穴があった。こうした共有値オブジェクトをどこに置き、境界をどう強制するかの決定が必要である。

## 決定

集約横断でビジネス上の意味を持つ値オブジェクトは、`internal/domain/lexicon/` に置く**キュレートされたドメイン lexicon**に住まわせる。他のドメインパッケージはこれを import **してよい**。lexicon 自身は `pkg/**` と `internal/apperror` にのみ依存し、集約には決して依存しない。

この名前は受け入れの問いそのものを担っている: そこに属するのは**ビジネスの語彙**であって、単に 2 箇所で使われているものではない。受け入れ基準は意図的に狭く、**すべて**を満たす型だけが属する:

- **値オブジェクト / ドメイン概念**であること。集約でも、単一の集約に紐づくサービスでもないこと。
- 実際に**2 つ以上の集約**で使われていること（あるいは間近にそうなること）——「いつか再利用するかもしれない」ではない。
- `pkg/` に置くことを妨げる**ビジネスセマンティクス**を伴うこと（通貨、非負性、最小単位、税ルール、…）。
- 追加が**共同所有**の決定であること——ここでの変更は依存するすべての集約に波及するため、保守的に行われる。

境界は depguard（`maintain_a_sound_domain`）が強制する: ドメインのファイルに対して `internal/domain/` を deny し、`internal/domain/lexicon` を明示的に allow する。これにより domain→lexicon は許可され、domain→他の集約は禁止される（従来の lax な穴が塞がれる）。

配置は**まず `pkg/` から**解決する: `pkg/` のバーは機械的に強制される（depguard `independent_pkg`）のに対し、こちらのバーは散文であり、散文のバーは linter が引く境界を越えて型を押し出すことはできない。`pkg/` に落ちたことは lexicon の根拠にはならない——lexicon は lexicon 自身の問いを立てるのであり、これをフォールバック扱いにすれば代わりに `pkg/` ががらくた入れになってしまう。どちらのバーも通らない型は、その集約に留まる。

## 影響

### ポジティブな影響

- パス自体が意図を示す: `internal/domain/lexicon/<vo>` は「共有され、import 可能」と読めるため、集約横断の import が違反に見えなくなる（この ADR のきっかけとなったレビュー時の混乱）。
- depguard は lexicon を**許可**しつつ、場当たり的な集約間の結合を**禁止**するようになった——従来の lax な domain→domain よりも厳格で明確な境界である。
- `money` のセマンティクスがドメインに一度だけ存在し、重複なく、かつ `pkg/` へビジネスロジックを漏らすことなく、複数の集約から再利用される。

### ネガティブな影響

- 共有 lexicon は結合点である: lexicon パッケージの変更は依存するすべての集約に影響し得るため、保守的に進化させなければならない（これは受け入れバーが管理するコストである）。
- 貢献者が学ぶ配置の概念が 1 つ増える（集約 vs lexicon）。本 ADR、`docs/rules.md`、`internal/domain/lexicon/README.md` に記載する。

## 検討した代替案

### `money` をフラットな `internal/domain/money` の集約レベルパッケージとして維持する

却下: そのパスは集約と区別がつかないため、domain→domain の import が違反に見え続ける。また depguard は、共有パッケージを個別に列挙する（壊れやすい）か、domain→domain を丸ごと再び開く（lax な穴）かのどちらかをしない限り、これをきれいに許可できない。

### 汎用の `internal/domain/shared`（または `common` / `util`）パッケージ

却下: 決め手は**その名前が入口でどんな問いを立てるか**である。`shared` / `common` / `util` は「これは 2 箇所以上で使われているか?」と問うが、再利用されるものなら何でも yes と答えられる——結果としてパッケージは無関係なコードで埋まり、がらくた入れになる。`lexicon` は「これはビジネスの語彙か?」と問い、汎用のヘルパーはこれに yes と答えられない。上記の厳格な基準が線を守るのだが、それを定着させるのは名前である。

### DDD の *Shared Kernel* を名乗る `internal/domain/kernel`

再考の末に却下（この ADR は当初これを選んでいた）。Evans において Shared Kernel は **Context Map の関係**であり、2 つの Bounded Context が共同所有するモデルの部分集合を指す。すなわち複数の Bounded Context の存在を前提とする。本リポジトリは単一のモデルを持ち、共有するのは*集約*間であるため前提が成り立たず、その用語は持っていない意味のもとで占有されることになる。この用語が担う規律——小さく保つ、変更は共同の決定として行う——は上記でそれ自体の言葉として保持し、`kernel` は実際にその構造を導入する者のために空けておく。

### `money` を `pkg/` に置く

却下: `pkg/` はビジネスロジックを禁じ、コンテキスト非依存でなければならない。通貨 / 非負性 / 最小単位はドメインのセマンティクスである。`pkg/` に属するのは汎用の decimal コンテナ（`pkg/decimal`、[ADR-0038](0038-two-scale-quantity-model.ja.md)）だけである。

## 補足

<!-- sample-api:replace-begin -->
- lexicon パッケージ: `internal/domain/lexicon/money`（`Price`）。
<!-- sample-api:replace-with -->
<!-- = - lexicon パッケージ: まだ受け入れたものは無い。`internal/domain/lexicon/` は受け入れバーだけを持つ。 -->
<!-- sample-api:replace-end -->
- 強制: `.golangci.yaml` の depguard `maintain_a_sound_domain`（`internal/domain/` を deny、`internal/domain/lexicon` を allow）。
- 受け入れバー: `internal/domain/lexicon/README.md`、レイヤールール: `docs/rules.md`。
- これが可能にする 2 スケール数量モデル: [ADR-0038](0038-two-scale-quantity-model.ja.md)。
