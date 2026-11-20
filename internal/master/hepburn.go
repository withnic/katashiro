package master

import "strings"

var hepburnDigraphs = map[string]string{
	"きゃ": "kya", "きゅ": "kyu", "きょ": "kyo",
	"しゃ": "sha", "しゅ": "shu", "しょ": "sho", "しぇ": "she",
	"ちゃ": "cha", "ちゅ": "chu", "ちょ": "cho", "ちぇ": "che",
	"にゃ": "nya", "にゅ": "nyu", "にょ": "nyo",
	"ひゃ": "hya", "ひゅ": "hyu", "ひょ": "hyo",
	"みゃ": "mya", "みゅ": "myu", "みょ": "myo",
	"りゃ": "rya", "りゅ": "ryu", "りょ": "ryo",
	"ぎゃ": "gya", "ぎゅ": "gyu", "ぎょ": "gyo",
	"じゃ": "ja", "じゅ": "ju", "じょ": "jo", "じぇ": "je",
	"ぢゃ": "ja", "ぢゅ": "ju", "ぢょ": "jo",
	"びゃ": "bya", "びゅ": "byu", "びょ": "byo",
	"ぴゃ": "pya", "ぴゅ": "pyu", "ぴょ": "pyo",
	"ふぁ": "fa", "ふぃ": "fi", "ふぇ": "fe", "ふぉ": "fo",
	"てぃ": "ti", "でぃ": "di", "うぃ": "wi", "うぇ": "we", "うぉ": "wo",
	"ゔぁ": "va", "ゔぃ": "vi", "ゔぇ": "ve", "ゔぉ": "vo",
}

var hepburnMono = func() map[rune]string {
	m := map[rune]string{}
	kana := []rune("あいうえおかきくけこさしすせそたちつてとなにぬねのはひふへほまみむめもやゆよらりるれろわゐゑをんがぎぐげござじずぜぞだぢづでどばびぶべぼぱぴぷぺぽゔぁぃぅぇぉゃゅょゎ")
	roma := strings.Fields("a i u e o ka ki ku ke ko sa shi su se so ta chi tsu te to na ni nu ne no ha hi fu he ho ma mi mu me mo ya yu yo ra ri ru re ro wa i e o n ga gi gu ge go za ji zu ze zo da ji zu de do ba bi bu be bo pa pi pu pe po vu a i u e o ya yu yo wa")
	if len(kana) != len(roma) {
		panic("hepburn table length mismatch")
	}
	for i, r := range kana {
		m[r] = roma[i]
	}
	return m
}()

// ToHepburn はひらがな・カタカナの読みを修正ヘボン式ローマ字（小文字 ASCII）に変換する。
// 人名表記の慣例に合わせ長音は省略する（「おう」「おお」→ o、「うう」→ u、ー は無視）。
// ただし「おう」の直後が母音で始まる場合（いのうえ → inoue）は省略しない。
// 撥音「ん」は常に n、促音「っ」は後続子音を重ねる（ch の前は t）。
// 変換できない文字は除去する。
func ToHepburn(reading string) string {
	rs := []rune(ToHiragana(reading))
	var syl []string
	for i := 0; i < len(rs); i++ {
		if i+1 < len(rs) {
			if s, ok := hepburnDigraphs[string(rs[i:i+2])]; ok {
				syl = append(syl, s)
				i++
				continue
			}
		}
		switch r := rs[i]; r {
		case 'っ':
			syl = append(syl, "っ")
		case 'ー':
			// 長音は省略
		default:
			if s, ok := hepburnMono[r]; ok {
				syl = append(syl, s)
			}
		}
	}
	var b strings.Builder
	for i, s := range syl {
		if s == "っ" {
			if i+1 < len(syl) && syl[i+1] != "っ" {
				next := syl[i+1]
				switch {
				case strings.HasPrefix(next, "ch"):
					b.WriteByte('t')
				case !isVowel(next[0]) && next != "n":
					b.WriteByte(next[0])
				}
			}
			continue
		}
		cur := b.String()
		if cur != "" {
			last := cur[len(cur)-1]
			nextVowel := i+1 < len(syl) && syl[i+1] != "っ" && syl[i+1] != "" && isVowel(syl[i+1][0])
			if s == "u" && (last == 'u' || (last == 'o' && !nextVowel)) {
				continue
			}
			if s == "o" && last == 'o' {
				continue
			}
		}
		b.WriteString(s)
	}
	return b.String()
}

func isVowel(c byte) bool {
	return c == 'a' || c == 'i' || c == 'u' || c == 'e' || c == 'o'
}
