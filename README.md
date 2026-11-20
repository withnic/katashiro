# katashiro（形代）

日本の Web サービス・業務システム（決済・KYC・会員管理・バッチ処理）のテスト向けに、
**最大 100 万件の重複なし・整合性のあるダミー個人データ**を決定論的に生成・払い出しする
Go 製シングルバイナリツール（CLI + HTTP API サーバー）です。

- 住所・姓名の小さな辞書だけを持ち、個人データは実行時に `O(1)` で合成（実データの辞書は `master fetch` で取得）
- Cycle-Walking Feistel（20bit・4ラウンド）で `0..999,999` を全単射置換し、100 万件で重複ゼロを保証
- `--seed` と `--base-date`、使う辞書が同じなら、いつどこで実行しても同じ出力
- 全銀協カナ・修正ヘボン式ローマ字・Luhn・モジュラス11 などの形式に準拠
- `CGO_ENABLED=0` の Pure Go（SQLite 出力も `modernc.org/sqlite`）

## Installation

[GitHub Releases](../../releases) から OS / アーキテクチャ別のビルド済みバイナリ（linux / darwin / windows × amd64 / arm64）を取得できます。
ソースからビルドする場合は次のとおりです。

```sh
CGO_ENABLED=0 go build -ldflags="-w -s" -o katashiro ./cmd/katashiro
# または
go install ./cmd/katashiro
```

```sh
# Tips: エイリアス設定 (~/.zshrc や ~/.bashrc 等に追加)
alias kata=katashiro
```

## Quick Start

```sh
# 重複なしで 10 件を JSON で払い出す（カーソルが進む）
katashiro next 10 --format=json

# テストスイート用に 2,000 件を決定論的に生成（カーソルとは独立）
katashiro export 2000 --seed=42 --base-date=2026-01-01 --format=jsonl

# 全モジュール付きで SQLite に 100 万件出力
katashiro export 1000000 --out=personas.sqlite --with-finance --with-kyc --with-profile
```

## CLI

| サブコマンド | 説明 |
| :-- | :-- |
| `next <N>` | プロファイルのカーソル位置から重複なしで N 件を払い出し、カーソルを進める。100 万件の上限に達すると **Exit Code 2** で終了する（`--loop` でカーソル 0 にラップアラウンド）。 |
| `export <N>` | カーソルと独立にインデックス `0..N-1` を一括出力する（最大 100 万件）。 |
| `reset` | `--profile` のカーソルを 0 に戻す。 |
| `serve` | HTTP API サーバーを起動する。 |
| `master fetch` | 日本郵便の郵便番号データと gimei の人名データを取得・検査し、実データの辞書ディレクトリを作る（下記）。 |
| `master info` | 使う辞書の件数・出典・フィンガープリントを表示する。 |

### 主なフラグ

| フラグ | 対象 | 説明 |
| :-- | :-- | :-- |
| `--format` | next / export | `json`（既定）/ `jsonl` / `csv`、export のみ `sqlite`。export で省略すると `--out` の拡張子から推定する |
| `--profile=<name>` | next / reset | カーソルのプロファイル名（既定 `default`） |
| `--loop` | next | 上限到達時にラップアラウンドする |
| `--out=<path>` / `--force` | export | 出力先。既存ファイルは `--force` 指定時のみ上書き（SQLite は `DROP TABLE IF EXISTS personas` で再作成） |
| `--seed=<int64>` | 共通 | 乱数シード（既定 `1`） |
| `--base-date=<YYYY-MM-DD>` | 共通 | 年齢計算の基準日（既定 `2026-01-01`、`today` で実行日） |
| `--with-finance` / `--with-kyc` / `--with-profile` | next / export | オプションモジュールを含める |
| `--set key=value` | next / export | スマートオーバーライド（複数指定可、下記） |
| `--set-raw key=value` | next / export | 整合チェックなしで上書き（異常系テスト用、下記） |
| `--offset=<M>` | export | M 人目（0 始まり）から出力する。条件を変えて分割生成するとき用 |
| `--min-age` / `--max-age` | 共通 | 年齢範囲（既定 18〜80） |
| `--strict-phone` | 共通 | 安全帯域 `0X0-0XXX-XXXX` ではなく実在プレフィックス帯 `0X0-1XXX-XXXX` を使う |
| `--master-dir=<dir>` | 共通 | 外部の辞書ディレクトリを使う（`master fetch` で作ったもの、または `addresses.json` / `names.json`）。未指定時は環境変数 `KATASHIRO_MASTER_DIR`、それもなければ埋め込み辞書 |

