package masterbuild

import (
	"fmt"
	"io"
	"slices"

	"github.com/withnic/katashiro/internal/master"
)

const maxIssuesShown = 30

// Report はクレンジング結果と生成辞書の検査結果を w に書く。問題があればエラーを返す。st は nil でもよい。
func Report(w io.Writer, d *master.Dict, st *KenAllStats) error {
	if st != nil {
		fmt.Fprintf(w, "== ken_all クレンジング ==\n入力行数: %d\n", st.Rows)
		acts := []cleanseAction{actKeep, actStripParen, actNoListing, actNextBanchi, actIchien, actRange, actFragment}
		for _, a := range acts {
			fmt.Fprintf(w, "  %-28s %7d\n", a, st.Actions[a])
			for _, ex := range st.Examples[a] {
				fmt.Fprintf(w, "      例: %s\n", ex)
			}
		}
	}

	fmt.Fprintf(w, "\n== 住所辞書 ==\nレコード数（重複統合後）: %d\n", d.Addresses.Len())
	empty := 0
	var issues []string
	for _, p := range master.Prefectures {
		s, e := d.PrefectureRange(p.Code)
		fmt.Fprintf(w, "  %02d %-5s 公安委員会=%s %6d件\n", p.Code, p.Name, p.PSCCode, e-s)
		for i := s; i < e; i++ {
			a := d.Address(i)
			if a.Town == "" {
				empty++
			}
			for _, msg := range master.AddressIssues(a) {
				issues = append(issues, fmt.Sprintf("%s %s %s %q: %s", a.Zip, a.Prefecture, a.City, a.Town, msg))
			}
		}
	}
	fmt.Fprintf(w, "町域が空文字: %d\n", empty)

	nameIssues := 0
	for g, label := range []string{"姓", "男性名", "女性名"} {
		s, e := d.Names.Group(g)
		fmt.Fprintf(w, "%s: %d件\n", label, e-s)
		for i := s; i < e; i++ {
			for _, msg := range master.NameIssues(d.Name(i)) {
				issues = append(issues, fmt.Sprintf("%s %s/%s: %s", label, d.Name(i).Kanji, d.Name(i).Hiragana, msg))
				nameIssues++
			}
		}
	}

	if len(issues) == 0 {
		fmt.Fprintln(w, "\n問題は見つかりませんでした。")
		return nil
	}
	slices.Sort(issues)
	fmt.Fprintf(w, "\n== 問題 %d 件（先頭 %d 件を表示） ==\n", len(issues), min(len(issues), maxIssuesShown))
	for _, s := range issues[:min(len(issues), maxIssuesShown)] {
		fmt.Fprintln(w, "  "+s)
	}
	return fmt.Errorf("%d issues found", len(issues))
}
