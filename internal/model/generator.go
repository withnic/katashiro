package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/withnic/katashiro/internal/engine"
	"github.com/withnic/katashiro/internal/master"
)

// DefaultBaseDate は --base-date の既定値。
const DefaultBaseDate = "2026-01-01"

// Options は生成パイプラインの設定。
type Options struct {
	Seed        int64
	BaseDate    time.Time // 日付部のみ使用（UTC）
	MinAge      int       // 既定 18
	MaxAge      int       // 既定 80
	WithFinance bool
	WithKYC     bool
	WithProfile bool
	StrictPhone bool
	Sets        []string     // "key=value" 形式の --set（整合を取る）
	RawSets     []string     // "key=value" 形式の --set-raw（整合チェックなしで生成後に上書き。異常系テスト用）
	Dict        *master.Dict // nil なら埋め込み辞書
}

// DefaultOptions は仕様上の既定値を持つ Options を返す。
func DefaultOptions() Options {
	base, _ := ParseDate(DefaultBaseDate)
	return Options{Seed: 1, BaseDate: base, MinAge: 18, MaxAge: 80}
}

// Generator は Options に基づき Persona を決定論的に合成する。並行利用して安全。
type Generator struct {
	opt     Options
	dict    *master.Dict
	feistel *engine.Feistel

	// 上流連動オーバーライド
	birthStart time.Time
	birthDays  int
	fixedBirth bool
	gender     string // "" なら seqID から決定
	addrStart  int
	addrCount  int
	lastName   *master.Name // --set personal.last_name（nil なら辞書から選ぶ）
	firstName  *master.Name // --set personal.first_name（nil なら辞書から選ぶ）
	docType    string       // --set kyc.document_type（"" なら seqID から決定）
	married    *bool        // --set profile.is_married（nil なら年齢から決定）

	post    []postOverride // 単純上書き
	rawPost []postOverride // --set-raw（最後に適用）
}

type postOverride struct {
	col   *Column
	value string
}

