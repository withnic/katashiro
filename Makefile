# katashiro の開発用タスク。`make help` で一覧を表示する。
# ツールのバージョンはここで固定する（CI も make 経由で同じバージョンを使う）。

GOLANGCI_LINT_VERSION := v2.15.0
GOVULNCHECK_VERSION   := v1.10.0

BIN     := $(CURDIR)/bin
BINARY  := katashiro
VERSION ?= dev

GOLANGCI_LINT := $(BIN)/golangci-lint-$(GOLANGCI_LINT_VERSION)
GOVULNCHECK   := $(BIN)/govulncheck-$(GOVULNCHECK_VERSION)

.DEFAULT_GOAL := help
.PHONY: help build test test-race lint fmt vuln generate notices tidy ci clean

help: ## タスク一覧を表示する
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  %-10s %s\n", $$1, $$2}'

build: ## CGO なしでバイナリをビルドする（./katashiro）
	CGO_ENABLED=0 go build -trimpath -ldflags="-w -s -X main.version=$(VERSION)" -o $(BINARY) ./cmd/katashiro

test: ## テストを実行する
	go test ./...

test-race: ## データ競合検出付きでテストする（cgo と C コンパイラが必要）
	go test -race ./...

lint: $(GOLANGCI_LINT) ## golangci-lint（go vet・staticcheck などとフォーマット確認）
	$(GOLANGCI_LINT) run ./...
	$(GOLANGCI_LINT) fmt --diff ./...

fmt: $(GOLANGCI_LINT) ## gofmt / goimports で整形する
	$(GOLANGCI_LINT) fmt ./...

vuln: $(GOVULNCHECK) ## 依存の既知の脆弱性を調べる
	$(GOVULNCHECK) ./...

generate: ## 内蔵シードから埋め込み辞書を再生成する
	go generate ./...

notices: ## 依存ライブラリのライセンス文 THIRD_PARTY_NOTICES を作る
	go run ./tools/notices > THIRD_PARTY_NOTICES

tidy: ## go.mod / go.sum を整理する
	go mod tidy

ci: lint test vuln ## CI と同じ確認をまとめて実行する

clean: ## ビルド成果物とツールを消す
	rm -rf $(BIN) $(BINARY) THIRD_PARTY_NOTICES

# ツールはバージョン付きのファイル名で bin/ に入れる。バージョンを変えると自動で入れ直す。
$(GOLANGCI_LINT):
	@mkdir -p $(BIN)
	GOBIN=$(BIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@mv $(BIN)/golangci-lint $@

$(GOVULNCHECK):
	@mkdir -p $(BIN)
	GOBIN=$(BIN) go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	@mv $(BIN)/govulncheck $@
