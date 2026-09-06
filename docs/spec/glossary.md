# Glossary — Business Vocabulary Spec

> このシステムの**業務語彙**（Ubiquitous Language）の正本。1 つの語に 1 つの意味、1 つの名前を、
> 会話と spec とコードで共有するための統括 spec。
> 生成の支援は [`/glossary`](../../.claude/skills/glossary/SKILL.md) が行う。**正名は決めない。**

## Overview

業務語彙はここ、`docs/spec/` に住む。`README.md` や `docs/adr/**` には住まない。あちらが述べるのは
実装の構造と、その構造を選んだ意思決定である。**層 README へ育ってしまった業務語は、家を出た語である。**
同じ語がどこか別の場所で定義し直され、それに誰も気づかない。

feature ごとの spec だけでは足りず、その理由がこのページの存在理由そのものである。**Evans が最も
警戒すること——1 つの語が 2 つの意味を持つ、2 つの語が 1 つの意味を指す——は feature を跨いだときにしか
起きない。** feature 単位の spec しか無いリポジトリが持っているのは方言の集合であって、言語ではない。

**この表が上であり、コードが下である。** 語の意味は業務が決め、モデルはそれを表現できるよう作られる。
両者が食い違ったとき、疑うのはコードの側である——ここの行を実装に合わせて書き換えるのは、
**業務が何を話しているかをコードに決めさせる**ことであり、それはこのページが存在する理由の否定になる。

このプロジェクトは既に 2 度踏んでいる。どちらも検査ではなく偶然の発見だった。`value object` が
`internal/domain` と `pkg/` で違う意味を持っていた件と、`kernel` が 3 つの意味を持っていた件である。

## How a term earns a row

**業務を知っている人が「これは業務の語だ」と認める**とき、その語は行を得る。これはドメイン層が
「この条件は基準か」を判定するときと同じ問いであり、同じであることは意図している。

