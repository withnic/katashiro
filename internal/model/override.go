package model

import (
	"fmt"
	"strconv"
)

// Module はフィールドが属するオプションモジュール。
type Module string

const (
	ModuleCore    Module = ""
	ModuleFinance Module = "finance"
	ModuleKYC     Module = "kyc"
	ModuleProfile Module = "profile"
)

// Kind はフラット出力時の値の型。
type Kind int

const (
	KindString Kind = iota
	KindInt
	KindBool
	KindNullableString
)

// Column はフラット列（CSV / SQLite）と、--set の単純上書き・--set-raw のキーの対応定義。
// 列順は仕様書「フラット列名」の順序に一致させる。
type Column struct {
	Name   string // フラット列名（例: personal_full_name）
	Key    string // --set キー（JSON パス、例: personal.full_name）
	Module Module
	Kind   Kind
	get    func(*Persona) any
	set    func(*Persona, string) error
}

// Value は p におけるこの列の値を返す（string / int / bool / *string）。
func (c *Column) Value(p *Persona) any { return c.get(p) }

func strCol(name, key string, m Module, ptr func(*Persona) *string) Column {
	return Column{Name: name, Key: key, Module: m, Kind: KindString,
		get: func(p *Persona) any { return *ptr(p) },
		set: func(p *Persona, v string) error { *ptr(p) = v; return nil }}
}

func intCol(name, key string, m Module, ptr func(*Persona) *int) Column {
	return Column{Name: name, Key: key, Module: m, Kind: KindInt,
		get: func(p *Persona) any { return *ptr(p) },
		set: func(p *Persona, v string) error {
			n, err := strconv.Atoi(v)
			if err != nil {
				return fmt.Errorf("integer required: %q", v)
			}
			*ptr(p) = n
			return nil
		}}
}