フラグは位置引数の前後どちらにも書けます（`katashiro next 10 --format=csv` / `katashiro next --format=csv 10`）。

### 終了コード

| コード | 意味 |
| :-- | :-- |
| 0 | 成功 |
| 1 | 引数エラー・入出力エラー |
| 2 | 100 万件の上限到達（`next` で `--loop` なし） |

### 状態ファイル

`next` のカーソルは `cursor_<profile>.json` として保存され、`gofrs/flock` で排他制御されます。保存先は次の順に決まります。

1. `KATASHIRO_STATE_DIR`
2. `$XDG_STATE_HOME/katashiro`
3. `~/.local/state/katashiro`（Linux / macOS / WSL）、`os.UserConfigDir()/katashiro`（Windows）

## スマートオーバーライド（`--set key=value`）

### 上流連動キー（関連フィールドを自動で整合）

| キー | 効果 |
| :-- | :-- |
| `personal.age=<N>` | 基準日時点でちょうど満 N 歳となる 365（366）日の範囲から seqID ごとに生年月日を決める。18 歳未満は運転免許証 `null`・未婚・扶養 0、職業・年収も年齢に連動 |
| `personal.birthday=<YYYY-MM-DD>` | 生年月日を固定し、基準日時点の満年齢を逆算する（下流モジュールも連動） |
| `personal.gender=<male\|female\|other\|unspecified>` | 性別を固定する。`male` / `female` は名もその性別の辞書から選ぶ。`other` / `unspecified`（「その他」「回答しない」のテスト用）は男女どちらの辞書からも選ぶ。既定の生成では `male` / `female` のみ |
| `personal.last_name=<姓>` / `personal.first_name=<名>` | 同姓同名テスト向け。**読みの併記が必須**（`personal.last_name_hiragana=<よみ>` か `personal.last_name_katakana=<ヨミ>`。名も同様）。読みからカタカナ・全銀協カナ・ローマ字まで揃える。名前から性別は決めない |
| `profile.is_married=<true\|false>` | 婚姻の有無を固定し、扶養人数も連動させる（`--with-profile` が必要）。18 歳未満を含む設定で `true` を指定するとエラー |
| `kyc.document_type=<drivers_license\|passport>` | 本人確認に提示する書類を固定する（`--with-kyc` が必要）。既定では 18 歳以上の約 8 割が免許証、それ以外はパスポート。18 歳未満を含む設定で `drivers_license` を指定するとエラー |
| `contact.prefecture=<都道府県名>` | 該当都道府県の住所だけから抽選し、郵便番号・市区町村・町域・運転免許証の公安委員会コード（例: 福岡県 `90`）まで揃える |

```sh
katashiro export 100 --set personal.age=17 --with-kyc --with-profile   # 免許証 null・未婚・扶養0
katashiro export 100 --set contact.prefecture=福岡県 --with-kyc          # 免許証番号は 90 から始まる
katashiro export 100 --set personal.last_name=佐藤 --set personal.last_name_hiragana=さとう \
                     --set personal.first_name=太郎 --set personal.first_name_katakana=タロウ   # 全員「佐藤 太郎」（メールは一意）
katashiro export 100 --with-kyc --set kyc.document_type=passport         # 全員パスポートで本人確認
```

### テスト観点別の例