// NewGenerator は Options を検証して Generator を作る。
func NewGenerator(opt Options) (*Generator, error) {
	if opt.BaseDate.IsZero() {
		return nil, errors.New("base date is required")
	}
	opt.BaseDate = time.Date(opt.BaseDate.Year(), opt.BaseDate.Month(), opt.BaseDate.Day(), 0, 0, 0, 0, time.UTC)
	if opt.MinAge < 0 || opt.MaxAge < opt.MinAge || opt.MaxAge > 150 {
		return nil, fmt.Errorf("invalid age range: %d..%d", opt.MinAge, opt.MaxAge)
	}
	g := &Generator{opt: opt, dict: opt.Dict, feistel: engine.NewFeistel(opt.Seed)}
	if g.dict == nil {
		d, err := master.Default()
		if err != nil {
			return nil, err
		}
		g.dict = d
	}
	g.addrStart, g.addrCount = 0, g.dict.Addresses.Len()

	var ageSet, birthSet string
	var last, first nameSet
	// 読みの指定。漢字と組なら上流で使い、組でなければ単純上書きにする。
	type readingSet struct {
		isLast     bool
		key, value string
	}
	var readings []readingSet
	for _, kv := range opt.Sets {
		key, value, err := parseKV("--set", kv)
		if err != nil {
			return nil, err
		}
		switch key {
		case "personal.age", "personal_age":
			ageSet = value
		case "personal.birthday", "personal_birthday":
			birthSet = value
		case "personal.gender", "personal_gender":
			switch value {
			case GenderMale, GenderFemale, GenderOther, GenderUnspecified:
			default:
				return nil, fmt.Errorf("personal.gender must be male, female, other or unspecified: %q", value)
			}
			g.gender = value
		case "contact.prefecture", "contact_prefecture":
			p, ok := master.PrefectureByName(value)
			if !ok {
				return nil, fmt.Errorf("unknown prefecture: %q", value)
			}
			s, e := g.dict.PrefectureRange(p.Code)
			if s == e {
				return nil, fmt.Errorf("no address records for %s", value)
			}
			g.addrStart, g.addrCount = s, e-s
		case "personal.last_name", "personal_last_name":
			last.kanji = value
		case "personal.first_name", "personal_first_name":
			first.kanji = value
		case "personal.last_name_hiragana", "personal_last_name_hiragana":
			last.hiragana = value
			readings = append(readings, readingSet{true, key, value})
		case "personal.last_name_katakana", "personal_last_name_katakana":
			last.katakana = value
			readings = append(readings, readingSet{true, key, value})
		case "personal.first_name_hiragana", "personal_first_name_hiragana":
			first.hiragana = value
			readings = append(readings, readingSet{false, key, value})
		case "personal.first_name_katakana", "personal_first_name_katakana":
			first.katakana = value
			readings = append(readings, readingSet{false, key, value})
		case "profile.is_married", "profile_is_married":
			if !opt.WithProfile {
				return nil, fmt.Errorf("--set %s requires --with-profile", key)
			}
			b, err := strconv.ParseBool(value)
			if err != nil {
				return nil, fmt.Errorf("profile.is_married must be true or false: %q", value)
			}
			g.married = &b
		case "kyc.document_type", "kyc_document_type":
			if !opt.WithKYC {
				return nil, fmt.Errorf("--set %s requires --with-kyc", key)
			}
			if value != DocDriversLicense && value != DocPassport {
				return nil, fmt.Errorf("kyc.document_type must be %s or %s: %q", DocDriversLicense, DocPassport, value)
			}
			g.docType = value
		default:
			o, err := g.columnOverride("--set", key, value)
			if err != nil {
				return nil, err
			}
			g.post = append(g.post, o)
		}
	}

	switch {
	case birthSet != "":
		b, err := ParseDate(birthSet)
		if err != nil {
			return nil, fmt.Errorf("personal.birthday: %w", err)
		}
		if b.After(opt.BaseDate) {
			return nil, fmt.Errorf("personal.birthday %s is after base date", birthSet)
		}
		if ageSet != "" {
			if n, err := strconv.Atoi(ageSet); err != nil || n != AgeAt(b, opt.BaseDate) {
				return nil, fmt.Errorf("personal.age=%s contradicts personal.birthday=%s", ageSet, birthSet)
			}
		}
		g.birthStart, g.birthDays, g.fixedBirth = b, 1, true
	case ageSet != "":
		n, err := strconv.Atoi(ageSet)
		if err != nil || n < 0 || n > 150 {
			return nil, fmt.Errorf("personal.age must be an integer in 0..150: %q", ageSet)
		}
		g.birthStart, g.birthDays = BirthdayRange(opt.BaseDate, n, n)
	default:
		g.birthStart, g.birthDays = BirthdayRange(opt.BaseDate, opt.MinAge, opt.MaxAge)
	}
	minAge := AgeAt(g.birthStart.AddDate(0, 0, g.birthDays-1), opt.BaseDate)
	if g.docType == DocDriversLicense && minAge < 18 {
		return nil, fmt.Errorf("kyc.document_type=%s には 18 歳以上が必要です（最小年齢 %d）", DocDriversLicense, minAge)
	}
	if g.married != nil && *g.married && minAge < 18 {
		return nil, fmt.Errorf("profile.is_married=true には 18 歳以上が必要です（最小年齢 %d）", minAge)
	}

	// 姓・名: 漢字を指定したら読みも必須。読みからカナ・全銀協カナ・ローマ字を作る（性別は名前で決めない）。
	// 漢字なしで読みだけを指定した場合は、その列の単純上書き。
	for _, ns := range []struct {
		set   nameSet
		label string
		dst   **master.Name
	}{{last, "personal.last_name", &g.lastName}, {first, "personal.first_name", &g.firstName}} {
		if ns.set.kanji == "" {
			continue
		}
		n, err := ns.set.resolve(ns.label)
		if err != nil {
			return nil, err
		}
		*ns.dst = &n
	}
	for _, r := range readings {
		if (r.isLast && last.kanji != "") || (!r.isLast && first.kanji != "") {
			continue // 漢字と組で上流で使った
		}
		g.post = append(g.post, postOverride{col: LookupColumn(r.key), value: r.value})
	}

	for _, kv := range opt.RawSets {
		key, value, err := parseKV("--set-raw", kv)
		if err != nil {
			return nil, err
		}
		o, err := g.columnOverride("--set-raw", key, value)
		if err != nil {
			return nil, err
		}
		g.rawPost = append(g.rawPost, o)
	}
	return g, nil
}

