package model

import (
	"fmt"

	"github.com/withnic/katashiro/internal/master"
)

// nameSet は --set で指定された姓または名（漢字と、ひらがな・カタカナの読み）。
type nameSet struct {
	kanji, hiragana, katakana string
}

// resolve は漢字と読みから名前を組み立て、カナ・全銀協カナ・ローマ字を導出する。
// 読み（ひらがなかカタカナ）は必須。両方あれば一致していなければならない。
// 辞書は引かない（辞書の版によって結果が変わらないようにするため）。
func (s nameSet) resolve(label string) (master.Name, error) {
	reading := s.hiragana
	if s.katakana != "" {
		h := master.ToHiragana(s.katakana)
		if reading != "" && reading != h {
			return master.Name{}, fmt.Errorf("%s の読みが一致しません: ひらがな %q / カタカナ %q", label, s.hiragana, s.katakana)
		}
		reading = h
	}
	if reading == "" {
		return master.Name{}, fmt.Errorf("%s=%s には読みが必要です。--set personal.%s_hiragana=<よみ> または personal.%s_katakana=<ヨミ> も指定してください",
			label, s.kanji, label[len("personal."):], label[len("personal."):])
	}
	e := master.NameEntry{Kanji: s.kanji, Hiragana: reading}.Complete()
	n := master.Name{Kanji: e.Kanji, Hiragana: e.Hiragana, Katakana: e.Katakana, Zengin: e.ZenginKana, Romaji: e.Romaji}
	if issues := master.NameIssues(n); len(issues) > 0 {
		return n, fmt.Errorf("%s: 読み %q から名前を組み立てられません: %v", label, reading, issues)
	}
	return n, nil
}