| 観点 | コマンド |
| :-- | :-- |
| 未成年（0〜17 歳） | `katashiro export 100 --min-age=0 --max-age=17 --with-kyc --with-profile` |
| 成人（18〜64 歳） | `katashiro export 100 --min-age=18 --max-age=64` |
| 高齢者（65 歳以上） | `katashiro export 100 --min-age=65 --max-age=100` |
| 年齢の境界値 | `--set personal.age=17` / `18` / `20` / `65`、誕生日当日は `--set personal.birthday=2008-01-01` |
| 性別 | `--set personal.gender=female`（`other` / `unspecified` も可） |
| 婚姻 | `--with-profile --set profile.is_married=true` |
| 本人確認書類 | `--with-kyc --set kyc.document_type=drivers_license` / `passport` |
| 同姓同名 | `--set personal.last_name=佐藤 --set personal.last_name_hiragana=さとう --set personal.first_name=太郎 --set personal.first_name_hiragana=たろう` |
| 地域 | `--set contact.prefecture=北海道` |

### 単純上書きキー

上記以外のキーは、生成後の最終ステップで対象フィールドだけを上書きします。読み仮名やローマ字への逆変換はしません。
キーは JSON パス（例: `personal.full_name`、`finance.credit_card.number`、`profile.is_married`）とフラット列名（例: `finance_bank_code`）のどちらでも指定できます。
モジュールのフィールドを上書きする場合は、そのモジュールのフラグ（例: `--with-finance`）も必要です。

```sh
katashiro export 5 --set "personal.full_name=山田 太郎" --set "profile.company_name=株式会社テスト" --with-profile
```

### 異常系テスト用の上書き（`--set-raw`）

`--set` は常に整合の取れたデータを作り、矛盾する指定（17 歳で既婚など）はエラーにします。
利用側のシステムが不整合なデータを弾けるかを試したいときは `--set-raw` を使います。
指定した項目だけを生成後にそのまま上書きし、関連項目の再計算や整合チェックはしません。

```sh
katashiro export 10 --with-profile --set personal.age=17 --set-raw profile.is_married=true          # 17 歳で既婚
katashiro export 10 --with-profile --set personal.age=18 \
  --set-raw profile.occupation=医師 --set-raw profile.annual_income_man_yen=1500                    # 18 歳の医師で年収 1500 万円
katashiro export 10 --with-kyc --set personal.age=17 --set-raw kyc.document_type=drivers_license     # 17 歳で免許証を提示
```

### 条件を混ぜたデータを作る

条件ごとに分けて生成し、結合します。`export` をそのまま複数回実行すると毎回 1 人目から出るため、**結合すると同じ人（同じ ID・電話番号など）が重複します**。
`--offset` で開始位置をずらすか、同じプロファイルで `next` を使ってください。

```sh
# パスポート 3 割・免許証 7 割の 1,000 人
katashiro export 300 --with-kyc --set kyc.document_type=passport                     > a.jsonl
katashiro export 700 --with-kyc --set kyc.document_type=drivers_license --offset=300 > b.jsonl

# next の場合（reset から始めれば毎回同じ 1,000 人になる）
katashiro reset --profile=mix
katashiro next 300 --profile=mix --with-kyc --set kyc.document_type=passport
katashiro next 700 --profile=mix --with-kyc --set kyc.document_type=drivers_license
```

HTTP API には事前に `export` で投入しておいたデータを使う方法もあります。テストからは DB の未使用レコードを順に使えば、毎回バラバラの人を取り出せます。

## HTTP API サーバー

```sh
katashiro serve --port=8080               # カーソルはファイル永続（CLI の next と共有）
katashiro serve --port=8080 --in-memory   # オンメモリのアトミックカウンタ
```

| メソッド・パス | 説明 |
| :-- | :-- |
| `POST /api/v1/personas/next?count=10&profile=e2e&with_kyc=true&with_profile=true` | 重複なしで `count` 件を JSON 配列で返す。上限到達時は `409 Conflict`（`loop=true` でラップアラウンド）。`with_finance`、`set=key=value`（複数可）も指定できる |
| `POST /api/v1/personas/reset?profile=e2e` | プロファイルのカーソルを 0 に戻す |
| `GET /healthz` | ヘルスチェック |

```sh
curl -X POST 'http://localhost:8080/api/v1/personas/next?count=3&profile=e2e&with_kyc=true'
curl -X POST 'http://localhost:8080/api/v1/personas/next?count=3&set=contact.prefecture%3D%E7%A6%8F%E5%B2%A1%E7%9C%8C'
```

