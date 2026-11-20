package master

import "strings"

// ZenginMaxLen は全銀協フォーマットの口座名義カナ最大文字数。
const ZenginMaxLen = 30

var zenginMap = func() map[rune]string {
	m := map[rune]string{}
	pair := func(from, to string, suffix string) {
		f, t := []rune(from), []rune(to)
		if len(f) != len(t) {
			panic("zengin table length mismatch")
		}
		for i := range f {
			m[f[i]] = string(t[i]) + suffix
		}
	}
	pair("アイウエオカキクケコサシスセソタチツテトナニヌネノハヒフヘホマミムメモヤユヨラリルレロワヲン",
		"ｱｲｳｴｵｶｷｸｹｺｻｼｽｾｿﾀﾁﾂﾃﾄﾅﾆﾇﾈﾉﾊﾋﾌﾍﾎﾏﾐﾑﾒﾓﾔﾕﾖﾗﾘﾙﾚﾛﾜｦﾝ", "")
	// 小文字カナは大文字化
	pair("ァィゥェォッャュョヮヵヶ", "ｱｲｳｴｵﾂﾔﾕﾖﾜｶｹ", "")
	pair("ｧｨｩｪｫｯｬｭｮ", "ｱｲｳｴｵﾂﾔﾕﾖ", "")
	// 濁点・半濁点は独立1文字に分離
	pair("ガギグゲゴザジズゼゾダヂヅデドバビブベボ", "ｶｷｸｹｺｻｼｽｾｿﾀﾁﾂﾃﾄﾊﾋﾌﾍﾎ", "ﾞ")
	pair("パピプペポ", "ﾊﾋﾌﾍﾎ", "ﾟ")
	m['ヴ'] = "ｳﾞ"
	m['ヰ'] = "ｲ"
	m['ヱ'] = "ｴ"
	m['ー'] = "ｰ"
	m['－'] = "ｰ"
	m['゛'] = "ﾞ"
	m['゜'] = "ﾟ"
	m['　'] = " "
	m[' '] = " "
	return m
}()

// ToKatakana はひらがなを全角カタカナに変換する（その他の文字はそのまま）。
func ToKatakana(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'ぁ' && r <= 'ゖ' {
			return r + 0x60
		}
		return r
	}, s)
}

// ToHiragana は全角カタカナをひらがなに変換する（その他の文字はそのまま）。
func ToHiragana(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'ァ' && r <= 'ヶ' {
			return r - 0x60
		}
		return r
	}, s)
}

// ToZengin はひらがな・カタカナ文字列を全銀協フォーマットの半角カナに変換する。
//   - 濁点・半濁点は独立した 1 文字（ﾞ ﾟ）に分離
//   - 小文字カナ（ｧｨｩｪｫｯｬｭｮ 等）は大文字化
//   - 長音は ｰ、空白は半角スペース 1 文字
//   - 英字は大文字化、数字はそのまま、それ以外の文字は除去
//   - 最大 30 文字で切り詰め
func ToZengin(s string) string {
	var b strings.Builder
	n := 0
	emit := func(str string) bool {
		for _, r := range str {
			if n >= ZenginMaxLen {
				return false
			}
			b.WriteRune(r)
			n++
		}
		return true
	}
	for _, r := range ToKatakana(s) {
		var out string
		switch {
		case zenginMap[r] != "":
			out = zenginMap[r]
		case r >= 'ｱ' && r <= 'ﾝ', r == 'ｦ', r == 'ｰ', r == 'ﾞ', r == 'ﾟ':
			out = string(r)
		case r >= '0' && r <= '9', r >= 'A' && r <= 'Z':
			out = string(r)
		case r >= 'a' && r <= 'z':
			out = string(r - 'a' + 'A')
		case r >= '０' && r <= '９':
			out = string(r - '０' + '0')
		case r >= 'Ａ' && r <= 'Ｚ':
			out = string(r - 'Ａ' + 'A')
		case r >= 'ａ' && r <= 'ｚ':
			out = string(r - 'ａ' + 'A')
		default:
			continue
		}
		if !emit(out) {
			break
		}
	}
	return b.String()
}
