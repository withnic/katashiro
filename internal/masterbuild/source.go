package masterbuild

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/withnic/katashiro/internal/master"
)

func seedAddressEntries() ([]master.AddressEntry, error) {
	var out []master.AddressEntry
	for _, line := range strings.Split(seedAddresses, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Split(line, "|")
		if len(f) != 4 {
			return nil, fmt.Errorf("bad seed address line %q", line)
		}
		out = append(out, master.AddressEntry{Prefecture: f[0], Zip: f[1], City: f[2], Town: f[3]})
	}
	return out, nil
}

func seedNamesFile() master.NamesFile {
	parse := func(s string) []master.NameEntry {
		var out []master.NameEntry
		for _, tok := range strings.Fields(s) {
			k, h, _ := strings.Cut(tok, ":")
			out = append(out, master.NameEntry{Kanji: k, Hiragana: h})
		}
		return out
	}
	return master.NamesFile{
		LastNames: parse(seedLastNames),
		Male:      parse(seedMaleNames),
		Female:    parse(seedFemaleNames),
	}
}

func readSource(src string) ([]byte, error) {
	if !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") {
		return os.ReadFile(src)
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(src)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", src, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

var parenRe = regexp.MustCompile(`（[^（）]*）`)

// cleanseAction は町域クレンジングで行った処理の種類（-check のレポート用）。
type cleanseAction string

const (
	actKeep       cleanseAction = "そのまま"
	actStripParen cleanseAction = "括弧書きを除去"
	actNoListing  cleanseAction = "「以下に掲載がない場合」→空文字"
	actNextBanchi cleanseAction = "「〜の次に…がくる場合」→空文字"
	actIchien     cleanseAction = "「〜一円」→空文字"
	actRange      cleanseAction = "範囲表記（〜）→空文字"
	actFragment   cleanseAction = "複数行分割の断片として除外"
)

// cleanseTown は ken_all の町域をクレンジングする。actFragment のときレコードごと除外する。
func cleanseTown(town string) (string, cleanseAction) {
	// 括弧書きを先に除去する（「琴平町の次に４２７番地以降がくる場合（川東）」のように注記の後ろに括弧が付く行があるため）
	act := actKeep
	if stripped := parenRe.ReplaceAllString(town, ""); stripped != town {
		town, act = stripped, actStripParen
	}
	switch {
	case town == "以下に掲載がない場合":
		return "", actNoListing
	case strings.Contains(town, "の次に") && strings.HasSuffix(town, "がくる場合"):
		return "", actNextBanchi
	case strings.HasSuffix(town, "一円"):
		return "", actIchien
	}
	if strings.ContainsAny(town, "（）、") {
		return "", actFragment
	}
	if strings.ContainsAny(town, "〜～") { // 「越中畑６４地割〜越中畑６６地割」のような範囲表記
		return "", actRange
	}
	return strings.TrimSpace(town), act
}

// KenAllStats は ken_all 取り込み時のクレンジング統計。
type KenAllStats struct {
	Rows     int
	Actions  map[cleanseAction]int
	Examples map[cleanseAction][]string // 処理ごとの元の町域（先頭数件）
}

const examplesPerAction = 5

// parseKenAll は utf_ken_all.zip（または展開済み CSV）を読み、クレンジング済み住所を返す。
func parseKenAll(b []byte) ([]master.AddressEntry, *KenAllStats, error) {
	st := &KenAllStats{Actions: map[cleanseAction]int{}, Examples: map[cleanseAction][]string{}}
	var r io.Reader = bytes.NewReader(b)
	if bytes.HasPrefix(b, []byte("PK")) {
		zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			return nil, nil, err
		}
		var f *zip.File
		for _, zf := range zr.File {
			if strings.HasSuffix(strings.ToLower(zf.Name), ".csv") {
				f = zf
				break
			}
		}
		if f == nil {
			return nil, nil, errors.New("no CSV in zip")
		}
		rc, err := f.Open()
		if err != nil {
			return nil, nil, err
		}
		defer rc.Close()
		r = rc
	}
	cr := csv.NewReader(bufio.NewReader(r))
	cr.FieldsPerRecord = -1
	var out []master.AddressEntry
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		if len(rec) < 9 {
			continue
		}
		st.Rows++
		town, act := cleanseTown(rec[8])
		st.Actions[act]++
		if act != actKeep && len(st.Examples[act]) < examplesPerAction {
			st.Examples[act] = append(st.Examples[act], rec[8])
		}
		if act == actFragment {
			continue
		}
		out = append(out, master.AddressEntry{Zip: rec[2], Prefecture: rec[6], City: rec[7], Town: town})
	}
	if len(out) == 0 {
		return nil, nil, errors.New("no address records")
	}
	return out, st, nil
}

// parseGimeiYAML は willnet/gimei の names.yml（first_name.male / first_name.female / last_name の
// [漢字, ひらがな, カタカナ, ...] リスト）だけを読む最小パーサ。ブロック形式（- - 漢字）と
// フロー形式（- ['漢字', 'ひらがな', ...]）の両方に対応する。ローマ字は修正ヘボン式で再導出する。
func parseGimeiYAML(b []byte) (master.NamesFile, error) {
	var nf master.NamesFile
	var stack []struct {
		indent int
		key    string
	}
	var cur *[]master.NameEntry
	var entry []string
	flush := func() {
		if cur != nil && len(entry) >= 2 {
			*cur = append(*cur, master.NameEntry{Kanji: entry[0], Hiragana: entry[1]})
		}
		entry = nil
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || trimmed == "---" {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		switch {
		case strings.HasPrefix(trimmed, "- [") && strings.HasSuffix(trimmed, "]"):
			// フロー形式: - ['漢字', 'ひらがな', 'カタカナ', 'Romaji']
			flush()
			for _, v := range strings.Split(strings.TrimSuffix(strings.TrimPrefix(trimmed, "- ["), "]"), ",") {
				entry = append(entry, unquote(v))
			}
			flush()
		case strings.HasPrefix(trimmed, "- - "):
			flush()
			entry = []string{unquote(strings.TrimPrefix(trimmed, "- - "))}
		case strings.HasPrefix(trimmed, "- "):
			entry = append(entry, unquote(strings.TrimPrefix(trimmed, "- ")))
		case strings.HasSuffix(trimmed, ":"):
			flush()
			for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, struct {
				indent int
				key    string
			}{indent, strings.TrimSuffix(trimmed, ":")})
			var path []string
			for _, s := range stack {
				path = append(path, s.key)
			}
			switch strings.Join(path, ".") {
			case "first_name.male":
				cur = &nf.Male
			case "first_name.female":
				cur = &nf.Female
			case "last_name":
				cur = &nf.LastNames
			default:
				cur = nil
			}
		}
	}
	flush()
	if err := sc.Err(); err != nil {
		return nf, err
	}
	if len(nf.LastNames) == 0 || len(nf.Male) == 0 || len(nf.Female) == 0 {
		return nf, errors.New("names.yml: missing last_name / first_name.male / first_name.female")
	}
	return nf, nil
}

func unquote(s string) string {
	return strings.Trim(strings.TrimSpace(s), `"'`)
}