構造や機構の名前は、コード中でどれだけ目立っていても該当しない。`Attributes`・`Repository`・
`ListParams`・`FeedCursor` が述べているのはこのシステムの作りであって、業務が話していることではない。
それらは [Mechanism vocabulary](#mechanism-vocabulary) に記録し、生成器が繰り返し提案しないようにする。

**行為と、その行為が残す状態は別の語である。** 業務が「◯◯する」という行為と「◯◯済み」という状態を区別して話すなら
行は 2 つ要る。片方だけを持つ表は起きたことと今の姿を 1 語に兼ねさせ、**いつ起きたのかを問えなくする。**
どちらが公開契約に出るかも一致しない——行為が API の操作として現れ、状態は表に出ないことがある。

| 列 | 何を書くか |
| --- | --- |
| 用語 | 業務が言うとおりの語 |
| 定義 | ドメインエキスパートが受け入れる 1 文。実装の語彙を使わずに書く |
| 所有 | 意味を所有する feature と、あれば集約。**ちょうど 1 つ**。2 つあるならその衝突こそが findings。集約を持たない投影は feature だけを書く——語を定義していないモデルに帰属させないため。集約横断の語彙（`internal/domain/lexicon`）は設計上どの feature のものでもないので `lexicon` と書き、これは衝突ではない |
| コードシンボル | それを担う公開識別子（`package.Type` / `package.Type.Method` / `package.Func` / `package.Value`）。`query` のようにパッケージ名だけでは一意にならない場合は親を添える（`dashboard/query.SalesResult`）。**識別子だけを書く**——注記を添えると、この列を突き合わせる検査が通らなくなり、統べる主張が検査できないものになる |
| 公開名 | 公開契約（OpenAPI schema / field）での名前。表に出ない語は空 |

公開名が空なのは普通である。すべての業務語が API 境界を越えるわけではない。**コードシンボルが空なのは
findings** — 業務が使っていてモデルが使っていない語は、実装漏れか、そもそも語ではないかのどちらか。

## Terms

<!-- sample-api:begin -->
以下の行はサンプル機能一式に由来し、**サンプルを撤去すると消える**。残るのはこのページと、ここに書かれた
規則のほうである。言語はこのプロジェクトを運用する側のものである。

| 用語 | 定義 | 所有 | コードシンボル | 公開名 |
| --- | --- | --- | --- | --- |
| 商品 | 顧客が購入できる品目。売値と在庫を持ち、公開されている間だけ顧客の目に触れる | product / Product | `product.Product` | `ProductResponse` |
| 商品カテゴリ | 顧客が商品を探すときの分類。商品はちょうど 1 つに属する | product-category / ProductCategory | `category.Category` | `ProductCategoryResponse` |
| 商品ステータス | 商品を今どう取り扱っているかの区分 | product-status / ProductStatus | `status.Status` | `ProductStatusResponse` |
| 公開中 | 顧客がその商品を見つけて購入できる状態 | product / Product | `product.IsPublished` | — |
| 廃番 | その商品の取り扱いを終えた状態。取り消せず、廃番の商品は公開中になれない | product / Product | `product.IsDiscontinued` | `discontinuedAt` |
| クーポン | 利用者に生じた事情への補償として発行される値引きの権利。受給者は発行時に確定し移らない | coupon / Coupon | `coupon.Coupon` | — |
| 値引き | クーポンがいくら引くかの決まり方と、その値の組。定額と定率がある | coupon / Coupon | `coupon.Discount` | — |
| 適用範囲 | クーポンがどの明細を対象にするか。全体・カテゴリ限定・商品限定がある | coupon / Coupon | `coupon.Scope` | — |
| 引き換え | 保有するクーポンを購入へ適用し、使用済みにする行為。一度きりで取り消せない | coupon / Coupon | `coupon.Coupon.Redeem` | `usedAt` |
| 対象明細 | クーポンの適用範囲に入る明細。値引き額はこの小計から決まる | coupon / Coupon | `coupon.Line` | — |
| 値引き額 | 引き換えによって請求から差し引かれる額。決済スケールの整数 | purchase / Purchase | `purchase.Purchase.DiscountAmount` | `discount_amount` |
| 在庫僅少 | 補充しなければ品切れが近い水準まで在庫が減っている状態 | product / Product | `product.Product.IsLowStock` | — |
| 代表画像 | 商品を 1 枚で表すときに使う画像。出品者が並べた表示順の先頭がこれにあたる | product / Product | `product.Product.PrimaryImage` | `imagePath` |
| 購入 | 顧客が商品を買った事実。何をいくらで買ったかが確定している | purchase / Purchase | `purchase.Purchase` | `PurchaseResponse` |
| 購入明細 | 購入 1 件に含まれる商品ごとの行。単価は買った時点の値で確定し、後の値上げに追随しない | purchase / Purchase | `purchase.PurchaseDetail` | `PurchaseDetailResponse` |
| 購入コード | 顧客との問い合わせでその購入を指し示すための符号 | purchase / Purchase | `purchase.Purchase.Code` | `code` |
| キャンセル済み | 購入が取り消され、履行されないことが確定した状態 | purchase / Purchase | `purchase.Purchase.IsCanceled` | — |
| 発送可能 | 購入が発送してよい状態。支払いを終え、まだ発送していないことを指す | purchase / Purchase | `purchase.Purchase.IsShippable` | — |
| まとめ発送 | 発送待ちの購入のうち、1 便にまとめて発送してよい組 | purchase / Purchase | `dispatch.GroupForDispatch` | — |
| ユーザー | このサービスで商品を購入する人 | user / User | `user.User` | `UserResponse` |
| 在籍 | ユーザーがこのサービスの利用を続けている状態 | user / User | `user.User.IsActive` | — |
| 都道府県 | 住所を広域で区分する単位 | prefecture / Prefecture | `prefecture.Prefecture` | `PrefectureResponse` |
| メールアドレス | このサービスがそのユーザーへ連絡を届けるための宛先。1 人に 1 つ | user / User | `user.Email` | `email` |
| 郵便番号 | 住所を配達の区域まで絞り込む符号 | user / User | `user.PostalCode` | `postalCode` |
| 金額 | 売り買いでやり取りされる貨幣の量。商品の売値も購入明細の単価もこれで表す | lexicon | `money.Price` | — |
| 住所候補 | 郵便番号から引ける住所のうち、その番号が指しうるもの | address | `address.Candidate` | `AddressCandidate` |
| 売上 | ある期間に成立した購入の金額の合計 | dashboard | `dashboard/query.SalesResult` | `salesAmount` |
| 集計期間 | 集計の対象として区切る時間の範囲 | 横断（dashboard / purchase / product-ranking） | `timewindow.Window` | `orderedAfter` / `orderedBefore` |
| 為替レート | ある通貨を別の通貨へ換算するときの比率 | exchange-rate | `exchangerate.Rate` | — |
| 参考換算額 | 表示のために別通貨へ換算した金額。支払いに使う値ではない | exchange-rate | `exchangerate.ReferenceAmount` | `ReferenceAmount` |
| 販売数量ランキング | 売れた数の多い順に商品を並べたもの | product-ranking | `ranking.QuantityRankingView` | `ProductQuantityRankingItem` |
| 売上金額ランキング | 売れた金額の多い順に商品を並べたもの。母集団は販売数量ランキングと同一で、指標と並び順だけが違う | product-ranking | `ranking.AmountRankingView` | `ProductAmountRankingItem` |
| ユーザー検索 | 条件に当てはまるユーザーを探し出すこと | user-search | `search.UserSearchResult` | — |
| 未処理 | 購入が成立した直後の、まだ何も進んでいない状態 | purchase / Purchase | `purchase.StatusUnprocessed` | — |
| 受付中 | 購入の注文を受け付け、内容の確認に入る前の状態 | purchase / Purchase | `purchase.StatusAccepted` | — |
| 確認中 | 受け付けた注文の内容を確認している状態 | purchase / Purchase | `purchase.StatusConfirming` | — |
| 処理中 | 支払いを終え、発送に向けた処理を進めている状態 | purchase / Purchase | `purchase.StatusProcessing` | — |
| 支払い済み | 購入の代金が支払われた状態 | purchase / Purchase | `purchase.StatusPaid` | — |
| 発送済み | 購入された商品が顧客へ向けて送り出された状態 | purchase / Purchase | `purchase.StatusShipped` | — |
| 配達済み | 購入された商品が顧客に届いた状態。ここから先へは進まない | purchase / Purchase | `purchase.StatusDelivered` | — |
| 購入完了 | 購入が果たされた状態。以後は取り消せない | purchase / Purchase | `purchase.StatusCompleted` | — |
| 進行中 | 購入が成立してから、まだ決着していない状態。購入完了・配達済み・キャンセル済みのいずれにも至っておらず、この先の状態がまだあり得る。担い手は決着済みかを問う述語で、進行中はその否定である | purchase / Purchase | `purchase.Status.IsTerminal` | — |
| 支払い | 顧客が購入の代金を払うこと。これが起きた購入は支払い済みになる | purchase / Purchase | `purchase.Purchase.Pay` | `pay` |
| 発送 | 購入された商品を顧客へ向けて送り出すこと。これが起きた購入は発送済みになる | purchase / Purchase | `purchase.Purchase.Ship` | `ship` |
| 配達 | 購入された商品が顧客の手元に届くこと。これが起きた購入は配達済みになる | purchase / Purchase | `purchase.Purchase.Deliver` | `deliver` |
| キャンセル | 購入を取り消し、履行しないと決めること。これが起きた購入はキャンセル済みになる | purchase / Purchase | `purchase.Purchase.Cancel` | `cancel` |
| 管理者 | 一般の利用者には許されない操作を行える役割 | user / User | `user.RoleCodeAdmin` | `admin` |
| 退会 | ユーザーがこのサービスの利用をやめること | user / User | `user.User.MarkAsDeleted` | — |
| 購入可能 | 在籍しているユーザーにだけ認められる、購入を受け付けてよい状態 | membership | `membership.EnsurePurchasable` | — |
| 退会可能 | 進行中の購入を残していないユーザーにだけ認められる、退会してよい状態 | membership | `membership.EnsureWithdrawable` | — |
| 在庫の増減 | 購入の成立や取り消しによらず、補充または差し引きとして商品の在庫数を増減させること | product / Product | `product.Product.AdjustStock` | — |
| カート | 顧客が買うつもりの商品を、購入を確定させるまで入れておく控え。入れても商品は取り置かれない | cart / Cart | `cart.Cart` | `CartResponse` |
| 問い合わせ | 利用者が運営に対して開始し、運営が応答する一連のやり取り。利用者は同時に 1 件だけ持つ | inquiry / Inquiry | `inquiry.Inquiry` | — |
| 問い合わせメッセージ | 問い合わせの中で一方が相手に送る 1 通。送った後に直すことも取り消すこともない | inquiry / Message | `inquiry.Message` | — |
| 回答者 | 利用者の問い合わせに応答する運営側の担当者 | inquiry / Message | `inquiry.AuthorKindOperator` | — |
<!-- sample-api:end -->

## Mechanism vocabulary

以下の公開名は業務語**ではない**。生成器が提案し続けないため、そして読者がコード上の目立ちを業務上の
重要さと取り違えないために記録する。

| 名前 | 業務語でない理由 |
| --- | --- |
| `Attributes` / `Profile` | 位置引数の取り違えを防ぐための引数の束 |
| `PurchaseDetailAttributes` | 位置引数の取り違えを防ぐための引数の束   <!-- sample-api:line --> |
| `Repository` / `LockRepository` | 永続化の抽象。業務はリポジトリを持たない |
| `RoleRepository` | 永続化の抽象。業務はリポジトリを持たない   <!-- sample-api:line --> |
| `ListParams` / `Counts` | 読み取り経路の問い合わせとページングの形 |
| `ListFeedParams` / `FeedCursor` / `FeedItem` | 読み取り経路の問い合わせとページングの形   <!-- sample-api:line --> |
| `DetailInput` / `LockedProduct` | コンストラクタの入力であって、業務が名前を付けるものではない   <!-- sample-api:line --> |
| `Event` / `EventType` | 事実の封筒。事実の**名前**は語だが、封筒は語ではない |
| `StatusRef` / `CategoryRef` | 同一性に表示用の属性を添えた集約横断参照   <!-- sample-api:line --> |
| `Detail` | 購入 1 件の読み取り形。業務が呼ぶ語は書き込み側の集約が既に持っている（購入・購入明細）   <!-- sample-api:line --> |
| `DBHealth` / `Ok` / `Degraded` / `Unhealthy` | 稼働の観測点。業務ではなく運用が見るもの |
| `New` / `New<VO>` | 構築の入口。名前は規約であって業務の語ではない。業務が同じ行為を別の名前で呼ぶなら、その語のほうが行になる |
| `Reconstruct` | 永続化からの再構成。業務は既に起きた事実を作り直さない。復元は保存の裏返しであって行為ではない |
| `CanTransitionTo` / `IsZero` | 状態機械の機構。**遷移そのものは業務の語だが、遷移可否の問い方は違う** |
| `EnsureVersion` | 同時更新の検出。業務は版を持たない |
| `Decimal` / `String` / `ToMinorUnit` | 金額の表現変換 |
| `Update` / `UpdateProfile` | 業務上の出来事ではなく属性の置換。状態も動かさず事実も残さない。付随する規則は状態を表す行が担う |
| `StatusCode` / `Type` | 値の符号と種別の取り出し |

次のサフィックスは、それが付いているというだけで機構語である。個別の名前は機能が増えるたびに
新しく現れるので、**名前だけを列挙していては抑止が永久に追いつかない。**

| サフィックス | 何を述べているか |
| --- | --- |
| `Result` / `ReadModel` / `View` | 問い合わせの答えの形 |
| `Input` / `Params` | 呼び出しに渡す値の束 |
| `Summary` / `Breakdown` / `Count` / `List` / `Item` | 集計と並びの形 |
| `Cursor` | ページングの位置 |
| `DTO` | 層をまたぐ運搬の器 |

規約から機械的に生える名前の族——不変条件違反の sentinel、境界値の定数、入力フィールドの識別子——も
業務語ではない。**ここには書き写さない。** どの族がそれに当たるかは
[`.claude/scaffold-spec/domain-spec.md`](../../.claude/scaffold-spec/domain-spec.md) が
「spec に書かない自動派生」として既に宣言しており、同じ事実を 2 箇所に置けば、読み返されないほうが
黙って腐る。生成器はあちらを読んで差し引く。

`Usecase` / `Gateway` / `QueryService` / `Repository` そのものである名前も同じである。これらは
アーキテクチャの継ぎ目を指しており、サフィックスを落とすと何も残らない——それが見分けである。

ここにある名前とサフィックスは昇格しうる。業務が使い始めたら上の表へ移す。**この一覧は判断の記録で
あって、恒久的な除外ではない。**

## 語彙を改訂するとき

モデルを作っていると、業務が 1 語で呼んでいたものが実は 2 つだった、と分かることがある。**そのとき
先に直すのはこの表であって、コードではない。** 語を分け、それぞれの定義を書き、所有を決め、
そのあとでモデルがその区別を表現するよう変わる。

順序が逆になると——コードで型を 2 つに割ってから表を追認すると——**表はコードの索引に退化する。**
索引はコードと矛盾できず、矛盾できない語彙はモデルの誤りを指摘できない。このページの価値は
コードに反対できることの中にしかない。

改訂の入口は 3 つある。いずれも人間が行う。

- 実装中に見つかった区別（上記）
- [Watch list](#watch-list) に積まれた語が決着したとき
- [`/glossary`](../../.claude/skills/glossary/SKILL.md) が報告する新出語・orphan・同音異義

## Watch list

このページが捕まえるために在る 2 つの失敗。どちらも単一の feature からは見えない。

- **同音異義** — 1 つの識別子、2 つの feature で違う定義。識別子の衝突は機械で検出できるが、定義が
  実際に違うかは読み取りであり、改名するかどうかは判断である
- **同義** — 2 つの語、1 つの概念。機械では検出できない。2 つの行を読んだ人が「これは同じものだ」と
  気づいたときに表面化する。**この表が生成されるだけでなく読まれることを前提にしているのはそのため**

### 同音異義（未決）

<!-- sample-api:replace-begin -->
**どちらの名前が勝つかも、2 つが本当に別概念かも、ここでは決めない。** 決めるのは業務がどう話すかの
決定であり、人のものである。

- **`Status`** — `product/status.Status` は商品の取扱区分を表す参照マスタの実体（`id` / `name` /
  `code` / `sortKey` を持ち、表示順があり、書き込み API を持たない）。`purchase.Status` は購入の
  進行段階を表す値オブジェクト（許可遷移と終端性を内蔵し、doc は「符号の大小で判定してはならない」と
  明言）。**片方は集約、片方は値オブジェクトで、指しているものが違う。**用語表は前者にだけ
  「商品ステータス」の行を与えており、後者は個々の状態（未処理・支払い済み…）の行だけを持つ。
  さらに `purchase/status.Status`（購入ステータスマスタの実体）が加わり、衝突は 2 方向になった。
  `purchase` 配下で値オブジェクトと参照マスタが同じ `Status` を名乗り、かつ素の識別子 `status.Status`
  が product-status と purchase-status の 2 つの feature に跨って重複する。**後者は単一の feature の
  spec からは見えない。**マスタ側だけを改名するか、値オブジェクト側を業務の言葉へ寄せるかは未決

- **`salesAmount`** — `dashboard/query.SalesResult` は購入の支払金額（小計 + 税額 + 送料）を決済スケールの
  セント整数で合計し、商品の公開状態を見ない。`ranking.AmountRankingItemView.SalesAmount` は明細
  （単価 × 数量）を価格スケールの正確な decimal で合計し、公開済み商品に限る。**母集団もスケールも
  違うのに、公開名が同じ `salesAmount` である。**両者が一致しないことは各 op の description に
  書かれているが、名前を分けるかは業務がどう話すかの決定である
<!-- sample-api:replace-with -->
<!-- = **どちらの名前が勝つかも、2 つが本当に別概念かも、ここでは決めない。** 決めるのは業務がどう話すかの -->
<!-- = 決定であり、人のものである。 -->
<!-- = -->
<!-- = 現在エントリなし。 -->
<!-- sample-api:replace-end -->

### 行にするかが未決

<!-- sample-api:replace-begin -->
以下は**同義かどうかが未決**の語である。

- **廃番 / 廃盤 / 販売終了** — 「廃番」は商品集約が持つ状態（`product.IsDiscontinued`、`discontinuedAt`
  由来）で、不変条件と述語を持つ。「廃盤」と「販売終了」は商品ステータスマスタの行
  （`database/migrations/000008_create_product_statuses.up.sql` の code 7 / code 4）で、
  業務規則を一切持たない表示専用のラベルである。**同じ事柄を指す 3 語なのか、取り扱いの終了
  （状態）と表示上の区分（ラベル）という別の物なのかは決めていない。** 廃番の実装はマスタの行を
  書き換えないため、現状は独立に併存する。3 語を統合するか、マスタ側の 2 行を廃番へ連動させるか、
  別概念として残すかは、業務がどう話すかの決定である

以下は**行にするかどうかが未決**の語である。

- **購入の成立** — 支払い・発送・配達・キャンセルは行為の側が行を得たが、購入そのものの成立だけ
  行が無い。構築子（`purchase.New`）は担い手になれない——名前が規約であり、集約によって新規発生と
  復元の両方を意味するからである（[Mechanism vocabulary](#mechanism-vocabulary) 参照）。事実の側には
  `purchase.EventCreated` という名前が既にあり、担い手の候補はそちらである。**行を立てるかは、この
  成立を「購入」という既存の行が兼ねてよいかの判断**であり、兼ねさせるなら 1 語が集約と出来事の
  2 つを指すことになる。
<!-- sample-api:replace-with -->
<!-- = 同義・行にするかどうかが未決の語は、いずれも現在エントリなし。 -->
<!-- sample-api:replace-end -->

## What this document is not

- **データ辞書ではない。** 列・型・制約は feature ごとの spec と migration のもの。ここの行が述べるのは
  語の意味であって、保存のされ方ではない
- **規則の置き場ではない。** 「その語が指すものの値がどう決まるか」は業務知識であり `docs/spec/<layer>/<パッケージパス>.md` に属する。
  このページが述べるのは「その語が何を指すか」である
- **生成物ではない。** `/glossary` は行を提案し欠落を報告するが、文言は人間のものであり、どの名前が
  勝つかという判断もすべて人間のものである