// parseKV は "key=value" を分割する。flag はエラーメッセージ用（--set / --set-raw）。
func parseKV(flag, kv string) (key, value string, err error) {
	key, value, ok := strings.Cut(kv, "=")
	key = strings.TrimSpace(key)
	if !ok || key == "" {
		return "", "", fmt.Errorf("%s must be key=value: %q", flag, kv)
	}
	return key, value, nil
}

// columnOverride は生成後に列を上書きする指定を検証して返す（キーの存在・モジュール有効・値の型）。
func (g *Generator) columnOverride(flag, key, value string) (postOverride, error) {
	col := LookupColumn(key)
	if col == nil {
		return postOverride{}, fmt.Errorf("unknown %s key: %q", flag, key)
	}
	if !g.moduleEnabled(col.Module) {
		return postOverride{}, fmt.Errorf("%s %s requires --with-%s", flag, key, col.Module)
	}
	if err := col.set(newEmptyPersona(), value); err != nil {
		return postOverride{}, fmt.Errorf("%s %s: %w", flag, key, err)
	}
	return postOverride{col: col, value: value}, nil
}

// 性別（personal.gender）。既定の生成では male / female のみを出し、
// other / unspecified は --set personal.gender で指定したときだけ出す。
const (
	GenderMale        = "male"
	GenderFemale      = "female"
	GenderOther       = "other"
	GenderUnspecified = "unspecified"
)

// Options は正規化済みの設定を返す。
func (g *Generator) Options() Options { return g.opt }

func (g *Generator) moduleEnabled(m Module) bool {
	switch m {
	case ModuleFinance:
		return g.opt.WithFinance
	case ModuleKYC:
		return g.opt.WithKYC
	case ModuleProfile:
		return g.opt.WithProfile
	}
	return true
}

// SeqID はカーソル index ∈ [0, 1,000,000) を Feistel 置換で seqID に写す。
func (g *Generator) SeqID(index uint32) uint32 { return g.feistel.Permute(index) }

// At はカーソル index の Persona を返す。
func (g *Generator) At(index uint32) *Persona { return g.Build(g.SeqID(index)) }

func (g *Generator) rnd(seqID uint32, salt uint64) uint64 {
	return engine.AttrRand(g.opt.Seed, seqID, salt)
}

