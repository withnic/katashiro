package model

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/withnic/katashiro/internal/engine"
	"github.com/withnic/katashiro/internal/master"
)

func newGen(t *testing.T, mutate func(*Options)) *Generator {
	t.Helper()
	o := DefaultOptions()
	if mutate != nil {
		mutate(&o)
	}
	g, err := NewGenerator(o)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func date(s string) time.Time {
	d, err := ParseDate(s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestAgeAtCalendarComparison(t *testing.T) {
	base := date("2026-01-01")
	cases := []struct {
		birth string
		want  int
	}{
		{"2006-01-01", 20}, // 誕生日当日
		{"2006-01-02", 19}, // 誕生日前日
		{"2005-12-31", 20},
		{"2008-01-01", 18},
		{"2008-01-02", 17},
		{"2000-02-29", 25},
	}
	for _, c := range cases {
		if got := AgeAt(date(c.birth), base); got != c.want {
			t.Errorf("AgeAt(%s) = %d, want %d", c.birth, got, c.want)
		}
	}
	// うるう日生まれ: 平年の 2/28 時点ではまだ誕生日前
	if got := AgeAt(date("2004-02-29"), date("2023-02-28")); got != 18 {
		t.Errorf("leap birthday on 02-28 = %d, want 18", got)
	}
	if got := AgeAt(date("2004-02-29"), date("2023-03-01")); got != 19 {
		t.Errorf("leap birthday on 03-01 = %d, want 19", got)
	}
}

func TestBirthdayRangeExact(t *testing.T) {
	for _, base := range []string{"2026-01-01", "2028-02-29", "2027-03-01", "2026-12-31"} {
		b := date(base)
		for _, n := range []int{0, 17, 18, 20, 80} {
			start, days := BirthdayRange(b, n, n)
			if days < 365 || days > 366 {
				t.Fatalf("base=%s age=%d: days=%d", base, n, days)
			}
			last := start.AddDate(0, 0, days-1)
			if AgeAt(start, b) != n || AgeAt(last, b) != n {
				t.Fatalf("base=%s age=%d: endpoints not age %d", base, n, n)
			}
			if AgeAt(start.AddDate(0, 0, -1), b) == n || (last.Before(b) && AgeAt(last.AddDate(0, 0, 1), b) == n) {
				t.Fatalf("base=%s age=%d: range not maximal", base, n)
			}
		}
	}
}

func TestDefaultAgeRange18to80(t *testing.T) {
	g := newGen(t, nil)
	for i := uint32(0); i < 5000; i++ {
		p := g.At(i)
		if p.Personal.Age < 18 || p.Personal.Age > 80 {
			t.Fatalf("age %d out of range", p.Personal.Age)
		}
		if AgeAt(date(p.Personal.Birthday), date("2026-01-01")) != p.Personal.Age {
			t.Fatal("age/birthday mismatch")
		}
	}
}

// --set personal.age=20 で全員が満20歳になり、生年月日が365日内に分散する。
func TestOverrideAge20(t *testing.T) {
	g := newGen(t, func(o *Options) { o.Sets = []string{"personal.age=20"} })
	base := date("2026-01-01")
	days := map[string]bool{}
	var minB, maxB time.Time
	for i := uint32(0); i < 10000; i++ {
		p := g.At(i)
		b := date(p.Personal.Birthday)
		if p.Personal.Age != 20 || AgeAt(b, base) != 20 {
			t.Fatalf("age=%d birthday=%s", p.Personal.Age, p.Personal.Birthday)
		}
		days[p.Personal.Birthday] = true
		if minB.IsZero() || b.Before(minB) {
			minB = b
		}
		if b.After(maxB) {
			maxB = b
		}
	}
	if span := maxB.Sub(minB).Hours()/24 + 1; span > 366 {
		t.Fatalf("birthday span %v days exceeds a year", span)
	}
	if len(days) < 350 {
		t.Fatalf("birthdays should spread across the year, distinct=%d", len(days))
	}
	if minB != date("2005-01-02") || maxB != date("2006-01-01") {
		t.Fatalf("range = %s..%s", minB.Format(DateLayout), maxB.Format(DateLayout))
	}
}

// --set personal.age=17 --with-kyc --with-profile で免許証が null かつ未婚・扶養0。
func TestOverrideAge17Minor(t *testing.T) {
	g := newGen(t, func(o *Options) {
		o.Sets = []string{"personal.age=17"}
		o.WithKYC, o.WithProfile = true, true
	})
	for i := uint32(0); i < 5000; i++ {
		p := g.At(i)
		if p.Personal.Age != 17 {
			t.Fatalf("age=%d", p.Personal.Age)
		}
		if p.KYC.DriversLicenseNumber != nil {
			t.Fatalf("minor must not have a license: %s", *p.KYC.DriversLicenseNumber)
		}
		if p.Profile.IsMarried || p.Profile.DependentsCount != 0 {
			t.Fatalf("minor must be unmarried with 0 dependents: %+v", p.Profile)
		}
		if p.KYC.PassportNumber == "" {
			t.Fatal("passport must still be generated")
		}
	}
	b, _ := json.Marshal(g.At(0))
	if !strings.Contains(string(b), `"drivers_license_number":null`) {
		t.Fatalf("JSON must contain null license: %s", b)
	}
}

// 20歳未満（18・19歳）も未婚・扶養0。20歳では既婚者が出現しうる。
func TestUnder18UnmarriedAnd18Plus(t *testing.T) {
	g := newGen(t, func(o *Options) { o.Sets = []string{"personal.age=17"}; o.WithKYC, o.WithProfile = true, true })
	for i := uint32(0); i < 3000; i++ {
		if pr := g.At(i).Profile; pr.IsMarried || pr.DependentsCount != 0 {
			t.Fatal("age 17 must be unmarried with 0 dependents")
		}
	}
	// 18・19歳は婚姻可能（少数）、免許証あり
	g = newGen(t, func(o *Options) { o.MinAge, o.MaxAge = 18, 19; o.WithKYC, o.WithProfile = true, true })
	married := 0
	for i := uint32(0); i < 20000; i++ {
		p := g.At(i)
		if p.KYC.DriversLicenseNumber == nil {
			t.Fatal("18+ must have a license")
		}
		if p.Profile.IsMarried {
			married++
		}
	}
	if married == 0 || married > 1000 {
		t.Fatalf("18-19 married should be rare but present: %d/20000", married)
	}
}

func TestMarriedOverride(t *testing.T) {
	for _, v := range []bool{true, false} {
		g := newGen(t, func(o *Options) {
			o.WithProfile = true
			o.Sets = []string{"profile.is_married=" + strconv.FormatBool(v)}
		})
		deps := map[int]bool{}
		for i := uint32(0); i < 3000; i++ {
			pr := g.At(i).Profile
			if pr.IsMarried != v {
				t.Fatalf("is_married must be %v", v)
			}
			deps[pr.DependentsCount] = true
		}
		if v && len(deps) != 5 {
			t.Fatalf("married dependents should range 0..4: %v", deps)
		}
	}
	for _, mut := range []func(*Options){
		func(o *Options) {
			o.WithProfile = true
			o.Sets = []string{"profile.is_married=true", "personal.age=17"}
		},
		func(o *Options) { o.WithProfile = true; o.MinAge = 0; o.Sets = []string{"profile.is_married=true"} },
		func(o *Options) { o.Sets = []string{"profile.is_married=true"} },
		func(o *Options) { o.WithProfile = true; o.Sets = []string{"profile.is_married=yes"} },
	} {
		o := DefaultOptions()
		mut(&o)
		if _, err := NewGenerator(o); err == nil {
			t.Errorf("%v must fail", o.Sets)
		}
	}
	// false は 17 歳でも整合している
	newGen(t, func(o *Options) {
		o.WithProfile = true
		o.Sets = []string{"profile.is_married=false", "personal.age=17"}
	})
}

// --set-raw は整合チェックなしで上書きし、異常系テスト用の矛盾したデータを作れる。
func TestSetRaw(t *testing.T) {
	g := newGen(t, func(o *Options) {
		o.WithKYC, o.WithProfile = true, true
		o.Sets = []string{"personal.age=17"}
		o.RawSets = []string{"profile.is_married=true", "kyc.document_type=drivers_license", "profile.occupation=医師",
			"profile.annual_income_man_yen=1500", "personal.age=99"}
	})
	p := g.At(0)
	if !p.Profile.IsMarried || p.KYC.DocumentType != DocDriversLicense || p.Profile.Occupation != "医師" ||
		p.Profile.AnnualIncomeManYen != 1500 || p.Personal.Age != 99 {
		t.Fatalf("%+v %+v %+v", p.Personal, p.KYC, p.Profile)
	}
	// raw は対象列だけを変え、他は整合した生成値のまま（生年月日は 17 歳のまま）
	if AgeAt(date(p.Personal.Birthday), date("2026-01-01")) != 17 || p.KYC.DriversLicenseNumber != nil {
		t.Fatal("raw must not recompute related fields")
	}
	for _, raw := range [][]string{{"unknown=1"}, {"finance.my_number=1"}, {"profile.is_married=maybe"}, {"noequals"}} {
		o := DefaultOptions()
		o.WithProfile = true
		o.RawSets = raw
		if _, err := NewGenerator(o); err == nil {
			t.Errorf("%v must fail", raw)
		}
	}
}

func TestIncomeByAge(t *testing.T) {
	g := newGen(t, func(o *Options) { o.WithProfile = true; o.MinAge, o.MaxAge = 18, 22 })
	for i := uint32(0); i < 5000; i++ {
		if pr := g.At(i).Profile; pr.AnnualIncomeManYen > 400 {
			t.Fatalf("18-22 must not earn over 400: %+v", pr)
		}
	}
}

func TestGenderOtherUnspecified(t *testing.T) {
	for _, gen := range []string{GenderOther, GenderUnspecified} {
		g := newGen(t, func(o *Options) { o.Sets = []string{"personal.gender=" + gen} })
		d, _ := master.Default()
		ms, me := d.Names.Group(master.GroupMale)
		maleNames := map[string]bool{}
		for i := ms; i < me; i++ {
			maleNames[d.Name(i).Kanji+d.Name(i).Hiragana] = true
		}
		male, female := 0, 0
		for i := uint32(0); i < 2000; i++ {
			p := g.At(i).Personal
			if p.Gender != gen {
				t.Fatalf("gender=%s", p.Gender)
			}
			if maleNames[p.FirstName+p.FirstNameHiragana] {
				male++
			} else {
				female++
			}
		}
		if male < 800 || female < 800 {
			t.Fatalf("%s should draw first names from both dictionaries: male=%d female=%d", gen, male, female)
		}
	}
	// 既定では male / female のみ
	g := newGen(t, nil)
	for i := uint32(0); i < 3000; i++ {
		if gd := g.At(i).Personal.Gender; gd != GenderMale && gd != GenderFemale {
			t.Fatalf("default gender %s", gd)
		}
	}
}

// --set contact.prefecture=福岡県 --with-kyc で住所が福岡県、免許証先頭2桁が 90。
func TestOverridePrefectureFukuoka(t *testing.T) {
	g := newGen(t, func(o *Options) { o.Sets = []string{"contact.prefecture=福岡県"}; o.WithKYC = true })
	for i := uint32(0); i < 5000; i++ {
		p := g.At(i)
		if p.Contact.Prefecture != "福岡県" {
			t.Fatalf("prefecture=%s", p.Contact.Prefecture)
		}
		zip := p.Contact.ZipCode
		if !(strings.HasPrefix(zip, "80") || strings.HasPrefix(zip, "81") || strings.HasPrefix(zip, "82") || strings.HasPrefix(zip, "83")) {
			t.Fatalf("zip %s is not a Fukuoka zip", zip)
		}
		lic := *p.KYC.DriversLicenseNumber
		if !strings.HasPrefix(lic, "90") || !engine.LicenseValid(lic) {
			t.Fatalf("license %s", lic)
		}
	}
}

func TestOverrideGenderSyncsFirstName(t *testing.T) {
	gm := newGen(t, func(o *Options) { o.Sets = []string{"personal.gender=female"} })
	female := map[string]bool{}
	for i := uint32(0); i < 3000; i++ {
		p := gm.At(i)
		if p.Personal.Gender != "female" {
			t.Fatal("gender not fixed")
		}
		female[p.Personal.FirstName] = true
	}
	// 既定生成で男性に割り当てられた名が女性固定時に出現しないこと
	g := newGen(t, nil)
	for i := uint32(0); i < 3000; i++ {
		p := g.At(i)
		if p.Personal.Gender == "male" && female[p.Personal.FirstName] {
			t.Fatalf("male first name %s used for female", p.Personal.FirstName)
		}
	}
}

func TestOverrideBirthday(t *testing.T) {
	g := newGen(t, func(o *Options) {
		o.Sets = []string{"personal.birthday=2006-01-01"}
		o.WithKYC, o.WithProfile = true, true
	})
	p := g.At(5)
	if p.Personal.Birthday != "2006-01-01" || p.Personal.Age != 20 {
		t.Fatalf("%+v", p.Personal)
	}
	g = newGen(t, func(o *Options) {
		o.Sets = []string{"personal.birthday=2006-01-02"}
		o.WithKYC, o.WithProfile = true, true
	})
	p = g.At(5)
	if p.Personal.Age != 19 || p.Profile.IsMarried {
		t.Fatalf("birthday-eve must be 19 and unmarried: %+v", p.Personal)
	}
	if _, err := NewGenerator(func() Options {
		o := DefaultOptions()
		o.Sets = []string{"personal.birthday=2006-01-01", "personal.age=30"}
		return o
	}()); err == nil {
		t.Fatal("contradicting age/birthday must fail")
	}
}

func TestSimpleOverride(t *testing.T) {
	g := newGen(t, func(o *Options) {
		o.Sets = []string{"personal.full_name=山田 太郎", "profile.is_married=true", "finance_bank_code=0005"}
		o.WithProfile, o.WithFinance = true, true
	})
	p := g.At(0)
	if p.Personal.FullName != "山田 太郎" || !p.Profile.IsMarried || p.Finance.BankAccount.BankCode != "0005" {
		t.Fatalf("%+v", p)
	}
	// 読みは逆変換しない（full_name 以外は元の生成値のまま）
	orig := newGen(t, func(o *Options) { o.WithProfile, o.WithFinance = true, true }).At(0)
	if p.Personal.FullNameHiragana != orig.Personal.FullNameHiragana || p.Personal.LastName != orig.Personal.LastName {
		t.Fatal("simple override must not touch other fields")
	}
	bad := [][]string{{"unknown.key=1"}, {"profile.is_married=maybe"}, {"finance.my_number=1"}, {"personal.gender=man"}, {"contact.prefecture=東京"}}
	for _, sets := range bad {
		o := DefaultOptions()
		o.Sets = sets
		o.WithProfile = true
		if _, err := NewGenerator(o); err == nil {
			t.Errorf("%v must fail", sets)
		}
	}
}

// 10,000 件で一意性必須項目が 1 件も衝突しないこと（Feistel 経由のカーソル 0..9999）。
func TestUniqueness10000(t *testing.T) {
	g := newGen(t, func(o *Options) { o.WithFinance, o.WithKYC, o.WithProfile = true, true, true })
	sets := map[string]map[string]bool{}
	add := func(kind, v string) {
		if sets[kind] == nil {
			sets[kind] = map[string]bool{}
		}
		if sets[kind][v] {
			t.Fatalf("duplicate %s: %s", kind, v)
		}
		sets[kind][v] = true
	}
	for i := uint32(0); i < 10000; i++ {
		p := g.At(i)
		add("id", strconv.FormatUint(p.ID, 10))
		add("email", p.Contact.Email)
		add("phone", p.Contact.PhoneNumber)
		add("phone7", strings.ReplaceAll(p.Contact.PhoneNumber, "-", "")[4:])
		add("account", p.Finance.BankAccount.AccountNumber)
		add("card", p.Finance.CreditCard.Number)
		add("mynumber", p.Finance.MyNumber)
		add("passport", p.KYC.PassportNumber)
		if p.KYC.DriversLicenseNumber != nil {
			add("license", (*p.KYC.DriversLicenseNumber)[4:10]) // 交付番号6桁単体で一意
		}
	}
}

func TestFieldFormats(t *testing.T) {
	g := newGen(t, func(o *Options) { o.WithFinance, o.WithKYC, o.WithProfile = true, true, true })
	amex, others := 0, 0
	for i := uint32(0); i < 4000; i++ {
		p := g.At(i)
		c := p.Finance.CreditCard
		if !engine.LuhnValid(c.Number) {
			t.Fatalf("Luhn invalid: %s", c.Number)
		}
		if c.Brand == "American Express" {
			amex++
			if len(c.Number) != 15 || len(c.CVV) != 4 || !strings.HasPrefix(c.Number, "378282") {
				t.Fatalf("amex %+v", c)
			}
		} else {
			others++
			if len(c.Number) != 16 || len(c.CVV) != 3 {
				t.Fatalf("card %+v", c)
			}
		}
		if !engine.MyNumberValid(p.Finance.MyNumber) || !strings.HasPrefix(p.Finance.MyNumber, "000") {
			t.Fatalf("my number %s", p.Finance.MyNumber)
		}
		if lic := p.KYC.DriversLicenseNumber; lic == nil || len(*lic) != 12 || !engine.LicenseValid(*lic) || (*lic)[11] != '0' {
			t.Fatalf("license %v", lic)
		}
		if pp := p.KYC.PassportNumber; len(pp) != 9 || pp[0] != 'T' {
			t.Fatalf("passport %s", pp)
		}
		ph := p.Contact.PhoneNumber
		if len(ph) != 13 || ph[3] != '-' || ph[4] != '0' || ph[8] != '-' {
			t.Fatalf("phone %s", ph)
		}
		if !strings.HasSuffix(p.Contact.Email, ".example.com") && !strings.Contains(p.Contact.Email, "@example.") {
			t.Fatalf("email %s", p.Contact.Email)
		}
		if p.Finance.BankAccount.AccountHolderKana != p.Personal.BankHolderKana {
			t.Fatal("account holder must equal bank_holder_kana")
		}
		pr := p.Profile
		if lo, hi := incomeRange(pr.Occupation, p.Personal.Age); pr.AnnualIncomeManYen < lo || pr.AnnualIncomeManYen > hi {
			t.Fatalf("income %d out of %d..%d: %+v", pr.AnnualIncomeManYen, lo, hi, pr)
		}
		if hasEmployer(pr.Occupation) != (pr.Industry != "" && pr.CompanyName != "") {
			t.Fatalf("employer fields mismatch: %+v", pr)
		}
		if pr.Industry != "" && !slices.Contains(AllIndustryNames, pr.Industry) {
			t.Fatalf("industry %q is not a JSIC major group", pr.Industry)
		}
		if pr.Industry == indPublic && pr.Occupation != occCivil {
			t.Fatalf("only civil servants work in 公務: %+v", pr)
		}
		if pr.CompanyName != "" && !companyMatchesIndustry(pr) {
			t.Fatalf("company %q does not match industry %q", pr.CompanyName, pr.Industry)
		}
		if pr.AnnualIncomeManYen%10 != 0 || pr.DependentsCount < 0 || pr.DependentsCount > 4 {
			t.Fatalf("profile %+v", pr)
		}
		if b := p.Contact.Building; b != "" && !strings.HasSuffix(b, "号室") {
			t.Fatalf("building %s", b)
		}
	}
	if amex == 0 || others == 0 {
		t.Fatal("all brands should appear")
	}
}

func companyMatchesIndustry(pr *Profile) bool {
	if pr.Occupation == occCivil {
		for _, c := range civilEmployers {
			if strings.HasSuffix(pr.CompanyName, c.suffix) && c.industry == pr.Industry {
				return true
			}
		}
		return false
	}
	for _, d := range Industries {
		if d.name != pr.Industry {
			continue
		}
		for _, suf := range d.suffixes {
			if strings.HasSuffix(pr.CompanyName, suf) {
				return true
			}
		}
	}
	return false
}

func TestDeterminism(t *testing.T) {
	a := newGen(t, func(o *Options) { o.WithFinance, o.WithKYC, o.WithProfile = true, true, true })
	b := newGen(t, func(o *Options) { o.WithFinance, o.WithKYC, o.WithProfile = true, true, true })
	c := newGen(t, func(o *Options) { o.Seed = 42; o.WithFinance, o.WithKYC, o.WithProfile = true, true, true })
	ja, _ := json.Marshal(a.At(123))
	jb, _ := json.Marshal(b.At(123))
	jc, _ := json.Marshal(c.At(123))
	if string(ja) != string(jb) {
		t.Fatal("same seed must be identical")
	}
	if string(ja) == string(jc) {
		t.Fatal("different seed should differ")
	}
}

func BenchmarkBuildFull(b *testing.B) {
	o := DefaultOptions()
	o.WithFinance, o.WithKYC, o.WithProfile = true, true, true
	g, _ := NewGenerator(o)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = g.At(uint32(i % engine.SpaceSize))
	}
}

func TestIndustriesMatchJSIC(t *testing.T) {
	if len(AllIndustryNames) != 20 {
		t.Fatalf("JSIC has 20 major groups, got %d", len(AllIndustryNames))
	}
	seen := map[string]bool{}
	for _, d := range Industries {
		if !slices.Contains(AllIndustryNames, d.name) || d.weight <= 0 || len(d.suffixes) == 0 || seen[d.name] {
			t.Fatalf("bad industry def %+v", d)
		}
		seen[d.name] = true
	}
	if len(Industries) != 19 || seen[indPublic] {
		t.Fatal("Industries must be all major groups except 公務")
	}
	// 10万件で全大分類が出現する（公務は公務員経由）
	g := newGen(t, func(o *Options) { o.WithProfile = true })
	got := map[string]bool{}
	for i := uint32(0); i < 100000; i++ {
		got[g.At(i).Profile.Industry] = true
	}
	for _, name := range AllIndustryNames {
		if !got[name] {
			t.Errorf("industry %q never generated", name)
		}
	}
}

func TestDocumentType(t *testing.T) {
	// 既定: 両方の書類が出現し、提示書類が免許証なら免許証番号がある
	g := newGen(t, func(o *Options) { o.WithKYC = true })
	counts := map[string]int{}
	for i := uint32(0); i < 3000; i++ {
		k := g.At(i).KYC
		counts[k.DocumentType]++
		if k.DocumentType == DocDriversLicense && k.DriversLicenseNumber == nil {
			t.Fatal("drivers_license document without a license number")
		}
	}
	if counts[DocDriversLicense] == 0 || counts[DocPassport] == 0 || len(counts) != 2 {
		t.Fatalf("counts=%v", counts)
	}
	// 未成年は必ずパスポート
	g = newGen(t, func(o *Options) { o.WithKYC = true; o.MinAge, o.MaxAge = 0, 17 })
	for i := uint32(0); i < 2000; i++ {
		if g.At(i).KYC.DocumentType != DocPassport {
			t.Fatal("minors must present a passport")
		}
	}
	// 固定
	for _, doc := range []string{DocDriversLicense, DocPassport} {
		g = newGen(t, func(o *Options) { o.WithKYC = true; o.Sets = []string{"kyc.document_type=" + doc} })
		for i := uint32(0); i < 2000; i++ {
			if g.At(i).KYC.DocumentType != doc {
				t.Fatalf("document_type must be fixed to %s", doc)
			}
		}
	}
	bad := []Options{}
	for _, mut := range []func(*Options){
		func(o *Options) {
			o.WithKYC = true
			o.Sets = []string{"kyc.document_type=drivers_license", "personal.age=17"}
		},
		func(o *Options) {
			o.WithKYC = true
			o.MinAge = 16
			o.Sets = []string{"kyc.document_type=drivers_license"}
		},
		func(o *Options) { o.WithKYC = true; o.Sets = []string{"kyc.document_type=residence_card"} },
		func(o *Options) { o.Sets = []string{"kyc.document_type=passport"} },
	} {
		o := DefaultOptions()
		mut(&o)
		bad = append(bad, o)
	}
	for i, o := range bad {
		if _, err := NewGenerator(o); err == nil {
			t.Errorf("case %d must fail", i)
		}
	}
}

// 同姓同名: 姓・名を固定すると読み・カナ・全銀協カナ・ローマ字・性別まで揃い、メールは一意のまま。
func TestSameNameOverride(t *testing.T) {
	g := newGen(t, func(o *Options) {
		o.WithFinance = true
		o.Sets = []string{"personal.last_name=佐藤", "personal.last_name_hiragana=さとう",
			"personal.first_name=太郎", "personal.first_name_katakana=タロウ"}
	})
	emails := map[string]bool{}
	genders := map[string]bool{}
	for i := uint32(0); i < 2000; i++ {
		p := g.At(i)
		pp := p.Personal
		if pp.FullName != "佐藤　太郎" || pp.FullNameHiragana != "さとう　たろう" || pp.FullNameKatakana != "サトウ　タロウ" ||
			pp.BankHolderKana != "ｻﾄｳ ﾀﾛｳ" || pp.RomajiName != "taro.sato" || p.Finance.CreditCard.CardHolder != "TARO SATO" {
			t.Fatalf("%+v", pp)
		}
		if emails[p.Contact.Email] {
			t.Fatalf("duplicate email %s", p.Contact.Email)
		}
		emails[p.Contact.Email] = true
		genders[pp.Gender] = true
	}
	// 名前から性別を決めない（性別は通常どおり決まる）
	if !genders[GenderMale] || !genders[GenderFemale] {
		t.Fatalf("gender must not be inferred from the name: %v", genders)
	}
}

func TestNameOverrideReadings(t *testing.T) {
	// 読みを指定すれば辞書にない名前も使える
	g := newGen(t, func(o *Options) {
		o.Sets = []string{"personal.last_name=形代", "personal.last_name_katakana=カタシロ",
			"personal.first_name=一太", "personal.first_name_hiragana=いった", "personal.gender=male"}
	})
	p := g.At(0).Personal
	if p.FullName != "形代　一太" || p.FullNameHiragana != "かたしろ　いった" || p.FullNameKatakana != "カタシロ　イッタ" ||
		p.BankHolderKana != "ｶﾀｼﾛ ｲﾂﾀ" || p.RomajiName != "itta.katashiro" || p.Gender != "male" {
		t.Fatalf("%+v", p)
	}
	// 同字異音も読みで固定できる
	g = newGen(t, func(o *Options) {
		o.Sets = []string{"personal.first_name=陽菜", "personal.first_name_hiragana=はるな"}
	})
	for i := uint32(0); i < 200; i++ {
		if p := g.At(i).Personal; p.FirstNameHiragana != "はるな" || p.FirstNameKatakana != "ハルナ" {
			t.Fatalf("%+v", p)
		}
	}
	// 読みだけの指定は従来どおり単純上書き
	g = newGen(t, func(o *Options) {
		o.Sets = []string{"personal.last_name_hiragana=てすと", "personal.first_name_katakana=テスト"}
	})
	if p := g.At(0).Personal; p.LastNameHiragana != "てすと" || p.FirstNameKatakana != "テスト" {
		t.Fatalf("%+v", p)
	}
	for _, sets := range [][]string{
		{"personal.last_name=佐藤"}, // 辞書にあっても読みは必須
		{"personal.first_name=太郎"},
		{"personal.last_name=佐藤", "personal.last_name_hiragana=さとう", "personal.last_name_katakana=サイトウ"}, // 不一致
		{"personal.last_name=形代", "personal.last_name_hiragana=Katashiro"},                               // ひらがなでない
		{"personal.gender=man"},
	} {
		o := DefaultOptions()
		o.Sets = sets
		if _, err := NewGenerator(o); err == nil {
			t.Errorf("%v must fail", sets)
		}
	}
}

func TestChildrenAreStudents(t *testing.T) {
	g := newGen(t, func(o *Options) { o.WithProfile = true; o.MinAge, o.MaxAge = 0, 14 })
	for i := uint32(0); i < 2000; i++ {
		if pr := g.At(i).Profile; pr.Occupation != occStudent {
			t.Fatalf("age %d occupation %s", g.At(i).Personal.Age, pr.Occupation)
		}
	}
}
