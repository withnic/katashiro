// Command katashiro は日本向けダミー個人データを決定論的に生成・払い出しする CLI / HTTP サーバー。
//
//	katashiro next <N>   [flags]  カーソルから重複なしで N 件払い出す
//	katashiro export <N> [flags]  カーソルと独立にインデックス 0..N-1 を一括出力する
//	katashiro reset      [flags]  プロファイルのカーソルを 0 に戻す
//	katashiro serve      [flags]  HTTP API サーバーを起動する
//	katashiro master fetch|info   実データ辞書の取得 / 使う辞書の確認
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/withnic/katashiro/internal/engine"
	"github.com/withnic/katashiro/internal/exporter"
	"github.com/withnic/katashiro/internal/master"
	"github.com/withnic/katashiro/internal/masterbuild"
	"github.com/withnic/katashiro/internal/model"
	"github.com/withnic/katashiro/internal/server"
	"github.com/withnic/katashiro/internal/state"
)

// 終了コード。
const (
	exitOK        = 0
	exitError     = 1
	exitExhausted = 2 // 1,000,000 件上限到達（--loop なし）
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

const usage = `katashiro - 日本向けダミー個人データ高速生成ツール

Usage:
  katashiro next <N>   [--format=json|csv|jsonl] [--profile=NAME] [--loop] [common flags]
  katashiro export <N> [--out=PATH] [--format=csv|json|jsonl|sqlite] [--force] [--offset=M] [common flags]
  katashiro reset      [--profile=NAME]
  katashiro serve      [--port=8080] [--in-memory] [--seed=N] [--base-date=YYYY-MM-DD]
  katashiro master fetch [--out=DIR]  日本郵便・gimei から実データの辞書を作る
  katashiro master info  [--master-dir=DIR]  使う辞書の件数とフィンガープリントを表示
  katashiro version

Common flags:
  --seed=<int64>              乱数シード（既定 1）
  --base-date=<YYYY-MM-DD>    年齢計算の基準日（既定 2026-01-01、today で実行日）
  --with-finance / --with-kyc / --with-profile
  --set key=value             スマートオーバーライド（関連項目と整合を取る。複数指定可）
  --set-raw key=value         整合チェックなしで上書き（異常系テスト用。複数指定可）
  --min-age / --max-age       年齢範囲（既定 18〜80）
  --strict-phone              実在プレフィックス帯の電話番号を使う
  --master-dir=DIR            外部の辞書ディレクトリを使う（既定: 環境変数 KATASHIRO_MASTER_DIR、なければ埋め込み辞書）

Run 'katashiro <command> -h' for details.
`

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitError
	}
	var err error
	switch args[0] {
	case "next":
		err = cmdNext(args[1:], stdout, stderr)
	case "export":
		err = cmdExport(args[1:], stdout, stderr)
	case "reset":
		err = cmdReset(args[1:], stdout, stderr)
	case "serve":
		err = cmdServe(args[1:], stderr)
	case "master":
		err = cmdMaster(args[1:], stdout, stderr)
	case "version", "--version", "-v":
		fmt.Fprintln(stdout, "katashiro", version)
		return exitOK
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return exitOK
	default:
		fmt.Fprintf(stderr, "katashiro: unknown command %q\n\n%s", args[0], usage)
		return exitError
	}
	switch {
	case err == nil, errors.Is(err, flag.ErrHelp):
		return exitOK
	case errors.Is(err, state.ErrExhausted):
		fmt.Fprintln(stderr, "katashiro:", err)
		return exitExhausted
	default:
		fmt.Fprintln(stderr, "katashiro:", err)
		return exitError
	}
}

// multiFlag は複数指定できるフラグ（--set / --set-raw）。
type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

// genFlags は next / export / serve 共通の生成フラグ。
type genFlags struct {
	seed                         int64
	baseDate                     string
	minAge, maxAge               int
	withFinance, withKYC, withPr bool
	strictPhone                  bool
	masterDir                    string
	sets, rawSets                multiFlag
}