// Build は seqID ∈ [0, 1,000,000) から Persona を合成する。
func (g *Generator) Build(seqID uint32) *Persona {
	p := &Persona{ID: uint64(seqID) + 1}
	base := g.opt.BaseDate

	// --- 上流: 生年月日・年齢・性別 ---
	birth := g.birthStart
	if !g.fixedBirth {
		birth = birth.AddDate(0, 0, int(g.rnd(seqID, engine.SaltBirthday)%uint64(g.birthDays)))
	}
	age := AgeAt(birth, base)
	rg := g.rnd(seqID, engine.SaltGender)
	gender := g.gender
	if gender == "" {
		gender = GenderMale
		if rg%2 == 1 {
			gender = GenderFemale
		}
	}

	// --- 氏名（姓名の --set があればそれを使い、なければ辞書から選ぶ） ---
	d := g.dict
	rl, rf := g.rnd(seqID, engine.SaltLastName), g.rnd(seqID, engine.SaltFirstName)
	var last, first master.Name
	if g.lastName != nil {
		last = *g.lastName
	} else {
		ls, le := d.Names.Group(master.GroupLastName)
		last = d.Name(ls + int(rl%uint64(le-ls)))
	}
	if g.firstName != nil {
		first = *g.firstName
	} else {
		// male / female は性別と名マスタを同期。other / unspecified は男女どちらの辞書からも選ぶ
		fg := master.GroupMale
		switch gender {
		case GenderFemale:
			fg = master.GroupFemale
		case GenderOther, GenderUnspecified:
			if (rg>>1)%2 == 1 {
				fg = master.GroupFemale
			}
		}
		fs, fe := d.Names.Group(fg)
		first = d.Name(fs + int(rf%uint64(fe-fs)))
	}

	p.Personal = Personal{
		Birthday:          birth.Format(DateLayout),
		Age:               age,
		Gender:            gender,
		LastName:          last.Kanji,
		FirstName:         first.Kanji,
		FullName:          last.Kanji + "　" + first.Kanji,
		LastNameHiragana:  last.Hiragana,
		FirstNameHiragana: first.Hiragana,
		FullNameHiragana:  last.Hiragana + "　" + first.Hiragana,
		LastNameKatakana:  last.Katakana,
		FirstNameKatakana: first.Katakana,
		FullNameKatakana:  last.Katakana + "　" + first.Katakana,
		BankHolderKana:    truncateRunes(last.Zengin+" "+first.Zengin, master.ZenginMaxLen),
		RomajiName:        first.Romaji + "." + last.Romaji,
	}

	// --- 住所（都道府県オーバーライド時はそのグループ内で抽選） ---
	addr := d.Address(g.addrStart + int(g.rnd(seqID, engine.SaltAddress)%uint64(g.addrCount)))
	rs := g.rnd(seqID, engine.SaltStreet)
	building := ""
	if rb := g.rnd(seqID, engine.SaltBuilding); rb%100 < 35 {
		building = buildingPrefixes[(rb>>8)%uint64(len(buildingPrefixes))] +
			buildingStems[(rb>>16)%uint64(len(buildingStems))] + " " +
			strconv.Itoa(int((rb>>24)%15+1)*100+int((rb>>32)%8+1)) + "号室"
	}
	p.Contact = Contact{
		ZipCode:       addr.Zip[:3] + "-" + addr.Zip[3:],
		Prefecture:    addr.Prefecture,
		City:          addr.City,
		Town:          addr.Town,
		StreetAddress: fmt.Sprintf("%d-%d-%d", rs%6+1, (rs>>8)%30+1, (rs>>16)%20+1),
		Building:      building,
		PhoneNumber:   g.phone(seqID),
		Email: p.Personal.RomajiName + "." + strconv.FormatUint(uint64(seqID), 10) + "@" +
			emailDomains[g.rnd(seqID, engine.SaltEmailDomain)%uint64(len(emailDomains))],
	}

	if g.opt.WithFinance {
		p.Finance = g.finance(seqID, p, first, last)
	}
	if g.opt.WithKYC {
		p.KYC = g.kyc(seqID, birth, age, addr.PSCCode)
	}
	if g.opt.WithProfile {
		p.Profile = g.profile(seqID, age)
	}

	// --- 単純上書き（最終ステップ） ---
	for _, o := range g.post {
		_ = o.col.set(p, o.value) // NewGenerator で検証済み
	}
	for _, o := range g.rawPost {
		_ = o.col.set(p, o.value) // 整合チェックなし（異常系テスト用）
	}
	return p
}

var phonePrefixes = [3]string{"090", "080", "070"}

// phone は seqID を 7 桁全単射（mod 10^7 のアフィン変換）で下 7 桁に埋め込む。
func (g *Generator) phone(seqID uint32) string {
	const mod = 10_000_000
	const mul = 3_141_593 // 10 と互いに素 → mod 10^7 で全単射
	add := engine.SplitMix64(uint64(g.opt.Seed)^engine.SaltPhone) % mod
	low := fmt.Sprintf("%07d", (uint64(seqID)*mul+add)%mod)
	lead := "0" // 総務省未割り当ての安全帯域
	if g.opt.StrictPhone {
		lead = "1"
	}
	return phonePrefixes[seqID%3] + "-" + lead + low[:3] + "-" + low[3:]
}

type cardBrand struct {
	name, bin      string
	derived, cvvLn int
}

var cardBrands = [4]cardBrand{
	{"Visa", "400000", 3, 3},
	{"Mastercard", "555555", 3, 3},
	{"JCB", "353011", 3, 3},
	{"American Express", "378282", 2, 4},
}

type bank struct{ code, name string }

var banks = [7]bank{
	{"0001", "みずほ銀行"}, {"0005", "三菱UFJ銀行"}, {"0009", "三井住友銀行"}, {"0010", "りそな銀行"},
	{"0033", "PayPay銀行"}, {"0036", "楽天銀行"}, {"9900", "ゆうちょ銀行"},
}