## Docker

```sh
docker build -t katashiro .
docker run --rm -p 8080:8080 katashiro                     # serve --port=8080 --in-memory
docker run --rm katashiro export 10 --format=csv           # CLI としても使える
```

## 辞書（住所・姓名）

バイナリに埋め込まれているのは、47 都道府県の代表住所約 440 件・姓名約 350 件の**内蔵シード**です。
ネットワークなしでそのまま使え、誰が実行しても同じデータになります。

住所や名前のバリエーションを増やしたいときは、実データの辞書を自分で取得します。

```sh
katashiro master fetch --out=./master   # 日本郵便（住所 約 12 万件）と gimei（姓名 約 1.3 万件）を取得・検査
export KATASHIRO_MASTER_DIR=$PWD/master  # または各コマンドに --master-dir=./master
katashiro master info                     # 件数・出典・フィンガープリントを確認
```

- **自動では使いません。** `--master-dir` か `KATASHIRO_MASTER_DIR` を指定したときだけ使います。知らないうちに人によって違う辞書で生成するのを防ぐためです。
- **チームで同じデータを使うには**、作ったディレクトリ（`addresses.bin`・`names.bin`・`manifest.json`・`NOTICE`）をそのまま共有するか、テスト用のリポジトリにコミットしてください。`master info` のフィンガープリントが同じなら、同じ `--seed` で同じデータになります。
- 日本郵便は最新版だけを配布しているため、`master fetch` を実行した時期によって辞書の中身は変わります。
- 辞書を他の人に渡すときは `NOTICE`（人名データ gimei の MIT ライセンス表示を含む）も一緒に渡してください。
- オフライン環境では、ネットワークのある環境で作った辞書ディレクトリを持ち込んで使います。ダウンロード済みのファイルからも作れます: `katashiro master fetch --ken-all=./utf_ken_all.zip --names=./names.yml`
- 取得したデータのクレンジング結果に問題（括弧の残りなど）があると、辞書を書き出さずにエラーで終わります。

## 出力データ

フィールドの完全な定義は [`docs/02_data_models_and_rules_v2.md`](docs/02_data_models_and_rules_v2.md) を参照してください。

- **personal**: 生年月日・満年齢・性別、氏名（漢字・ひらがな・カタカナ）、全銀協カナ、ローマ字
- **contact**: 郵便番号・住所・建物、電話番号（総務省未割り当ての `0X0-0XXX-XXXX` 帯）、メールアドレス（RFC 2606 の `example.*` ドメイン）
- **finance**（`--with-finance`）: クレジットカード（Visa / Mastercard / JCB は 16 桁・CVV 3 桁、Amex は 15 桁・CVV 4 桁、Luhn 準拠）、銀行口座、ダミー個人番号（`000` 始まり・モジュラス11）
- **kyc**（`--with-kyc`）: 運転免許証番号（公安委員会コード + 取得年 + 交付番号 + 検査数字 + 再発行回数）、パスポート番号
- **profile**（`--with-profile`）: 職業（申込フォーム型の 11 区分）・業種（日本標準産業分類の大分類 20 区分）・勤務先・年収（万円、年齢と連動）・婚姻（配偶者の有無）・扶養人数（扶養している家族の人数の目安）

### 出力例

埋め込み辞書・既定の `--seed=1` / `--base-date=2026-01-01` で実行した結果です（同じ条件なら誰が実行しても同じ出力になります）。

**JSON**（全モジュール付き）: `katashiro export 1 --with-finance --with-kyc --with-profile`