func (g *genFlags) register(fs *flag.FlagSet, modules bool) {
	fs.Int64Var(&g.seed, "seed", 1, "乱数シード")
	fs.StringVar(&g.baseDate, "base-date", model.DefaultBaseDate, "基準日 YYYY-MM-DD または today")
	fs.StringVar(&g.masterDir, "master-dir", "", "外部の辞書ディレクトリ（既定: 環境変数 KATASHIRO_MASTER_DIR、なければ埋め込み辞書）")
	fs.BoolVar(&g.strictPhone, "strict-phone", false, "実在プレフィックス帯（090-1XXX-XXXX 等）の電話番号を使う")
	fs.IntVar(&g.minAge, "min-age", 18, "最小年齢")
	fs.IntVar(&g.maxAge, "max-age", 80, "最大年齢")
	if modules {
		fs.BoolVar(&g.withFinance, "with-finance", false, "金融・決済・公的IDモジュールを含める")
		fs.BoolVar(&g.withKYC, "with-kyc", false, "本人確認・身分証モジュールを含める")
		fs.BoolVar(&g.withPr, "with-profile", false, "職業・年収・家族構成モジュールを含める")
		fs.Var(&g.sets, "set", "スマートオーバーライド key=value（関連項目と整合を取る。複数指定可）")
		fs.Var(&g.rawSets, "set-raw", "整合チェックなしで生成後に上書きする key=value（異常系テスト用。複数指定可）")
	}
}

func (g *genFlags) options() (model.Options, error) {
	o := model.DefaultOptions()
	o.Seed, o.MinAge, o.MaxAge = g.seed, g.minAge, g.maxAge
	o.WithFinance, o.WithKYC, o.WithProfile = g.withFinance, g.withKYC, g.withPr
	o.StrictPhone = g.strictPhone
	o.Sets = g.sets
	o.RawSets = g.rawSets
	if g.baseDate == "today" {
		now := time.Now()
		o.BaseDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	} else {
		d, err := model.ParseDate(g.baseDate)
		if err != nil {
			return o, fmt.Errorf("--base-date must be YYYY-MM-DD or today: %q", g.baseDate)
		}
		o.BaseDate = d
	}
	d, _, err := loadDict(g.masterDir)
	if err != nil {
		return o, err
	}
	o.Dict = d
	return o, nil
}

// masterDirEnv は既定の辞書ディレクトリを指定する環境変数。
const masterDirEnv = "KATASHIRO_MASTER_DIR"

// loadDict は --master-dir、なければ KATASHIRO_MASTER_DIR の辞書と、そのディレクトリを返す。
// どちらも未指定なら (nil, "", nil) を返し、呼び出し側は埋め込み辞書を使う。
func loadDict(dir string) (*master.Dict, string, error) {
	label := "--master-dir"
	if dir == "" {
		dir, label = os.Getenv(masterDirEnv), masterDirEnv
	}
	if dir == "" {
		return nil, "", nil
	}
	d, err := master.LoadDir(dir)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", label, err)
	}
	return d, dir, nil
}