func (g *Generator) finance(seqID uint32, p *Persona, first, last master.Name) *Finance {
	b := cardBrands[g.rnd(seqID, engine.SaltCardBrand)%4]
	mid := g.rnd(seqID, engine.SaltCardMiddle)
	pow := uint64(1000)
	if b.derived == 2 {
		pow = 100
	}
	payload := fmt.Sprintf("%s%0*d%06d", b.bin, b.derived, mid%pow, seqID)
	number := payload + string(engine.LuhnCheckDigit(payload))
	re := g.rnd(seqID, engine.SaltCardExp)
	cvvMod := uint64(1000)
	if b.cvvLn == 4 {
		cvvMod = 10000
	}
	rb := g.rnd(seqID, engine.SaltBank)
	branch := g.rnd(seqID, engine.SaltBranch)%999 + 1
	myPayload := fmt.Sprintf("000%08d", seqID)
	return &Finance{
		CreditCard: CreditCard{
			Brand:          b.name,
			Number:         number,
			ExpirationDate: fmt.Sprintf("%02d/%02d", re%12+1, (g.opt.BaseDate.Year()+1+int((re>>8)%5))%100),
			CVV:            fmt.Sprintf("%0*d", b.cvvLn, g.rnd(seqID, engine.SaltCardCVV)%cvvMod),
			CardHolder:     strings.ToUpper(first.Romaji + " " + last.Romaji),
		},
		BankAccount: BankAccount{
			BankCode:          banks[rb%uint64(len(banks))].code,
			BankName:          banks[rb%uint64(len(banks))].name,
			BranchCode:        fmt.Sprintf("%03d", branch),
			BranchName:        branchNames[branch%uint64(len(branchNames))] + "支店",
			AccountType:       "1",
			AccountNumber:     fmt.Sprintf("%07d", 1_000_000+seqID),
			AccountHolderKana: p.Personal.BankHolderKana,
		},
		MyNumber: myPayload + string(engine.MyNumberCheckDigit(myPayload)),
	}
}

// 本人確認書類の種類（kyc.document_type）。
const (
	DocDriversLicense = "drivers_license"
	DocPassport       = "passport"
)

func (g *Generator) kyc(seqID uint32, birth time.Time, age int, psc string) *KYC {
	k := &KYC{
		PassportNumber: fmt.Sprintf("T%c%07d", 'A'+byte(g.rnd(seqID, engine.SaltPassport)%26), 1_000_000+seqID),
		DocumentType:   DocPassport,
	}
	if age < 18 {
		return k // 18歳未満は運転免許証なし（null）、書類はパスポート
	}
	switch {
	case g.docType != "":
		k.DocumentType = g.docType
	case g.rnd(seqID, engine.SaltKYCDocument)%100 < 80: // 免許証保有者の多くは免許証を提示する
		k.DocumentType = DocDriversLicense
	}
	from, to := birth.Year()+18, g.opt.BaseDate.Year()
	year := from + int(g.rnd(seqID, engine.SaltLicenseYear)%uint64(to-from+1))
	payload := fmt.Sprintf("%s%02d%06d", psc, year%100, seqID)
	lic := payload + string(engine.LicenseCheckDigit(payload)) + "0"
	k.DriversLicenseNumber = &lic
	return k
}

func truncateRunes(s string, n int) string {
	i := 0
	for pos := range s {
		if i == n {
			return s[:pos]
		}
		i++
	}
	return s
}

var (
	emailDomains     = [3]string{"example.com", "example.jp", "example.net"}
	buildingPrefixes = []string{"メゾン", "ハイツ", "コーポ", "レジデンス", "パークハウス", "グランメール", "シャトー", "ヴィラ"}
	buildingStems    = []string{"サクラ", "ひまわり", "けやき", "青葉", "若葉", "富士見", "朝日", "白樺", "みどり", "コスモス", "銀河", "清風"}
	branchNames      = []string{"サクラ", "ツバキ", "カエデ", "イチョウ", "ケヤキ", "スズラン", "アヤメ", "キキョウ", "ボタン", "モミジ", "ヒノキ", "スミレ"}
	companyForms     = []string{"株式会社", "合同会社", "有限会社"}
	companyStems     = []string{"形代", "ミナト", "サンプル", "アオバ", "ヒカリ", "ミライ", "ツバサ", "ホシノ", "カザミ", "ナギサ"}
)
