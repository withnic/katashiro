package model

import "github.com/withnic/katashiro/internal/engine"

// 職業（profile.occupation）。申込フォームでよく使われる区分。
const (
	occEmployee   = "会社員"
	occExecutive  = "会社役員"
	occContract   = "契約社員"
	occDispatched = "派遣社員"
	occCivil      = "公務員"
	occSelf       = "自営業"
	occPartTime   = "パート・アルバイト"
	occStudent    = "学生"
	occHomemaker  = "主婦・主夫"
	occPension    = "年金受給"
	occUnemployed = "無職"
)

// Occupations は職業の全区分。
var Occupations = []string{occEmployee, occExecutive, occContract, occDispatched, occCivil, occSelf,
	occPartTime, occStudent, occHomemaker, occPension, occUnemployed}

// hasEmployer は勤務先（業種・社名）を持つ職業か。
func hasEmployer(occ string) bool {
	switch occ {
	case occStudent, occHomemaker, occPension, occUnemployed:
		return false
	}
	return true
}

type weighted struct {
	value  string
	weight int
}

func occupationTable(age int) []weighted {
	switch {
	case age < 15:
		return []weighted{{occStudent, 100}}
	case age < 18:
		return []weighted{{occStudent, 90}, {occPartTime, 10}}
	case age <= 22:
		return []weighted{{occStudent, 55}, {occPartTime, 15}, {occEmployee, 15}, {occContract, 4}, {occDispatched, 3},
			{occCivil, 3}, {occUnemployed, 5}}
	case age < 30:
		return []weighted{{occEmployee, 50}, {occExecutive, 1}, {occContract, 7}, {occDispatched, 5}, {occCivil, 8},
			{occSelf, 4}, {occPartTime, 10}, {occStudent, 4}, {occHomemaker, 6}, {occUnemployed, 5}}
	case age < 65:
		return []weighted{{occEmployee, 45}, {occExecutive, 5}, {occContract, 6}, {occDispatched, 4}, {occCivil, 8},
			{occSelf, 9}, {occPartTime, 10}, {occHomemaker, 9}, {occUnemployed, 4}}
	default:
		return []weighted{{occPension, 55}, {occPartTime, 12}, {occSelf, 12}, {occEmployee, 5}, {occExecutive, 4},
			{occContract, 4}, {occHomemaker, 5}, {occUnemployed, 3}}
	}
}

func pick(table []weighted, r uint64) string {
	total := 0
	for _, w := range table {
		total += w.weight
	}
	x := int(r % uint64(total))
	for _, w := range table {
		if x < w.weight {
			return w.value
		}
		x -= w.weight
	}
	return table[len(table)-1].value
}

// incomeRange は職業と年齢に応じた年収の範囲（万円、10万円単位）。
// 若年層で高年収が出ないよう、年齢で上限を変える。
func incomeRange(occ string, age int) (lo, hi int) {
	switch occ {
	case occStudent, occHomemaker, occUnemployed:
		return 0, 120
	case occPension:
		return 60, 300
	case occPartTime:
		if age < 18 {
			return 10, 100
		}
		return 80, 250
	case occContract, occDispatched:
		if age <= 22 {
			return 180, 300
		}
		return 200, 450
	case occExecutive:
		if age < 30 {
			return 300, 800
		}
		return 600, 2000
	default: // 会社員・公務員・自営業
		switch {
		case age <= 22:
			return 200, 400
		case age < 30:
			return 250, 600
		default:
			return 300, 1200
		}
	}
}

// 業種（profile.industry）は日本標準産業分類（令和5年7月改定）の大分類の名称。
const (
	indPublic = "公務（他に分類されるものを除く）"
	indEdu    = "教育、学習支援業"
	indMed    = "医療、福祉"
	indCoop   = "複合サービス事業"
)

// industryDef は大分類ごとの出現の重みと社名の末尾。
type industryDef struct {
	name     string
	weight   int      // 就業者数のおおよその比率に基づく重み
	suffixes []string // 社名の末尾
	noForm   bool     // 社名に会社の種類（株式会社など）を付けない
}