// parseInterleaved はフラグと位置引数が混在しても解析できるようにする
// （例: katashiro next 10 --format=json）。
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		if args[0] == "--" {
			return append(pos, args[1:]...), nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func parseCount(pos []string, cmd string) (int, error) {
	if len(pos) != 1 {
		return 0, fmt.Errorf("usage: katashiro %s <N> [flags]", cmd)
	}
	n, err := strconv.Atoi(pos[0])
	if err != nil || n < 1 || n > engine.SpaceSize {
		return 0, fmt.Errorf("N must be an integer in 1..%d: %q", engine.SpaceSize, pos[0])
	}
	return n, nil
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("katashiro "+name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

func streamWriter(format string, w io.Writer, cols []*model.Column) (exporter.Writer, error) {
	switch format {
	case exporter.FormatJSON:
		return exporter.NewJSON(w), nil
	case exporter.FormatJSONL:
		return exporter.NewJSONL(w), nil
	case exporter.FormatCSV:
		return exporter.NewCSV(w, cols), nil
	}
	return nil, fmt.Errorf("unsupported format %q", format)
}

func cmdNext(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("next", stderr)
	var g genFlags
	g.register(fs, true)
	format := fs.String("format", exporter.FormatJSON, "出力形式 json|csv|jsonl")
	profile := fs.String("profile", state.DefaultProfile, "カーソルのプロファイル名")
	loop := fs.Bool("loop", false, "上限到達時にカーソル 0 へラップアラウンドする")
	pos, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	n, err := parseCount(pos, "next")
	if err != nil {
		return err
	}
	opt, err := g.options()
	if err != nil {
		return err
	}
	gen, err := model.NewGenerator(opt)
	if err != nil {
		return err
	}
	out, err := streamWriter(*format, stdout, gen.Columns())
	if err != nil {
		return err
	}
	dir, err := state.ResolveDir()
	if err != nil {
		return err
	}
	start, err := state.NewFileStore(dir).Reserve(*profile, n, *loop)
	if err != nil {
		return err
	}
	for k := 0; k < n; k++ {
		if err := out.Write(gen.At(uint32((uint64(start) + uint64(k)) % engine.SpaceSize))); err != nil {
			return err
		}
	}
	return out.Close()
}

func inferFormat(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".sqlite", ".sqlite3", ".db":
		return exporter.FormatSQLite
	case ".csv":
		return exporter.FormatCSV
	case ".jsonl", ".ndjson":
		return exporter.FormatJSONL
	}
	return exporter.FormatJSON
}

func cmdExport(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("export", stderr)
	var g genFlags
	g.register(fs, true)
	format := fs.String("format", "", "出力形式 csv|json|jsonl|sqlite（省略時は --out の拡張子、なければ json）")
	outPath := fs.String("out", "", "出力先ファイル（省略時は標準出力。sqlite では必須）")
	force := fs.Bool("force", false, "既存の出力先を上書きする（sqlite は DROP TABLE して再作成）")
	offset := fs.Int("offset", 0, "何人目から出力するか（0 始まり）。条件を変えて分割生成するときに重複を避ける")
	pos, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	n, err := parseCount(pos, "export")
	if err != nil {
		return err
	}
	if *offset < 0 || *offset+n > engine.SpaceSize {
		return fmt.Errorf("--offset + N must be within 0..%d (offset=%d, N=%d)", engine.SpaceSize, *offset, n)
	}
	if *format == "" {
		*format = inferFormat(*outPath)
	}
	opt, err := g.options()
	if err != nil {
		return err
	}
	gen, err := model.NewGenerator(opt)
	if err != nil {
		return err
	}
	cols := gen.Columns()

	var out exporter.Writer
	var file *os.File
	switch {
	case *format == exporter.FormatSQLite:
		if *outPath == "" {
			return errors.New("--format=sqlite requires --out")
		}
		if out, err = exporter.NewSQLite(*outPath, cols, *force); err != nil {
			return err
		}
	case *outPath == "":
		if out, err = streamWriter(*format, stdout, cols); err != nil {
			return err
		}
	default:
		if _, err := streamWriter(*format, io.Discard, cols); err != nil {
			return err
		}
		flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
		if *force {
			flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		}
		if file, err = os.OpenFile(*outPath, flags, 0o644); err != nil {
			if errors.Is(err, os.ErrExist) {
				return fmt.Errorf("%s already exists (use --force to overwrite)", *outPath)
			}
			return err
		}
		defer file.Close()
		out, _ = streamWriter(*format, file, cols)
	}
	for i := 0; i < n; i++ {
		if err := out.Write(gen.At(uint32(*offset + i))); err != nil {
			out.Close()
			return err
		}
	}
	if err := out.Close(); err != nil {
		return err
	}
	if file != nil {
		return file.Close()
	}
	return nil
}

func cmdReset(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("reset", stderr)
	profile := fs.String("profile", state.DefaultProfile, "カーソルのプロファイル名")
	pos, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return errors.New("usage: katashiro reset [--profile=NAME]")
	}
	dir, err := state.ResolveDir()
	if err != nil {
		return err
	}
	if err := state.NewFileStore(dir).Reset(*profile); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "profile %q cursor reset to 0\n", *profile)
	return nil
}

