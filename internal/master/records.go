package master

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// AddressEntry は住所マスタ 1 件の交換用表現（genmaster / --master-dir の JSON）。
type AddressEntry struct {
	Zip        string `json:"zip"`
	Prefecture string `json:"prefecture"`
	City       string `json:"city"`
	Town       string `json:"town"`
}

// NameEntry は人名マスタ 1 件の交換用表現。katakana / zengin_kana / romaji は
// 省略時に hiragana から導出する。
type NameEntry struct {
	Kanji      string `json:"kanji"`
	Hiragana   string `json:"hiragana"`
	Katakana   string `json:"katakana,omitempty"`
	ZenginKana string `json:"zengin_kana,omitempty"`
	Romaji     string `json:"romaji,omitempty"`
}

// NamesFile は names.json の構造。
type NamesFile struct {
	LastNames []NameEntry `json:"last_names"`
	Male      []NameEntry `json:"male"`
	Female    []NameEntry `json:"female"`
}

// Complete は欠けている読み派生フィールドを埋める。
func (n NameEntry) Complete() NameEntry {
	if n.Katakana == "" {
		n.Katakana = ToKatakana(n.Hiragana)
	}
	if n.ZenginKana == "" {
		n.ZenginKana = ToZengin(n.Katakana)
	}
	if n.Romaji == "" {
		n.Romaji = ToHepburn(n.Hiragana)
	}
	return n
}

// BuildAddresses は住所エントリを都道府県別にグループ化・重複統合してバイナリ化する。
func BuildAddresses(entries []AddressEntry) ([]byte, error) {
	groups := make([][][]string, len(Prefectures))
	seen := map[AddressEntry]bool{}
	for _, e := range entries {
		p, ok := PrefectureByName(e.Prefecture)
		if !ok {
			return nil, fmt.Errorf("unknown prefecture %q", e.Prefecture)
		}
		if len(e.Zip) != 7 {
			return nil, fmt.Errorf("zip code must be 7 digits: %q", e.Zip)
		}
		if seen[e] {
			continue
		}
		seen[e] = true
		groups[p.Code-1] = append(groups[p.Code-1], []string{e.Zip, e.Prefecture, e.City, e.Town, p.PSCCode})
	}
	for _, g := range groups {
		slices.SortStableFunc(g, func(a, b []string) int { return cmp.Compare(a[0], b[0]) })
	}
	return BuildTable(AddrFields, groups)
}

// BuildNames は人名エントリを姓 / 男性名 / 女性名のグループでバイナリ化する。
// kanji+hiragana の組で一意化する（同字異音は別レコードとして保持）。
func BuildNames(f NamesFile) ([]byte, error) {
	conv := func(in []NameEntry) [][]string {
		seen := map[[2]string]bool{}
		var out [][]string
		for _, e := range in {
			if e.Kanji == "" || e.Hiragana == "" {
				continue
			}
			k := [2]string{e.Kanji, e.Hiragana}
			if seen[k] {
				continue
			}
			seen[k] = true
			e = e.Complete()
			out = append(out, []string{e.Kanji, e.Hiragana, e.Katakana, e.ZenginKana, e.Romaji})
		}
		return out
	}
	return BuildTable(NameFields, [][][]string{conv(f.LastNames), conv(f.Male), conv(f.Female)})
}

// LoadDir は外部の辞書ディレクトリを読み込む（--master-dir / KATASHIRO_MASTER_DIR 指定時のみ使用）。
// addresses.bin / names.bin（katashiro master fetch で作成）があればそれを、なければ
// addresses.json / names.json を読む。通常起動は埋め込み辞書のゼロパース参照。
func LoadDir(dir string) (*Dict, error) {
	ab, aerr := os.ReadFile(filepath.Join(dir, AddressesBinName))
	nb, nerr := os.ReadFile(filepath.Join(dir, NamesBinName))
	switch {
	case aerr == nil && nerr == nil:
		d, err := NewDict(ab, nb)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", dir, err)
		}
		return d, nil
	case aerr == nil || nerr == nil:
		return nil, fmt.Errorf("%s: %s と %s の両方が必要です", dir, AddressesBinName, NamesBinName)
	}
	var addrs []AddressEntry
	if err := readJSON(filepath.Join(dir, "addresses.json"), &addrs); err != nil {
		return nil, err
	}
	var names NamesFile
	if err := readJSON(filepath.Join(dir, "names.json"), &names); err != nil {
		return nil, err
	}
	ab, err := BuildAddresses(addrs)
	if err != nil {
		return nil, fmt.Errorf("addresses.json: %w", err)
	}
	nb, err = BuildNames(names)
	if err != nil {
		return nil, fmt.Errorf("names.json: %w", err)
	}
	return NewDict(ab, nb)
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// townResidues はクレンジング後の町域に残っていてはいけない表記。
var townResidues = []string{"（", "）", "(", ")", "「", "」", "〜", "～", "、", "以下に掲載がない場合", "がくる場合", "を除く", "以外"}

// AddressIssues は住所レコードの不整合（クレンジング漏れ・形式違反）を列挙する。
func AddressIssues(a Address) []string {
	var out []string
	if len(a.Zip) != 7 || !isASCIIDigits(a.Zip) {
		out = append(out, "郵便番号が7桁の数字ではない")
	}
	p, ok := PrefectureByName(a.Prefecture)
	switch {
	case !ok:
		out = append(out, "不明な都道府県")
	case p.PSCCode != a.PSCCode:
		out = append(out, "公安委員会コードが都道府県と一致しない")
	}
	if a.City == "" {
		out = append(out, "市区町村が空")
	}
	for _, r := range townResidues {
		if strings.Contains(a.Town, r) {
			out = append(out, "町域にクレンジング漏れ「"+r+"」")
		}
	}
	if strings.TrimSpace(a.Town) != a.Town {
		out = append(out, "町域の前後に空白")
	}
	return out
}

// NameIssues は人名レコードの不整合（読み・全銀協カナ・ローマ字の形式違反）を列挙する。
func NameIssues(n Name) []string {
	var out []string
	for _, r := range n.Hiragana {
		if (r < 'ぁ' || r > 'ゖ') && r != 'ー' {
			out = append(out, "読みにひらがな以外の文字")
			break
		}
	}
	if n.Katakana != ToKatakana(n.Hiragana) {
		out = append(out, "カタカナが読みと一致しない")
	}
	if n.Zengin == "" || n.Zengin != ToZengin(n.Katakana) {
		out = append(out, "全銀協カナが不正")
	}
	for _, small := range "ｧｨｩｪｫｯｬｭｮ" {
		if strings.ContainsRune(n.Zengin, small) {
			out = append(out, "全銀協カナに小文字カナ")
			break
		}
	}
	if n.Romaji == "" {
		out = append(out, "ローマ字が空")
	}
	for _, c := range n.Romaji {
		if c < 'a' || c > 'z' {
			out = append(out, "ローマ字に a-z 以外の文字")
			break
		}
	}
	return out
}

func isASCIIDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
