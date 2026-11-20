// Package masterbuild は住所・人名マスタの取得・クレンジング・検査・バイナリ化を行う。
// genmaster（埋め込み辞書の生成）と katashiro master fetch（利用者による実データ取得）が共用する。
package masterbuild

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/withnic/katashiro/internal/master"
)

// 外部ソースの既定 URL。
const (
	DefaultKenAllURL = "https://www.post.japanpost.jp/service/search/zipcode/download/utf/zip/utf_ken_all.zip"
	DefaultNamesURL  = "https://raw.githubusercontent.com/willnet/gimei/main/lib/data/names.yml"
)

// SourceBuiltinSeed は内蔵シードを使ったことを示すソース名。
const SourceBuiltinSeed = "builtin-seed"

// Sources は取り込むソース。空文字は内蔵シードを使う。
type Sources struct {
	KenAll string // utf_ken_all.zip / CSV のパスまたは URL
	Names  string // gimei 形式 names.yml のパスまたは URL
}

// Result は Build の結果。
type Result struct {
	Sources      Sources
	Addresses    []master.AddressEntry
	Names        master.NamesFile
	Stats        *KenAllStats // ken_all を取り込んだときだけ
	AddressesBin []byte
	NamesBin     []byte
	Dict         *master.Dict
}

// Build はソースを読み込み、クレンジングしてバイナリ辞書を組み立てる（47都道府県・姓/男性名/女性名の存在も検証する）。
func Build(src Sources) (*Result, error) {
	r := &Result{Sources: src}
	var err error
	if r.Addresses, err = seedAddressEntries(); err != nil {
		return nil, err
	}
	if src.KenAll != "" {
		b, err := readSource(src.KenAll)
		if err != nil {
			return nil, fmt.Errorf("ken-all: %w", err)
		}
		if r.Addresses, r.Stats, err = parseKenAll(b); err != nil {
			return nil, fmt.Errorf("ken-all: %w", err)
		}
	}
	r.Names = seedNamesFile()
	if src.Names != "" {
		b, err := readSource(src.Names)
		if err != nil {
			return nil, fmt.Errorf("names: %w", err)
		}
		if r.Names, err = parseGimeiYAML(b); err != nil {
			return nil, fmt.Errorf("names: %w", err)
		}
	}
	if r.AddressesBin, err = master.BuildAddresses(r.Addresses); err != nil {
		return nil, err
	}
	if r.NamesBin, err = master.BuildNames(r.Names); err != nil {
		return nil, err
	}
	if r.Dict, err = master.NewDict(r.AddressesBin, r.NamesBin); err != nil {
		return nil, err
	}
	return r, nil
}

// Check はクレンジング結果を検査する。問題があればレポート全文を含むエラーを返す。
func (r *Result) Check() error {
	var buf bytes.Buffer
	if err := Report(&buf, r.Dict, r.Stats); err != nil {
		return fmt.Errorf("%w\n%s", err, buf.String())
	}
	return nil
}

// WriteBins は addresses.bin / names.bin を dir に書き出す（埋め込み辞書用）。
func (r *Result) WriteBins(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, master.AddressesBinName), r.AddressesBin, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, master.NamesBinName), r.NamesBin, 0o644)
}

// WriteJSON は確認用の addresses.json / names.json を dir に書き出す（--master-dir の JSON 形式）。
func (r *Result) WriteJSON(dir string) error {
	if err := writeJSONFile(filepath.Join(dir, "addresses.json"), r.Addresses); err != nil {
		return err
	}
	return writeJSONFile(filepath.Join(dir, "names.json"), r.Names)
}

// Manifest は辞書ディレクトリの由来情報（manifest.json）。
type Manifest struct {
	Format          string `json:"format"`
	CreatedAt       string `json:"created_at"`
	AddressesSource string `json:"addresses_source"`
	NamesSource     string `json:"names_source"`
	Addresses       int    `json:"addresses"`
	LastNames       int    `json:"last_names"`
	MaleNames       int    `json:"male_names"`
	FemaleNames     int    `json:"female_names"`
	Fingerprint     string `json:"fingerprint"`
}

// sourceName は manifest・NOTICE に書くソース名。辞書を共有しても作成者のディレクトリ構成が出ないよう、
// ローカルファイルはファイル名だけにする。
func sourceName(s string) string {
	switch {
	case s == "":
		return SourceBuiltinSeed
	case strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://"):
		return s
	}
	return filepath.Base(s)
}

// dirFiles は WriteDir が書き出すファイル（この順に置き換える）。
var dirFiles = []string{master.AddressesBinName, master.NamesBinName, master.ManifestFile, "NOTICE"}

// WriteDir は利用者向けの辞書ディレクトリ（bin・manifest.json・NOTICE）を書き出す。
// 書き込みの途中で失敗しても dir に中途半端な辞書が残らないよう、同じ親ディレクトリの一時ディレクトリに
// すべて書き終えてから、各ファイルを rename で dir に移す。dir にある他のファイルには触れない。
func (r *Result) WriteDir(dir string, now time.Time) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(filepath.Dir(filepath.Clean(dir)), ".katashiro-master-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := r.writeFiles(staging, now); err != nil {
		return err
	}
	for _, name := range dirFiles {
		if err := os.Rename(filepath.Join(staging, name), filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}

func (r *Result) writeFiles(dir string, now time.Time) error {
	if err := r.WriteBins(dir); err != nil {
		return err
	}
	st := r.Dict.Stats()
	m := Manifest{
		Format:          "KTS1",
		CreatedAt:       now.UTC().Format(time.RFC3339),
		AddressesSource: sourceName(r.Sources.KenAll),
		NamesSource:     sourceName(r.Sources.Names),
		Addresses:       st.Addresses, LastNames: st.LastNames, MaleNames: st.MaleNames, FemaleNames: st.FemaleNames,
		Fingerprint: master.Fingerprint(r.AddressesBin, r.NamesBin),
	}
	if err := writeJSONFile(filepath.Join(dir, master.ManifestFile), m); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "NOTICE"), []byte(r.notice()), 0o644)
}

// notice は辞書を配布するときに同梱するデータ出典・ライセンス表示。
func (r *Result) notice() string {
	s := "このディレクトリの辞書は katashiro master fetch で生成したものです。\n\n"
	if r.Sources.KenAll != "" {
		s += `== 郵便番号データ ==
出典: 日本郵便株式会社 郵便番号データ（` + sourceName(r.Sources.KenAll) + `）
「郵便番号データに限っては日本郵便株式会社は著作権を主張しません。自由に配布していただいて結構です。」
https://www.post.japanpost.jp/zipcode/download.html

`
	}
	if r.Sources.Names != "" {
		s += `== 人名データ ==
出典: willnet/gimei lib/data/names.yml（` + sourceName(r.Sources.Names) + `）
https://github.com/willnet/gimei

` + gimeiLicense
	}
	return s
}

const gimeiLicense = `Copyright (c) 2012 willnet

MIT License

Permission is hereby granted, free of charge, to any person obtaining
a copy of this software and associated documentation files (the
"Software"), to deal in the Software without restriction, including
without limitation the rights to use, copy, modify, merge, publish,
distribute, sublicense, and/or sell copies of the Software, and to
permit persons to whom the Software is furnished to do so, subject to
the following conditions:

The above copyright notice and this permission notice shall be
included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
`

func writeJSONFile(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
