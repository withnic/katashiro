# **CLAUDE.md — katashiro（形代）**

# **プロジェクト概要**

日本のWebサービス・業務システムテスト向けに、最大100万件の重複なしダミー個人データをミリ秒単位で決定論的に生成・払い出しする Go 製シングルバイナリツール（CLI \+ HTTP API サーバー）。

詳細仕様は以下を正（Source of Truth）とする：

* `docs/01_system_overview_and_workflow_v2.md`（全体要件・CLI/HTTP仕様・アルゴリズム）  
* `docs/02_data_models_and_rules_v2.md`（データモデル・生成ルール・フラット列名）

# **技術スタック・依存制約**

| カテゴリ | 選定技術・制約方針 |
| :---- | :---- |
| **言語 / ビルド** | Go（`CGO_ENABLED=0` の Pure Go シングルバイナリ） |
| **CLI引数解析** | 標準ライブラリ `flag.NewFlagSet` のみ使用（Cobra 等の外部フレームワークは導入しない） |
| **SQLite出力** | `modernc.org/sqlite`（CGO不要） |
| **ファイルロック** | `github.com/gofrs/flock`（Linux / macOS / WSL / Windows 透過対応） |
| **依存方針** | 標準ライブラリ優先。不要な外部依存を増やさない |

# **ディレクトリ構成**

```
cmd/
  katashiro/main.go       # CLI エントリポイント (next / export / reset / serve)
  genmaster/main.go       # go generate 用マスタ生成・バイナリ変換ツール（処理は internal/masterbuild）
internal/
  engine/                 # Cycle-Walking Feistel, SplitMix64, Luhn, モジュラス11
  master/                 # //go:embed ゼロパース辞書リーダー, 全銀協カナ, ヘボン式ローマ字
  masterbuild/            # 内蔵シード, ken_all・gimei の取得・クレンジング・検査（genmaster と master fetch が共用）
    data/                 # 生成済み addresses.bin, names.bin
  model/                  # Persona 構造体, スマートオーバーライド (--set)
  state/                  # cursor_<profile>.json 永続化・排他制御
  exporter/               # ストリーミング出力 (json / jsonl / csv / sqlite)
  server/                 # HTTP API サーバー (katashiro serve)
docs/                     # v2 仕様書
```

# **よく使うコマンド**

```
# マスタバイナリ生成（オフラインシード生成対応）
go generate ./...

# テスト実行
go test ./...

# バイナリビルド（CGO無効）
CGO_ENABLED=0 go build -ldflags="-w -s" -o katashiro ./cmd/katashiro
```

# **厳守すべき設計ルール（Non-Negotiables）**

1. **起動1ms未満のゼロパース設計**  
   * 起動時に巨大なJSONを `encoding/json` でパースしない。`internal/master/data/*.bin` のオフセットインデックス付きバイナリを `//go:embed` で埋め込み、`O(1)` 参照する。  
   * `cmd/genmaster` には外部ソース取得モードに加え、ネットワークなしでも47都道府県（公安委員会コード `10`〜`97` 付き）の辞書バイナリを生成できる内蔵シードモードを必ず維持する。  
   * 埋め込み辞書は内蔵シードのみとし、実データ（郵便番号データ・gimei）は埋め込まない。実データは利用者が `katashiro master fetch` で取得し、`--master-dir` / `KATASHIRO_MASTER_DIR` を明示したときだけ使う（自動では使わない）。外部辞書の `.bin` も読み込むだけでパースしない。  
2. **100万件重複ゼロの数学的保証**  
   * 空間サイズ `N = 1,000,000` に対し、20bit（10bit × 10bit、4ラウンド）の **Cycle-Walking Feistel** を使う（単純な `% 1000000` は衝突するため厳禁）。  
   * 属性選択は `SplitMix64` \+ 属性別ソルト `AttrSalt` で攪拌し、属性間の相関を排除する。  
   * 一意性必須項目（メール、電話番号下7桁、クレカ中間桁、銀行口座番号 `1000000+seqID`、ダミー個人番号8桁枠、運転免許証交付番号6桁=`seqID`、パスポート番号）には `seqID` を直接または全単射で埋め込む。  
3. **日付・年齢・文字変換の厳密性**  
   * 基準日（`--base-date`、デフォルト `2026-01-01`）とシード（`--seed`、デフォルト `1`）で完全な再現性を保つ。  
   * 満年齢は `365.2425` 等の浮動小数点除算を使わず、カレンダー月日比較で厳密に算出する。  
   * 全銀協カナは濁点・半濁点を分離し、小文字カナ（`ｧｨｩｪｫｯｬｭｮ`）を必ず大文字化（`ｱｲｳｴｵﾂﾔﾕﾖ`）、最大30文字に収める。  
   * クレジットカードの Amex は **15桁・CVV 4桁**（他ブランドは16桁・CVV 3桁）とする。  
4. **スマートオーバーライド（`--set key=value`）の2段階処理**  
   * **上流連動キー**: `personal.age`, `personal.birthday`, `personal.gender`, `contact.prefecture`, `personal.last_name`, `personal.first_name`, `kyc.document_type`, `profile.is_married` は生成上流で反映し、関連項目（年齢⇔生年月日、性別⇔名前辞書、都道府県⇔住所・運転免許証公安委員会コード、18歳未満の免許証 `null`、18歳未満の未婚・扶養0、姓名⇔読みから導出したカナ・全銀協カナ・ローマ字、提示書類⇔年齢、婚姻⇔扶養・年齢）を整合させる。姓名は読み（`*_hiragana` か `*_katakana`）の併記を必須とし、名前から性別を推定しない。  
   * **単純上書きキー**: 上記以外のキー（`personal.full_name` 等）は生成後の最終ステップで対象フィールドのみ上書きする（形態素解析による読みの推定は行わない）。  
   * **`--set-raw`**: 異常系テスト用。すべてのキーを整合チェックなしで生成後に上書きする。既定の生成と `--set` は常に整合したデータを出す。  
5. **OOM回避のストリーミング出力**  
   * `export 1000000` で全件スライスをメモリに溜め込まない。`json` 配列出力も含め `bufio.Writer` で1件ずつストリーミング出力し、SQLite は単一トランザクション \+ Prepared Statement で一括 INSERT する。  
6. **Non-Goals（実装しないもの）**  
   * 条件検索・フィルタリング機能、カスタムフィールド用独自DSL、網羅的な異常系データ生成機能は追加しない。

# **テスト・実装時の行動指針**

* 変更後は必ず `go test ./...` を実行し、全テストが通ることを確認する。  
* Feistel 置換の100万件全単射テスト（`0..999,999` の衝突ゼロ検証）および境界値テスト（17歳・18歳・20歳・`contact.prefecture=福岡県`・並行リクエスト重複ゼロ）を壊さないこと。  
* テスト実行ログは失敗時のみ詳細を確認し、冗長な出力を抑える。