var allColumns = []Column{
	{Name: "id", Key: "id", Kind: KindInt,
		get: func(p *Persona) any { return int(p.ID) },
		set: func(p *Persona, v string) error {
			n, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return fmt.Errorf("integer required: %q", v)
			}
			p.ID = n
			return nil
		}},
	strCol("personal_birthday", "personal.birthday", ModuleCore, func(p *Persona) *string { return &p.Personal.Birthday }),
	intCol("personal_age", "personal.age", ModuleCore, func(p *Persona) *int { return &p.Personal.Age }),
	strCol("personal_gender", "personal.gender", ModuleCore, func(p *Persona) *string { return &p.Personal.Gender }),
	strCol("personal_last_name", "personal.last_name", ModuleCore, func(p *Persona) *string { return &p.Personal.LastName }),
	strCol("personal_first_name", "personal.first_name", ModuleCore, func(p *Persona) *string { return &p.Personal.FirstName }),
	strCol("personal_full_name", "personal.full_name", ModuleCore, func(p *Persona) *string { return &p.Personal.FullName }),
	strCol("personal_last_name_hiragana", "personal.last_name_hiragana", ModuleCore, func(p *Persona) *string { return &p.Personal.LastNameHiragana }),
	strCol("personal_first_name_hiragana", "personal.first_name_hiragana", ModuleCore, func(p *Persona) *string { return &p.Personal.FirstNameHiragana }),
	strCol("personal_full_name_hiragana", "personal.full_name_hiragana", ModuleCore, func(p *Persona) *string { return &p.Personal.FullNameHiragana }),
	strCol("personal_last_name_katakana", "personal.last_name_katakana", ModuleCore, func(p *Persona) *string { return &p.Personal.LastNameKatakana }),
	strCol("personal_first_name_katakana", "personal.first_name_katakana", ModuleCore, func(p *Persona) *string { return &p.Personal.FirstNameKatakana }),
	strCol("personal_full_name_katakana", "personal.full_name_katakana", ModuleCore, func(p *Persona) *string { return &p.Personal.FullNameKatakana }),
	strCol("personal_bank_holder_kana", "personal.bank_holder_kana", ModuleCore, func(p *Persona) *string { return &p.Personal.BankHolderKana }),
	strCol("personal_romaji_name", "personal.romaji_name", ModuleCore, func(p *Persona) *string { return &p.Personal.RomajiName }),
	strCol("contact_zip_code", "contact.zip_code", ModuleCore, func(p *Persona) *string { return &p.Contact.ZipCode }),
	strCol("contact_prefecture", "contact.prefecture", ModuleCore, func(p *Persona) *string { return &p.Contact.Prefecture }),
	strCol("contact_city", "contact.city", ModuleCore, func(p *Persona) *string { return &p.Contact.City }),
	strCol("contact_town", "contact.town", ModuleCore, func(p *Persona) *string { return &p.Contact.Town }),
	strCol("contact_street_address", "contact.street_address", ModuleCore, func(p *Persona) *string { return &p.Contact.StreetAddress }),
	strCol("contact_building", "contact.building", ModuleCore, func(p *Persona) *string { return &p.Contact.Building }),
	strCol("contact_phone_number", "contact.phone_number", ModuleCore, func(p *Persona) *string { return &p.Contact.PhoneNumber }),
	strCol("contact_email", "contact.email", ModuleCore, func(p *Persona) *string { return &p.Contact.Email }),
	strCol("finance_card_brand", "finance.credit_card.brand", ModuleFinance, func(p *Persona) *string { return &p.Finance.CreditCard.Brand }),
	strCol("finance_card_number", "finance.credit_card.number", ModuleFinance, func(p *Persona) *string { return &p.Finance.CreditCard.Number }),
	strCol("finance_card_expiration", "finance.credit_card.expiration_date", ModuleFinance, func(p *Persona) *string { return &p.Finance.CreditCard.ExpirationDate }),
	strCol("finance_card_cvv", "finance.credit_card.cvv", ModuleFinance, func(p *Persona) *string { return &p.Finance.CreditCard.CVV }),
	strCol("finance_card_holder", "finance.credit_card.card_holder", ModuleFinance, func(p *Persona) *string { return &p.Finance.CreditCard.CardHolder }),
	strCol("finance_bank_code", "finance.bank_account.bank_code", ModuleFinance, func(p *Persona) *string { return &p.Finance.BankAccount.BankCode }),
	strCol("finance_bank_name", "finance.bank_account.bank_name", ModuleFinance, func(p *Persona) *string { return &p.Finance.BankAccount.BankName }),
	strCol("finance_branch_code", "finance.bank_account.branch_code", ModuleFinance, func(p *Persona) *string { return &p.Finance.BankAccount.BranchCode }),
	strCol("finance_branch_name", "finance.bank_account.branch_name", ModuleFinance, func(p *Persona) *string { return &p.Finance.BankAccount.BranchName }),
	strCol("finance_account_type", "finance.bank_account.account_type", ModuleFinance, func(p *Persona) *string { return &p.Finance.BankAccount.AccountType }),
	strCol("finance_account_number", "finance.bank_account.account_number", ModuleFinance, func(p *Persona) *string { return &p.Finance.BankAccount.AccountNumber }),
	strCol("finance_account_holder_kana", "finance.bank_account.account_holder_kana", ModuleFinance, func(p *Persona) *string { return &p.Finance.BankAccount.AccountHolderKana }),
	strCol("finance_my_number", "finance.my_number", ModuleFinance, func(p *Persona) *string { return &p.Finance.MyNumber }),
	{Name: "kyc_drivers_license_number", Key: "kyc.drivers_license_number", Module: ModuleKYC, Kind: KindNullableString,
		get: func(p *Persona) any { return p.KYC.DriversLicenseNumber },
		set: func(p *Persona, v string) error {
			if v == "" || v == "null" {
				p.KYC.DriversLicenseNumber = nil
			} else {
				p.KYC.DriversLicenseNumber = &v
			}
			return nil
		}},
	strCol("kyc_passport_number", "kyc.passport_number", ModuleKYC, func(p *Persona) *string { return &p.KYC.PassportNumber }),
	strCol("kyc_document_type", "kyc.document_type", ModuleKYC, func(p *Persona) *string { return &p.KYC.DocumentType }),
	strCol("profile_occupation", "profile.occupation", ModuleProfile, func(p *Persona) *string { return &p.Profile.Occupation }),
	strCol("profile_industry", "profile.industry", ModuleProfile, func(p *Persona) *string { return &p.Profile.Industry }),
	strCol("profile_company_name", "profile.company_name", ModuleProfile, func(p *Persona) *string { return &p.Profile.CompanyName }),
	intCol("profile_annual_income_man_yen", "profile.annual_income_man_yen", ModuleProfile, func(p *Persona) *int { return &p.Profile.AnnualIncomeManYen }),
	{Name: "profile_is_married", Key: "profile.is_married", Module: ModuleProfile, Kind: KindBool,
		get: func(p *Persona) any { return p.Profile.IsMarried },
		set: func(p *Persona, v string) error {
			b, err := strconv.ParseBool(v)
			if err != nil {
				return fmt.Errorf("boolean required: %q", v)
			}
			p.Profile.IsMarried = b
			return nil
		}},
	intCol("profile_dependents_count", "profile.dependents_count", ModuleProfile, func(p *Persona) *int { return &p.Profile.DependentsCount }),
}

var columnIndex = func() map[string]*Column {
	m := map[string]*Column{}
	for i := range allColumns {
		c := &allColumns[i]
		m[c.Name] = c
		m[c.Key] = c
	}
	return m
}()

// LookupColumn は --set キー（JSON パスまたはフラット列名）から列定義を返す。
func LookupColumn(key string) *Column { return columnIndex[key] }

// Columns は有効なモジュールの列を仕様順で返す。
func Columns(withFinance, withKYC, withProfile bool) []*Column {
	var out []*Column
	for i := range allColumns {
		c := &allColumns[i]
		switch c.Module {
		case ModuleFinance:
			if !withFinance {
				continue
			}
		case ModuleKYC:
			if !withKYC {
				continue
			}
		case ModuleProfile:
			if !withProfile {
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

// Columns は Generator の設定に対応する列を返す。
func (g *Generator) Columns() []*Column {
	return Columns(g.opt.WithFinance, g.opt.WithKYC, g.opt.WithProfile)
}

func newEmptyPersona() *Persona {
	return &Persona{Finance: &Finance{}, KYC: &KYC{}, Profile: &Profile{}}
}
