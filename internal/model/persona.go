// Package model は seqID から Persona を決定論的に合成する生成パイプラインと、
// --set によるスマートオーバーライドを提供する。
package model

import "time"

// Persona はダミー個人データ 1 件。
type Persona struct {
	ID       uint64   `json:"id"`
	Personal Personal `json:"personal"`
	Contact  Contact  `json:"contact"`
	Finance  *Finance `json:"finance,omitempty"`
	KYC      *KYC     `json:"kyc,omitempty"`
	Profile  *Profile `json:"profile,omitempty"`
}

// Personal は基本属性・氏名。
type Personal struct {
	Birthday          string `json:"birthday"`
	Age               int    `json:"age"`
	Gender            string `json:"gender"`
	LastName          string `json:"last_name"`
	FirstName         string `json:"first_name"`
	FullName          string `json:"full_name"`
	LastNameHiragana  string `json:"last_name_hiragana"`
	FirstNameHiragana string `json:"first_name_hiragana"`
	FullNameHiragana  string `json:"full_name_hiragana"`
	LastNameKatakana  string `json:"last_name_katakana"`
	FirstNameKatakana string `json:"first_name_katakana"`
	FullNameKatakana  string `json:"full_name_katakana"`
	BankHolderKana    string `json:"bank_holder_kana"`
	RomajiName        string `json:"romaji_name"`
}

// Contact は住所・連絡先。
type Contact struct {
	ZipCode       string `json:"zip_code"`
	Prefecture    string `json:"prefecture"`
	City          string `json:"city"`
	Town          string `json:"town"`
	StreetAddress string `json:"street_address"`
	Building      string `json:"building"`
	PhoneNumber   string `json:"phone_number"`
	Email         string `json:"email"`
}

// Finance は --with-finance モジュール。
type Finance struct {
	CreditCard  CreditCard  `json:"credit_card"`
	BankAccount BankAccount `json:"bank_account"`
	MyNumber    string      `json:"my_number"`
}

// CreditCard はクレジットカード情報。
type CreditCard struct {
	Brand          string `json:"brand"`
	Number         string `json:"number"`
	ExpirationDate string `json:"expiration_date"`
	CVV            string `json:"cvv"`
	CardHolder     string `json:"card_holder"`
}

// BankAccount は銀行口座情報。
type BankAccount struct {
	BankCode          string `json:"bank_code"`
	BankName          string `json:"bank_name"`
	BranchCode        string `json:"branch_code"`
	BranchName        string `json:"branch_name"`
	AccountType       string `json:"account_type"`
	AccountNumber     string `json:"account_number"`
	AccountHolderKana string `json:"account_holder_kana"`
}

// KYC は --with-kyc モジュール。18歳未満の運転免許証番号は nil（JSON では null）。
type KYC struct {
	DriversLicenseNumber *string `json:"drivers_license_number"`
	PassportNumber       string  `json:"passport_number"`
	DocumentType         string  `json:"document_type"` // drivers_license / passport（本人確認に提示する書類）
}

// Profile は --with-profile モジュール。
type Profile struct {
	Occupation         string `json:"occupation"`
	Industry           string `json:"industry"`
	CompanyName        string `json:"company_name"`
	AnnualIncomeManYen int    `json:"annual_income_man_yen"`
	IsMarried          bool   `json:"is_married"`
	DependentsCount    int    `json:"dependents_count"`
}

// DateLayout は日付の入出力書式。
const DateLayout = "2006-01-02"

// ParseDate は YYYY-MM-DD を UTC の日付として解釈する。
func ParseDate(s string) (time.Time, error) {
	return time.ParseInLocation(DateLayout, s, time.UTC)
}

// AgeAt は基準日 base 時点の満年齢を、浮動小数点を使わずカレンダー上の
// 月日比較で厳密に算出する（(BaseMonth, BaseDay) < (BirthMonth, BirthDay) なら -1）。
func AgeAt(birth, base time.Time) int {
	age := base.Year() - birth.Year()
	if base.Month() < birth.Month() || (base.Month() == birth.Month() && base.Day() < birth.Day()) {
		age--
	}
	return age
}

// BirthdayRange は基準日時点で満 lo 歳以上 hi 歳以下となる生年月日の範囲
// [earliest, earliest+days) を返す。うるう日も含めカレンダー比較で境界を確定する。
func BirthdayRange(base time.Time, lo, hi int) (earliest time.Time, days int) {
	day := func(t time.Time, n int) time.Time { return t.AddDate(0, 0, n) }
	latest := base.AddDate(-lo, 0, 0)
	for AgeAt(latest, base) < lo {
		latest = day(latest, -1)
	}
	for next := day(latest, 1); !next.After(base) && AgeAt(next, base) >= lo; next = day(latest, 1) {
		latest = next
	}
	earliest = base.AddDate(-(hi + 1), 0, 0)
	for AgeAt(earliest, base) > hi {
		earliest = day(earliest, 1)
	}
	for prev := day(earliest, -1); AgeAt(prev, base) <= hi; prev = day(earliest, -1) {
		earliest = prev
	}
	return earliest, int(latest.Sub(earliest).Hours()/24) + 1
}
