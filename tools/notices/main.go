// Command notices は katashiro のバイナリにリンクされる依存モジュール（と Go 標準ライブラリ）の
// ライセンス文を集め、THIRD_PARTY_NOTICES として標準出力に書く。
//
// MIT / BSD はバイナリ配布時にライセンス文の同梱を求めるため、リリース（GoReleaser）と
// Docker イメージのビルド時に生成して同梱する。依存の更新とずれないよう、生成物はコミットしない。
//
//	go run ./tools/notices > THIRD_PARTY_NOTICES
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// targets はリリース対象の OS / アーキテクチャ（.goreleaser.yaml と揃える）。
// OS ごとにリンクされるパッケージが違うため、全ターゲットの和集合を取る。
var targets = [][2]string{
	{"linux", "amd64"}, {"linux", "arm64"},
	{"darwin", "amd64"}, {"darwin", "arm64"},
	{"windows", "amd64"}, {"windows", "arm64"},
}

// licenseFile はモジュール直下のライセンス・告知ファイル名。
var licenseFile = regexp.MustCompile(`(?i)^(LICEN[CS]E|COPYING|NOTICE)([-._].*)?$`)

type module struct {
	Path, Version, Dir string
}

func main() {
	pkg := flag.String("pkg", "./cmd/katashiro", "対象のメインパッケージ")
	flag.Parse()
	if err := run(os.Stdout, *pkg); err != nil {
		fmt.Fprintln(os.Stderr, "notices:", err)
		os.Exit(1)
	}
}

func run(w io.Writer, pkg string) error {
	self, err := goOutput(nil, "list", "-m", "-f", "{{.Path}}")
	if err != nil {
		return err
	}
	self = strings.TrimSpace(self)
	mods := map[string]module{}
	for _, t := range targets {
		out, err := goOutput([]string{"GOOS=" + t[0], "GOARCH=" + t[1], "CGO_ENABLED=0"},
			"list", "-deps", "-json=Standard,Module", pkg)
		if err != nil {
			return err
		}
		dec := json.NewDecoder(strings.NewReader(out))
		for dec.More() {
			var p struct {
				Standard bool
				Module   *module
			}
			if err := dec.Decode(&p); err != nil {
				return err
			}
			if p.Standard || p.Module == nil || p.Module.Path == self {
				continue
			}
			mods[p.Module.Path] = *p.Module
		}
	}
	paths := make([]string, 0, len(mods))
	for p := range mods {
		paths = append(paths, p)
	}
	slices.Sort(paths)

	fmt.Fprintln(w, "THIRD-PARTY SOFTWARE NOTICES")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "katashiro のバイナリには以下のソフトウェアが含まれています。各ライセンス文を掲載します。")
	fmt.Fprintln(w, "This binary includes the following third-party software. Their license texts follow.")

	goroot, err := goOutput(nil, "env", "GOROOT")
	if err != nil {
		return err
	}
	goVersion, _ := goOutput(nil, "env", "GOVERSION")
	if err := section(w, "Go standard library and runtime", strings.TrimSpace(goVersion), strings.TrimSpace(goroot), "LICENSE"); err != nil {
		return err
	}
	for _, p := range paths {
		m := mods[p]
		if m.Dir == "" {
			return fmt.Errorf("%s: module not downloaded (run go mod download)", p)
		}
		ents, err := os.ReadDir(m.Dir)
		if err != nil {
			return err
		}
		var files []string
		for _, e := range ents {
			if !e.IsDir() && licenseFile.MatchString(e.Name()) {
				files = append(files, e.Name())
			}
		}
		if len(files) == 0 {
			return fmt.Errorf("%s: no license file found in %s", p, m.Dir)
		}
		if err := section(w, m.Path, m.Version, m.Dir, files...); err != nil {
			return err
		}
	}
	return nil
}

func section(w io.Writer, name, version, dir string, files ...string) error {
	fmt.Fprintf(w, "\n%s\n%s %s\n%s\n", strings.Repeat("=", 78), name, version, strings.Repeat("=", 78))
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "\n--- %s ---\n\n%s\n", f, bytes.TrimSpace(b))
	}
	return nil
}

func goOutput(env []string, args ...string) (string, error) {
	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}
