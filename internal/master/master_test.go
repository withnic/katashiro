package master

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestToZengin(t *testing.T) {
	cases := map[string]string{
		"サトウ　タロウ":   "ｻﾄｳ ﾀﾛｳ",
		"がっこう":      "ｶﾞﾂｺｳ",
		"キョウコ":      "ｷﾖｳｺ",
		"ジュンイチ":     "ｼﾞﾕﾝｲﾁ",
		"パピプペポ":     "ﾊﾟﾋﾟﾌﾟﾍﾟﾎﾟ",
		"ヴァイオリン":    "ｳﾞｱｲｵﾘﾝ",
		"ｧｨｩｪｫｯｬｭｮ": "ｱｲｳｴｵﾂﾔﾕﾖ",
		"ルーシー":      "ﾙｰｼｰ",
		"abc１２":     "ABC12",
	}
	for in, want := range cases {
		if got := ToZengin(in); got != want {
			t.Errorf("ToZengin(%q) = %q, want %q", in, got, want)
		}
	}
	long := strings.Repeat("ガ", 20) // 40 文字相当（濁点分離）
	got := ToZengin(long)
	if n := utf8.RuneCountInString(got); n != ZenginMaxLen {
		t.Fatalf("zengin must be truncated to %d chars, got %d", ZenginMaxLen, n)
	}
	for _, small := range "ｧｨｩｪｫｯｬｭｮ" {
		if strings.ContainsRune(ToZengin("きゃっしゅぁぃぅぇぉょ"), small) {
			t.Fatalf("small kana %c must be capitalized", small)
		}
	}
}

func TestToHepburn(t *testing.T) {
	cases := map[string]string{
		"さとう": "sato", "たろう": "taro", "いのうえ": "inoue", "おおの": "ono",
		"ゆういち": "yuichi", "しゅんすけ": "shunsuke", "はっとり": "hattori",
		"まっちゃ": "matcha", "きんじょう": "kinjo", "ふじ": "fuji", "ちづる": "chizuru",
		"けんいち": "kenichi", "じゅん": "jun", "つよし": "tsuyoshi", "ケイスケ": "keisuke",
	}
	for in, want := range cases {
		if got := ToHepburn(in); got != want {
			t.Errorf("ToHepburn(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEmbeddedDictionary(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if d.Addresses.Len() < 300 {
		t.Fatalf("expected hundreds of addresses, got %d", d.Addresses.Len())
	}
	// 47都道府県すべてで都道府県別オフセットによる絞り込みが機能すること
	for _, p := range Prefectures {
		s, e := d.PrefectureRange(p.Code)
		if s >= e {
			t.Fatalf("%s: empty range", p.Name)
		}
		for i := s; i < e; i++ {
			a := d.Address(i)
			if a.Prefecture != p.Name {
				t.Fatalf("%s: record in wrong group %+v", p.Name, a)
			}
			if issues := AddressIssues(a); len(issues) > 0 {
				t.Fatalf("%+v: %v", a, issues)
			}
		}
	}
	if p, _ := PrefectureByName("福岡県"); p.PSCCode != "90" || p.Code != 40 {
		t.Fatalf("福岡県 = %+v", p)
	}
	if p, _ := PrefectureByName("北海道"); p.PSCCode != "10" {
		t.Fatal("北海道 must be 10")
	}
	if p, _ := PrefectureByName("沖縄県"); p.PSCCode != "97" {
		t.Fatal("沖縄県 must be 97")
	}
	for g := 0; g < NameGroups; g++ {
		s, e := d.Names.Group(g)
		for i := s; i < e; i++ {
			n := d.Name(i)
			if n.Kanji == "" {
				t.Fatalf("empty kanji %+v", n)
			}
			if issues := NameIssues(n); len(issues) > 0 {
				t.Fatalf("%+v: %v", n, issues)
			}
		}
	}
}

func TestZeroAllocLookup(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	n := d.Addresses.Len()
	allocs := testing.AllocsPerRun(1000, func() {
		_ = d.Address(n / 2)
		_ = d.Name(3)
	})
	if allocs != 0 {
		t.Fatalf("lookup allocated %v times", allocs)
	}
}

func TestBuildTableRoundTrip(t *testing.T) {
	b, err := BuildTable(2, [][][]string{{{"a", ""}, {"bc", "def"}}, {{"日本", "語"}}})
	if err != nil {
		t.Fatal(err)
	}
	tb, err := OpenTable(b)
	if err != nil {
		t.Fatal(err)
	}
	if tb.Len() != 3 || tb.Field(0, 0) != "a" || tb.Field(0, 1) != "" || tb.Field(1, 1) != "def" || tb.Field(2, 0) != "日本" {
		t.Fatal("round trip mismatch")
	}
	if s, e := tb.Group(1); s != 2 || e != 3 {
		t.Fatalf("group 1 = [%d,%d)", s, e)
	}
	if _, err := OpenTable(b[:20]); err == nil {
		t.Fatal("truncated table must fail")
	}
}

func BenchmarkDefaultOpen(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := NewDict(addressesBin, namesBin); err != nil {
			b.Fatal(err)
		}
	}
}