```json
[
  {
    "id": 581155,
    "personal": {
      "birthday": "1978-03-15",
      "age": 47,
      "gender": "male",
      "last_name": "田辺",
      "first_name": "聡",
      "full_name": "田辺　聡",
      "last_name_hiragana": "たなべ",
      "first_name_hiragana": "さとし",
      "full_name_hiragana": "たなべ　さとし",
      "last_name_katakana": "タナベ",
      "first_name_katakana": "サトシ",
      "full_name_katakana": "タナベ　サトシ",
      "bank_holder_kana": "ﾀﾅﾍﾞ ｻﾄｼ",
      "romaji_name": "satoshi.tanabe"
    },
    "contact": {
      "zip_code": "020-0034",
      "prefecture": "岩手県",
      "city": "盛岡市",
      "town": "盛岡駅前通",
      "street_address": "1-16-16",
      "building": "",
      "phone_number": "090-0259-4565",
      "email": "satoshi.tanabe.581154@example.com"
    },
    "finance": {
      "credit_card": {
        "brand": "Visa",
        "number": "4000005085811546",
        "expiration_date": "08/27",
        "cvv": "645",
        "card_holder": "SATOSHI TANABE"
      },
      "bank_account": {
        "bank_code": "9900",
        "bank_name": "ゆうちょ銀行",
        "branch_code": "859",
        "branch_name": "キキョウ支店",
        "account_type": "1",
        "account_number": "1581154",
        "account_holder_kana": "ﾀﾅﾍﾞ ｻﾄｼ"
      },
      "my_number": "000005811546"
    },
    "kyc": {
      "drivers_license_number": "211658115400",
      "passport_number": "TC1581154",
      "document_type": "drivers_license"
    },
    "profile": {
      "occupation": "自営業",
      "industry": "運輸業、郵便業",
      "company_name": "有限会社サンプルロジスティクス",
      "annual_income_man_yen": 920,
      "is_married": false,
      "dependents_count": 0
    }
  }
]
```

上は見やすく整形しています。実際の `--format=json` は配列の中に 1 件 1 行で並び、`--format=jsonl` は 1 行 1 件の JSON Lines です。
`id` は 1 始まりの通し番号（seqID + 1）で、メールなどに埋め込まれる番号は seqID（0 始まり）です。

**CSV**（モジュールなし）: `katashiro export 3 --format=csv`

```csv
id,personal_birthday,personal_age,personal_gender,personal_last_name,personal_first_name,personal_full_name,personal_last_name_hiragana,personal_first_name_hiragana,personal_full_name_hiragana,personal_last_name_katakana,personal_first_name_katakana,personal_full_name_katakana,personal_bank_holder_kana,personal_romaji_name,contact_zip_code,contact_prefecture,contact_city,contact_town,contact_street_address,contact_building,contact_phone_number,contact_email
581155,1978-03-15,47,male,田辺,聡,田辺　聡,たなべ,さとし,たなべ　さとし,タナベ,サトシ,タナベ　サトシ,ﾀﾅﾍﾞ ｻﾄｼ,satoshi.tanabe,020-0034,岩手県,盛岡市,盛岡駅前通,1-16-16,,090-0259-4565,satoshi.tanabe.581154@example.com
663138,1977-04-04,48,male,東,博之,東　博之,ひがし,ひろゆき,ひがし　ひろゆき,ヒガシ,ヒロユキ,ヒガシ　ヒロユキ,ﾋｶﾞｼ ﾋﾛﾕｷ,hiroyuki.higashi,167-0051,東京都,杉並区,荻窪,2-10-12,,070-0981-3484,hiroyuki.higashi.663137@example.jp
380126,1994-01-17,31,male,西村,勝,西村　勝,にしむら,まさる,にしむら　まさる,ニシムラ,マサル,ニシムラ　マサル,ﾆｼﾑﾗ ﾏｻﾙ,masaru.nishimura,862-0950,熊本県,熊本市中央区,水前寺,6-2-7,,080-0129-5368,masaru.nishimura.380125@example.jp
```

CSV / SQLite の列は仕様書のフラット列名の順に並び、指定したモジュールの列だけが出力されます。
SQLite では `profile_is_married` を `INTEGER (1/0)`、18 歳未満の運転免許証番号を `NULL` として保存します。

メール・電話番号下 7 桁・カード番号・口座番号・個人番号・免許証交付番号・パスポート番号は seqID をそのまま、または全単射で変換して埋め込んでいるため、100 万件の範囲で衝突しません。

## 埋め込み辞書の再生成（開発者向け）

