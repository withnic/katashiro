# **katashiro（形代）データ項目・生成ルール定義書 v2**

本ドキュメントは、擬似個人情報生成エンジン「katashiro（形代）」のデータモデル、マスタクレンジング仕様、フラット出力（CSV/SQLite）時のカラムマッピング、および100万件規模のデータ整合性を担保する各種生成ロジックの完全仕様（v2改訂版）である。

# **1\. 元マスタデータ仕様（前準備・クレンジング・バイナリ化）**

100万件の重複なし・矛盾ゼロのデータを高速生成するため、各種オープンデータおよびOSS辞書を事前にクレンジングし、オフセットインデックス付きのバイナリ辞書（KTS1 形式）として保持する。バイナリに埋め込むのは内蔵シード（47都道府県の代表住所約440件・姓名約350件、起動1ms未満）のみで、下表の実データは利用者が `katashiro master fetch` で取得し、同じ形式の辞書ディレクトリとして `--master-dir` / `KATASHIRO_MASTER_DIR` で指定する。

| マスタ種別 | 取得元ソース | 推奨規模 | クレンジング・整形・埋め込みルール |
| :---- | :---- | :---- | :---- |
| **郵便番号・住所マスタ**`addresses.bin` | 日本郵便 `utf_ken_all.zip`(UTF-8版) | 約12万件 | ・「以下に掲載がない場合」「〜の次に番地がくる場合」「〜一円」の町域は空文字に正規化。・1行完結の括弧書き `（...）` を正規表現で除去。・複数行分割レコードの断片（片側括弧や `、` を含む行）を除外し、`郵便番号 + 都道府県 + 市区町村 + 町域` で重複統合。・\*\*都道府県コード（JIS X 0401: 01〜47）**および**都道府県公安委員会コード（10〜97）\*\*を事前付与し、実行時の文字列検索コストをゼロにする。 |
| **人名マスタ**`names.bin` | OSS人名辞書(`willnet/gimei`, `names.yml` 等) | gimei: 姓 約500件・男性名 約2,500件・女性名 約10,700件 | ・`{kanji, hiragana, katakana, zengin_kana, romaji}` の5点セットを辞書生成時（`master fetch` / `go generate`）に事前変換して保持し、実行時の文字変換コストと読み破綻を完全防止。・同字異音（「陽菜」＝はるな/ひな）を網羅するため、`kanji + hiragana` のペアで一意管理。 |

# **2\. 出力データ項目一覧と生成ルール**

すべてのランダム選択・数値生成は、Cycle-Walking Feistel（空間サイズ \$N \= 1,000,000\$）によって得られた一意の `seqID`（\$0 \\le \\text{seqID} \\le 999,999\$）と属性別ソルト `AttrSalt` を `SplitMix64` に通した決定論的乱数から導出する。**日時や年齢を含む全項目はオンザフライで計算され、事前生成ファイルは保持しない。**

## **2.1 基本識別・属性 (`personal`)**

| フィールド名 (JSON) | フラット列名 (CSV/SQLite) | 型 | 制約 / 生成範囲 | 詳細仕様・算出アルゴリズム |
| :---- | :---- | :---- | :---- | :---- |
| `id` | `id` | uint64 | 1 〜 1,000,000 | `seqID + 1`。100万件内で完全一意。 |
| `personal.birthday` | `personal_birthday` | string | `YYYY-MM-DD` | **基準日連動の動的算出**: 基準日（`--base-date`、デフォルト `2026-01-01`）時点で **満18歳〜満80歳** となる日数オフセットを `attrRand(seqID, SaltBirthday)` で決定論的に算出。うるう年（2月29日）も含め標準 `time.Time.AddDate` で厳密に計算する。成人（18歳以上）を保証することで、クレジットカード・銀行口座・運転免許証との年齢矛盾を根本から排除する（※ `--min-age` / `--max-age` で変更可）。 |
| `personal.age` | `personal_age` | int | 18 〜 80 (デフォルト) | 基準日（`--base-date`）時点における正確な満年齢。カレンダー上の月日比較（`(BaseMonth, BaseDay) < (BirthMonth, BirthDay)` なら \$-1\$）で厳密に算出し、浮動小数点除算（365.2425）による誕生日当日の丸め誤差バグを防止する。 |
| `personal.gender` | `personal_gender` | string | `male` / `female`（`--set` 指定時のみ `other` / `unspecified`） | `attrRand(seqID, SaltGender) % 2` で決定し、使用する名（first\_name）マスタの性別区分と同期させる。`other` / `unspecified` は男女どちらの名マスタからも選ぶ。 |