// Industries は公務を除く大分類（公務は公務員にだけ割り当てる）。
var Industries = []industryDef{
	{"農業、林業", 2, []string{"農園", "ファーム", "林業", "アグリ"}, false},
	{"漁業", 1, []string{"水産", "漁業", "マリン"}, false},
	{"鉱業、採石業、砂利採取業", 1, []string{"鉱業", "砕石", "砂利"}, false},
	{"建設業", 7, []string{"建設", "工務店", "組", "土木"}, false},
	{"製造業", 15, []string{"工業", "製作所", "精機", "化学", "電機"}, false},
	{"電気・ガス・熱供給・水道業", 1, []string{"電力", "ガス", "エナジー"}, false},
	{"情報通信業", 4, []string{"テック", "ソリューションズ", "システムズ", "ネットワークス", "通信"}, false},
	{"運輸業、郵便業", 5, []string{"運輸", "物流", "交通", "ロジスティクス"}, false},
	{"卸売業、小売業", 15, []string{"商事", "商店", "ストア", "物産"}, false},
	{"金融業、保険業", 2, []string{"ファイナンス", "証券", "信託", "損害保険"}, false},
	{"不動産業、物品賃貸業", 2, []string{"不動産", "地所", "リース", "レンタル"}, false},
	{"学術研究、専門・技術サービス業", 4, []string{"総合研究所", "設計", "コンサルティング", "デザイン"}, false},
	{"宿泊業、飲食サービス業", 5, []string{"ホテル", "ダイニング", "フーズ", "旅館"}, false},
	{"生活関連サービス業、娯楽業", 3, []string{"トラベル", "ビューティー", "クリーニング", "エンターテインメント"}, false},
	{indEdu, 5, []string{"学院", "アカデミー", "ゼミナール", "スクール"}, false},
	{indMed, 13, []string{"メディカル", "ヘルスケア", "ケアサービス", "ライフケア"}, false},
	{indCoop, 1, []string{"農業協同組合", "漁業協同組合", "生活協同組合"}, true},
	{"サービス業（他に分類されないもの）", 7, []string{"サービス", "警備保障", "ビルメンテナンス", "人材サービス"}, false},
	{"分類不能の産業", 1, []string{"商会", "ホールディングス"}, false},
}

// AllIndustryNames は日本標準産業分類の大分類 20 区分の名称（A〜T の順）。
var AllIndustryNames = []string{
	"農業、林業", "漁業", "鉱業、採石業、砂利採取業", "建設業", "製造業", "電気・ガス・熱供給・水道業",
	"情報通信業", "運輸業、郵便業", "卸売業、小売業", "金融業、保険業", "不動産業、物品賃貸業",
	"学術研究、専門・技術サービス業", "宿泊業、飲食サービス業", "生活関連サービス業、娯楽業",
	indEdu, indMed, indCoop, "サービス業（他に分類されないもの）", indPublic, "分類不能の産業",
}

// civilEmployers は公務員の勤務先と業種の組。公立学校・公立病院は日本標準産業分類では教育・医療に分類される。
var civilEmployers = []struct{ suffix, industry string }{
	{"市役所", indPublic}, {"町役場", indPublic}, {"県庁", indPublic},
	{"市立中学校", indEdu}, {"市民病院", indMed},
}

func pickIndustry(r uint64) industryDef {
	total := 0
	for _, d := range Industries {
		total += d.weight
	}
	x := int(r % uint64(total))
	for _, d := range Industries {
		if x < d.weight {
			return d
		}
		x -= d.weight
	}
	return Industries[len(Industries)-1]
}

func (g *Generator) profile(seqID uint32, age int) *Profile {
	occ := pick(occupationTable(age), g.rnd(seqID, engine.SaltOccupation))
	pr := &Profile{Occupation: occ}
	lo, hi := incomeRange(occ, age)
	pr.AnnualIncomeManYen = lo + int(g.rnd(seqID, engine.SaltIncome)%uint64((hi-lo)/10+1))*10
	if hasEmployer(occ) {
		ri := g.rnd(seqID, engine.SaltIndustry)
		rc := g.rnd(seqID, engine.SaltCompany)
		stem := companyStems[(rc>>8)%uint64(len(companyStems))]
		if occ == occCivil {
			c := civilEmployers[ri%uint64(len(civilEmployers))]
			pr.Industry = c.industry
			pr.CompanyName = stem + c.suffix
		} else {
			ind := pickIndustry(ri)
			pr.Industry = ind.name
			form := companyForms[rc%uint64(len(companyForms))]
			if occ == occExecutive {
				form = "株式会社"
			}
			if ind.noForm {
				form = ""
			}
			pr.CompanyName = form + stem + ind.suffixes[(rc>>16)%uint64(len(ind.suffixes))]
		}
	}

	// 婚姻: 18歳未満は必ず未婚・扶養0（2022年4月の民法改正で婚姻年齢は男女とも18歳）。
	if age < 18 {
		return pr
	}
	if g.married != nil {
		pr.IsMarried = *g.married
	} else {
		rate := uint64(2) // 18〜19歳
		switch {
		case age >= 65:
			rate = 70
		case age >= 40:
			rate = 65
		case age >= 30:
			rate = 55
		case age >= 20:
			rate = 25
		}
		pr.IsMarried = g.rnd(seqID, engine.SaltMarried)%100 < rate
	}
	rd := g.rnd(seqID, engine.SaltDependents)
	if pr.IsMarried {
		pr.DependentsCount = int(rd % 5)
	} else if rd%10 == 0 {
		pr.DependentsCount = 1
	}
	return pr
}
