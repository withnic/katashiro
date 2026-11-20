// Command genmaster は katashiro の埋め込み辞書（internal/master/data/*.bin）を生成する。
//
// 既定ではネットワークを使わず内蔵シードデータ（47都道府県の代表住所と姓名）から生成する。
// 埋め込み辞書は内蔵シードのままにし、実データは利用者が katashiro master fetch で取得する。
// -ken-all / -names / -fetch はクレンジングの確認（-check）や開発用。
//
//	go generate ./...                                      # 内蔵シードから埋め込み辞書を生成
//	go run -tags katashiro_genmaster ./cmd/genmaster -ken-all ./utf_ken_all.zip -check
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/withnic/katashiro/internal/masterbuild"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "genmaster:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("genmaster", flag.ContinueOnError)
	out := fs.String("out", "internal/master/data", "出力ディレクトリ")
	kenAll := fs.String("ken-all", "", "utf_ken_all.zip / CSV のパスまたは URL（省略時は内蔵シード）")
	names := fs.String("names", "", "gimei 形式 names.yml のパスまたは URL（省略時は内蔵シード）")
	fetch := fs.Bool("fetch", false, "-ken-all / -names 未指定時に既定 URL から取得する")
	writeJSON := fs.Bool("json", false, "確認用 addresses.json / names.json も出力する（--master-dir 形式）")
	check := fs.Bool("check", false, "ファイルを書き出さず、クレンジング結果のレポートを表示する（問題があれば終了コード 1）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	src := masterbuild.Sources{KenAll: *kenAll, Names: *names}
	if *fetch {
		if src.KenAll == "" {
			src.KenAll = masterbuild.DefaultKenAllURL
		}
		if src.Names == "" {
			src.Names = masterbuild.DefaultNamesURL
		}
	}
	r, err := masterbuild.Build(src)
	if err != nil {
		return err
	}
	if *check {
		return masterbuild.Report(os.Stdout, r.Dict, r.Stats)
	}
	if err := r.WriteBins(*out); err != nil {
		return err
	}
	if *writeJSON {
		if err := r.WriteJSON(*out); err != nil {
			return err
		}
	}
	st := r.Dict.Stats()
	fmt.Fprintf(os.Stderr, "genmaster: addresses=%d last=%d male=%d female=%d -> %s\n",
		st.Addresses, st.LastNames, st.MaleNames, st.FemaleNames, *out)
	return nil
}