## **2.2 氏名・読み・全銀協カナ・ローマ字 (`personal`)**

| フィールド名 (JSON) | フラット列名 (CSV/SQLite) | 型 | 生成ルール・フォーマット | 例 |
| :---- | :---- | :---- | :---- | :---- |
| `personal.last_name` | `personal_last_name` | string | 姓（漢字）: `Surnames[attrRand(seqID, SaltLastName) % len(Surnames)]` | 佐藤 |
| `personal.first_name` | `personal_first_name` | string | 名（漢字）: 性別に対応する名マスタから `attrRand(seqID, SaltFirstName)` で選択 | 太郎 |
| `personal.full_name` | `personal_full_name` | string | 姓 \+ 全角スペース \+ 名 | 佐藤 太郎 |
| `personal.last_name_hiragana` | `personal_last_name_hiragana` | string | 姓（ひらがな） | さとう |
| `personal.first_name_hiragana` | `personal_first_name_hiragana` | string | 名（ひらがな） | たろう |
| `personal.full_name_hiragana` | `personal_full_name_hiragana` | string | 姓（ひらがな） \+ 全角スペース \+ 名（ひらがな） | さとう たろう |
| `personal.last_name_katakana` | `personal_last_name_katakana` | string | 姓（全角カタカナ） | サトウ |
| `personal.first_name_katakana` | `personal_first_name_katakana` | string | 名（全角カタカナ） | タロウ |
| `personal.full_name_katakana` | `personal_full_name_katakana` | string | 姓（全角カタカナ） \+ 全角スペース \+ 名（全角カタカナ） | サトウ タロウ |
| `personal.bank_holder_kana` | `personal_bank_holder_kana` | string | **全銀協フォーマット厳密準拠**: 半角カタカナ（濁点・半濁点は独立1文字 `ﾞ` `ﾟ`）、**拗音・促音の小文字は大文字化**（`ｧ`→`ｱ`, `ｯ`→`ﾂ`, `ｬ`→`ﾔ`, `ュ`→`ﾕ`, `ョ`→`ﾖ`）、長音 `ｰ`、姓と名の間は半角スペース1文字、最大30文字切り詰め。 | ｻﾄｳ ﾀﾛｳ |
| `personal.romaji_name` | `personal_romaji_name` | string | 修正ヘボン式ローマ字（小文字、`first.last` 形式） | taro.sato |

## **2.3 郵便番号・住所 (`contact`)**

| フィールド名 (JSON) | フラット列名 (CSV/SQLite) | 型 | 生成ルール・フォーマット | 例 |
| :---- | :---- | :---- | :---- | :---- |
| `contact.zip_code` | `contact_zip_code` | string | 7桁数字（デフォルト: `XXX-XXXX`） | 100-0001 |
| `contact.prefecture` | `contact_prefecture` | string | 住所マスタ連携（都道府県） | 東京都 |
| `contact.city` | `contact_city` | string | 住所マスタ連携（市区町村） | 千代田区 |
| `contact.town` | `contact_town` | string | 住所マスタ連携（町域。空文字の場合は省略） | 千代田 |
| `contact.street_address` | `contact_street_address` | string | 決定論的生成（`1〜6`丁目 \- `1〜30`番 \- `1〜20`号） | 1-2-3 |
| `contact.building` | `contact_building` | string | 約35%の確率で付与（架空マンション名 \+ `101〜1508`号室、それ以外は空文字 `""`） | メゾンサクラ 402号室 |

## **2.4 連絡先・誤送信防止ルール (`contact`)**

### **電話番号 (`contact.phone_number` / `contact_phone_number`)**

1. **安全帯域モード（デフォルト）**:  
   総務省未割り当ての `090-0XXX-XXXX` / `080-0XXX-XXXX` / `070-0XXX-XXXX`（計3,000万通り）を使用。  
   * **100万件重複ゼロ保証**: プレフィックス3種（`seqID % 3`）＋ 下7桁に `seqID`（`0000000`〜`0999999`）または7桁全単射値を割り当てることで、100万件すべての電話番号が完全一意かつ実在番号への誤架電ゼロとなる。  
