package engine

// LuhnCheckDigit は数字列 payload（チェックディジットを除く）に対する
// Luhn（Modulus 10）チェックディジットを返す。payload は ASCII 数字のみとする。
func LuhnCheckDigit(payload string) byte {
	sum := 0
	double := true // 右端（チェックディジットの左隣）から 2 倍する
	for i := len(payload) - 1; i >= 0; i-- {
		d := int(payload[i] - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return byte('0' + (10-sum%10)%10)
}

// LuhnValid は number 全体（末尾がチェックディジット）が Luhn を満たすかを返す。
func LuhnValid(number string) bool {
	if len(number) < 2 || !allDigits(number) {
		return false
	}
	return LuhnCheckDigit(number[:len(number)-1]) == number[len(number)-1]
}

// MyNumberCheckDigit は個人番号（11桁）の検査用数字を
// 「行政手続における特定の個人を識別するための番号の利用等に関する法律施行令」第8条の
// モジュラス11方式で算出する。
//
//	P = 11 - (Σ Pn × Qn mod 11)、Qn = n+1 (1≤n≤6) / n-6 (7≤n≤11)、余り ≤ 1 のとき 0
//
// Pn は右端（最下位）から数えた n 桁目。
func MyNumberCheckDigit(payload string) byte {
	if len(payload) != 11 || !allDigits(payload) {
		panic("engine: my number payload must be 11 digits")
	}
	sum := 0
	for n := 1; n <= 11; n++ {
		p := int(payload[11-n] - '0')
		q := n + 1
		if n >= 7 {
			q = n - 6
		}
		sum += p * q
	}
	rem := sum % 11
	if rem <= 1 {
		return '0'
	}
	return byte('0' + 11 - rem)
}

// MyNumberValid は 12 桁の個人番号が検査用数字を満たすかを返す。
func MyNumberValid(number string) bool {
	if len(number) != 12 || !allDigits(number) {
		return false
	}
	return MyNumberCheckDigit(number[:11]) == number[11]
}

// LicenseCheckDigit は運転免許証番号の先頭10桁に対する検査用数字を、
// 右端から重み 2,3,4,5,6,7 を循環させるモジュラス11方式で算出する（余り ≤ 1 のとき 0）。
func LicenseCheckDigit(payload string) byte {
	if len(payload) != 10 || !allDigits(payload) {
		panic("engine: license payload must be 10 digits")
	}
	sum := 0
	for n := 0; n < len(payload); n++ {
		d := int(payload[len(payload)-1-n] - '0')
		sum += d * (n%6 + 2)
	}
	rem := sum % 11
	if rem <= 1 {
		return '0'
	}
	return byte('0' + 11 - rem)
}

// LicenseValid は 12 桁の運転免許証番号の 11 桁目が検査用数字を満たすかを返す。
func LicenseValid(number string) bool {
	if len(number) != 12 || !allDigits(number) {
		return false
	}
	return LicenseCheckDigit(number[:10]) == number[10]
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