埋め込み辞書（`internal/master/data/*.bin`）は内蔵シードから作り、実データは埋め込みません（実データは利用者が `master fetch` で取得します）。
内蔵シードは `internal/masterbuild/seed_*.go` にあり、変更したときだけ再生成します。

```sh
go generate ./...   # 内蔵シードから生成。ネットワーク不要

# --master-dir 形式の確認用 JSON も出力する
go run -tags katashiro_genmaster ./cmd/genmaster -json -out ./custom_master
```

### クレンジング結果の確認

```sh
# ファイルを書き出さずにレポートだけ表示（処理別の件数と元の町域の例、都道府県別件数、クレンジング漏れの一覧）
go run -tags katashiro_genmaster ./cmd/genmaster -ken-all ./utf_ken_all.zip -check

# 本物のデータでクレンジングのテストを実行（未設定時はスキップ）
KATASHIRO_KEN_ALL=./utf_ken_all.zip go test ./internal/masterbuild -run RealData -v
```

`-check` は町域に括弧・`〜`・`、`・「がくる場合」などが残っていたり、郵便番号や公安委員会コードが不正だったりすると終了コード 1 で終わります。
埋め込み済み辞書に対する同じ検査は通常の `go test ./...` にも含まれています。

`internal/master/data/*.bin` はオフセットインデックス付きのバイナリで、`//go:embed` で埋め込まれます。
起動時はヘッダを読むだけで、各レコードはゼロコピー・ゼロアロケーションで参照されます。

## 開発

```sh
make help    # タスク一覧
make test    # テスト
make lint    # golangci-lint（go vet・staticcheck など + gofmt / goimports の確認）
make fmt     # 整形
make vuln    # govulncheck
make ci      # lint + test + vuln（CI と同じ確認）
make build   # ./katashiro をビルド
```

golangci-lint と govulncheck のバージョンは `Makefile` で固定しています（CI も `make` 経由で同じバージョンを使います）。
初回の `make lint` / `make vuln` で `bin/` にインストールされます。

### CI / リリース

| ワークフロー | 内容 |
| :-- | :-- |
| `.github/workflows/ci.yml` | `make lint`（golangci-lint）・`go test -race`、CGO 無効の 6 ターゲットクロスビルド、`make vuln`（govulncheck）、Docker イメージのビルドと動作確認 |
| `.github/workflows/codeql.yml` | CodeQL 静的解析（public リポジトリは無料） |
| `.github/workflows/release.yml` | `v*` タグの push で GoReleaser がバイナリとチェックサムを GitHub Releases に公開 |
| `.github/workflows/master-check.yml` | 週次で最新の `utf_ken_all.zip` を取得し、クレンジング漏れを検査（結果はジョブのサマリーに表示） |
| `.github/dependabot.yml` | Go モジュール・GitHub Actions・Docker ベースイメージの週次更新 |

```sh
git tag v0.1.0 && git push origin v0.1.0   # リリース
```

## データ出典

埋め込み辞書は katashiro 独自の内蔵シードです。`master fetch` で取得する実データの出典は次のとおりです。

- 郵便番号データ: [日本郵便株式会社](https://www.post.japanpost.jp/zipcode/download.html)（「郵便番号データに限っては日本郵便株式会社は著作権を主張しません」）
- 人名データ: [willnet/gimei](https://github.com/willnet/gimei)（MIT License。`master fetch` が出力する `NOTICE` に全文を記載）

## ライセンス

[MIT License](LICENSE)

リリースのバイナリと Docker イメージには、依存ライブラリ（BSD-3-Clause・MIT など）のライセンス文を集めた `THIRD_PARTY_NOTICES` を同梱しています（Docker イメージでは `/usr/share/doc/katashiro/`）。
生成は `go run ./tools/notices > THIRD_PARTY_NOTICES` で、依存の更新とずれないようリリース時に作るためリポジトリには含めません。

## スコープ外

条件検索・フィルタリング、カスタムフィールド用の独自 DSL、網羅的な異常系データの生成は提供しません。
属性の固定には `--set`、`jq` によるパイプ処理、またはテストコード側での上書きを使ってください。