2. **厳密バリデーションモード (`--strict-phone`)**:  
   `libphonenumber` 等のキャリア割当チェックを通過する実在プレフィックス（`090-1XXX-XXXX` 等）に `seqID` を埋め込んで一意生成する。

### **メールアドレス (`contact.email` / `contact_email`)**

* **生成ロジック**: `{romaji_name}.{seqID}@{example.com|example.jp|example.net}`  
* `seqID` がそのまま入るため、同姓同名であっても100万件内で100%一意となり、RFC 2606予約ドメインにより外部誤送信も物理的に発生しない。

## **2.5 金融・決済・公的IDモジュール（`--with-finance` 指定時）**

### **クレジットカード (`finance.credit_card`)**

主要4ブランドに対応し、`seqID` の直接埋め込みによる100万件重複ゼロと Luhnアルゴリズム（Modulus 10） を完全充足する。

| ブランド | BIN (先頭6桁) | 総桁数 | 中間桁の構成（重複ゼロ保証） | CVV桁数 |
| :---- | :---- | :---- | :---- | :---- |
| **Visa** | `400000` | 16桁 | 派生3桁 \+ seqID (6桁ゼロ埋め) \+ CheckDigit (1桁) | 3桁 |
| **Mastercard** | `555555` | 16桁 | 派生3桁 \+ seqID (6桁ゼロ埋め) \+ CheckDigit (1桁) | 3桁 |
| **JCB** | `353011` | 16桁 | 派生3桁 \+ seqID (6桁ゼロ埋め) \+ CheckDigit (1桁) | 3桁 |
| **American Express** | `378282` | **15桁** | 派生2桁 \+ seqID (6桁ゼロ埋め) \+ CheckDigit (1桁) | **4桁** |

* **フラット列名**: `finance_card_brand`, `finance_card_number`, `finance_card_expiration`, `finance_card_cvv`, `finance_card_holder`  
* **`expiration_date`**: 基準日（`--base-date`）の年から `1〜5` 年後の `MM/YY` 形式（決定論的に算出）。  
* **`card_holder`**: 大文字英字氏名（`FIRST LAST` 形式、例: `TARO SATO`）。

### **銀行口座 (`finance.bank_account`)**

| フィールド名 (JSON) | フラット列名 (CSV/SQLite) | 生成ルール / 100万件重複ゼロ保証 |
| :---- | :---- | :---- |
| `bank_code` / `bank_name` | `finance_bank_code` / `finance_bank_name` | 主要7行から選択（0001 みずほ / 0005 三菱UFJ / 0009 三井住友 / 0010 りそな / 0033 PayPay / 0036 楽天 / 9900 ゆうちょ） |
| `branch_code` / `branch_name` | `finance_branch_code` / `finance_branch_name` | `001〜999` の3桁数字 / 架空支店名（例: サクラ支店） |
| `account_type` | `finance_account_type` | `1`（普通預金） |
| `account_number` | `finance_account_number` | **7桁数字（単体ユニーク保証）**: `fmt.Sprintf("%07d", 1000000 + seqID)`（`1000000`〜`1999999`）により、銀行・支店が同じでも口座番号単体で100万件の衝突が絶対に発生しない。 |
| `account_holder_kana` | `finance_account_holder_kana` | `personal.bank_holder_kana` と同一値（全銀協準拠・大文字半角カナ） |

### **テスト用ダミー個人番号 (`finance.my_number` / `finance_my_number`)**

* **構造 (12桁)**: `000`（テスト用プレフィックス3桁） \+ `seqID` ゼロ埋め8桁（`00000000`〜`00999999`） \+ `P`（法令準拠モジュラス11ウェイト検査用数字1桁）。  
* `seqID` をそのまま8桁枠にマッピングするため、100万件内で衝突率0%かつチェックディジット検証を100%通過する。

## **2.6 本人確認・身分証モジュール（`--with-kyc` 指定時）**