func cmdServe(args []string, stderr io.Writer) error {
	fs := newFlagSet("serve", stderr)
	var g genFlags
	g.register(fs, false)
	port := fs.Int("port", 8080, "待ち受けポート")
	host := fs.String("host", "", "待ち受けアドレス（既定: 全インターフェース）")
	inMemory := fs.Bool("in-memory", false, "カーソルをファイルではなくオンメモリのアトミックカウンタで管理する")
	pos, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return errors.New("usage: katashiro serve [flags]")
	}
	opt, err := g.options()
	if err != nil {
		return err
	}
	if _, err := model.NewGenerator(opt); err != nil {
		return err
	}
	var alloc state.Allocator
	if *inMemory {
		alloc = state.NewMemoryStore()
	} else {
		dir, err := state.ResolveDir()
		if err != nil {
			return err
		}
		alloc = state.NewFileStore(dir)
	}
	srv := &http.Server{
		Addr:              net.JoinHostPort(*host, strconv.Itoa(*port)),
		Handler:           server.New(opt, alloc).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	fmt.Fprintf(stderr, "katashiro: listening on %s (seed=%d base-date=%s in-memory=%v)\n",
		srv.Addr, opt.Seed, opt.BaseDate.Format(model.DateLayout), *inMemory)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

func cmdMaster(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: katashiro master fetch|info [flags]")
	}
	switch args[0] {
	case "fetch":
		return cmdMasterFetch(args[1:], stdout, stderr)
	case "info":
		return cmdMasterInfo(args[1:], stdout, stderr)
	}
	return fmt.Errorf("unknown master command %q (fetch / info)", args[0])
}

func cmdMasterFetch(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("master fetch", stderr)
	out := fs.String("out", "master", "辞書の出力先ディレクトリ")
	kenAll := fs.String("ken-all", masterbuild.DefaultKenAllURL, "utf_ken_all.zip / CSV のパスまたは URL（seed で内蔵シード）")
	names := fs.String("names", masterbuild.DefaultNamesURL, "gimei 形式 names.yml のパスまたは URL（seed で内蔵シード）")
	force := fs.Bool("force", false, "出力先に既存の辞書があれば上書きする")
	pos, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return errors.New("usage: katashiro master fetch [--out=DIR] [--ken-all=SRC] [--names=SRC] [--force]")
	}
	if _, err := os.Stat(filepath.Join(*out, master.AddressesBinName)); err == nil && !*force {
		return fmt.Errorf("%s には既に辞書があります（--force で上書き）", *out)
	}
	src := masterbuild.Sources{KenAll: *kenAll, Names: *names}
	if src.KenAll == "seed" {
		src.KenAll = ""
	}
	if src.Names == "seed" {
		src.Names = ""
	}
	fmt.Fprintln(stderr, "katashiro: マスタデータを取得・検査しています...")
	r, err := masterbuild.Build(src)
	if err != nil {
		return err
	}
	if err := r.Check(); err != nil {
		return fmt.Errorf("クレンジング結果に問題があるため辞書を書き出しません: %w", err)
	}
	if err := r.WriteDir(*out, time.Now()); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "辞書を %s に作成しました。\n", *out)
	printDictInfo(stdout, r.Dict)
	abs, _ := filepath.Abs(*out)
	fmt.Fprintf(stdout, "\n使うときは --master-dir=%s を付けるか、次を設定してください:\n  export %s=%s\n", *out, masterDirEnv, abs)
	fmt.Fprintln(stdout, "チームで同じデータを使うときは、このディレクトリ（NOTICE を含む）をそのまま共有してください。")
	return nil
}

func printDictInfo(w io.Writer, d *master.Dict) {
	st := d.Stats()
	fmt.Fprintf(w, "  住所: %d 件 / 姓: %d 件 / 男性名: %d 件 / 女性名: %d 件\n", st.Addresses, st.LastNames, st.MaleNames, st.FemaleNames)
	fmt.Fprintf(w, "  フィンガープリント: %s\n", d.Fingerprint())
}

func cmdMasterInfo(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("master info", stderr)
	dir := fs.String("master-dir", "", "辞書ディレクトリ（既定: 環境変数 KATASHIRO_MASTER_DIR、なければ埋め込み辞書）")
	if _, err := parseInterleaved(fs, args); err != nil {
		return err
	}
	d, where, err := loadDict(*dir)
	if err != nil {
		return err
	}
	if d == nil {
		if d, err = master.Default(); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "辞書: 埋め込み（内蔵シード）")
	} else {
		fmt.Fprintf(stdout, "辞書: %s\n", where)
		var m masterbuild.Manifest
		if b, err := os.ReadFile(filepath.Join(where, master.ManifestFile)); err == nil && json.Unmarshal(b, &m) == nil {
			fmt.Fprintf(stdout, "  作成日時: %s\n  住所の出典: %s\n  人名の出典: %s\n", m.CreatedAt, m.AddressesSource, m.NamesSource)
		}
	}
	printDictInfo(stdout, d)
	return nil
}