| フィールド名 (JSON) | フラット列名 (CSV/SQLite) | 型 | 詳細仕様・整合性・重複ゼロ保証ロジック |
| :---- | :---- | :---- | :---- |
| `kyc.drivers_license_number` | `kyc_drivers_license_number` | string (12桁) | ・**1-2桁目**: 住所の都道府県に連動した都道府県公安委員会コード（北海道 `10` 〜 沖縄 `97`）。・**3-4桁目**: 初回取得年（西暦下2桁）。`生年 + 18` 〜 `基準年` の範囲で決定論的に算出。万が一 `--min-age` 指定で18歳未満のレコードを生成した場合は空文字 `""`（JSONでは `null`）とする。・**5-10桁目**: **交付番号6桁**。ハッシュ剰余による誕生日パラドックス衝突を防ぐため、`seqID`（`000000`〜`999999`）を直接ゼロ埋め6桁で格納し、100万件完全ユニークを保証。・**11桁目**: モジュラス11による検査用数字。・**12桁目**: 再発行回数 `0`。 |
| `kyc.passport_number` | `kyc_passport_number` | string (9桁) | `T` \+ 英字1文字（`A〜Z`） \+ 数字7桁（`1000000 + seqID`）。100万件完全ユニーク。 |
| `kyc.document_type` | `kyc_document_type` | string | 本人確認に提示する書類: `drivers_license` / `passport`。18歳未満は必ず `passport`。18歳以上は約80%が `drivers_license`（`attrRand(seqID, SaltKYCDocument)` で決定）。`--set kyc.document_type=...` で固定可能（`drivers_license` は18歳未満を含む設定ではエラー）。 |

## **2.7 職業・年収・家族構成モジュール（`--with-profile` 指定時）**

| フィールド名 (JSON) | フラット列名 (CSV/SQLite) | 型 | 詳細仕様・年齢/属性連動ロジック |
| :---- | :---- | :---- | :---- |
| `profile.occupation` | `profile_occupation` | string | 申込フォームでよく使われる11区分: `会社員` / `会社役員` / `契約社員` / `派遣社員` / `公務員` / `自営業` / `パート・アルバイト` / `学生` / `主婦・主夫` / `年金受給` / `無職`。年齢と連動（15歳未満は必ず学生、18〜22歳は学生確率高、65歳以上は年金受給確率高）。 |
| `profile.industry` | `profile_industry` | string | **日本標準産業分類（令和5年7月改定）の大分類20区分の名称**（例: `製造業`、`卸売業、小売業`、`医療、福祉`、`公務（他に分類されるものを除く）`）。就業者数のおおよその比率で重み付けして選ぶ。`公務` は公務員のみ（公務員は勤務先に応じて `公務` / `教育、学習支援業` / `医療、福祉`）。学生・主婦・主夫・年金受給・無職の場合は空文字 `""`。 |
| `profile.company_name` | `profile_company_name` | string | 業種に対応した架空企業名（例: 製造業 → `株式会社〇〇製作所`、公務員 → `〇〇市役所`）。学生・主婦・主夫・年金受給・無職の場合は空文字 `""`。 |
| `profile.annual_income_man_yen` | `profile_annual_income_man_yen` | int | 職業・年齢に応じた年収（10万円単位）。学生・主婦・主夫・無職: `0〜120`、年金受給: `60〜300`、パート: `80〜250`（18歳未満 `10〜100`）、契約・派遣: `200〜450`（22歳以下 `180〜300`）、会社役員: `600〜2000`（30歳未満 `300〜800`）、会社員・公務員・自営業: `300〜1200`（22歳以下 `200〜400`、23〜29歳 `250〜600`）。 |
| `profile.is_married` | `profile_is_married` | bool | 配偶者の有無。**JSONでは boolean (`true`/`false`)、CSVでは `"true"`/`"false"`、SQLiteでは `INTEGER (1/0)`**。18歳未満は必ず `false`（18〜19歳は少数のみ `true`）。 |
| `profile.dependents_count` | `profile_dependents_count` | int | 扶養している家族の人数の目安（税・社会保険上の扶養の定義ではない）。`0 〜 4`。18歳未満は必ず `0`。 |

# **3\. CSV / SQLite フラット出力スキーマ仕様**

* **カラム順序と命名の一元化**: CSVのヘッダー行とSQLiteの `personas` テーブルのカラム定義は、本仕様書の「フラット列名」の順序に完全一致させる。  
* **SQLiteテーブル定義**:  
  `--with-finance`, `--with-kyc`, `--with-profile` の指定有無にかかわらず、SQLiteエクスポート時は指定モジュールの列だけを持つスキーマを生成する（CSV と同じ列構成）。既存ファイルへの出力は `--force` 指定時のみ許可し、テーブルを DROP して再作成する。

