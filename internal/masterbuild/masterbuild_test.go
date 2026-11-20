package masterbuild

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/withnic/katashiro/internal/master"
)

func TestCleanseTown(t *testing.T) {
	cases := []struct {
		in   string
		want string
		act  cleanseAction
	}{
		{"以下に掲載がない場合", "", actNoListing},
		{"中央（１丁目）", "中央", actStripParen},
		{"大通西（１～１９丁目）", "大通西", actStripParen},
		{"藤野（４００、４００－２番地）", "藤野", actStripParen},
		{"大江（１丁目、２丁目「６５１、６６２、６６８番地」以外、３丁目５、１３－４、２０、６７", "", actFragment},
		{"６７８番地）", "", actFragment},
		{"猿払村の次に番地がくる場合", "", actNextBanchi},
		{"小菅村の次に１〜６６３番地がくる場合", "", actNextBanchi},
		{"奥尻町一円", "", actIchien},
		{"越中畑６４地割〜越中畑６６地割", "", actRange},
		{"上野々３９地割", "上野々３９地割", actKeep},
		{"琴平町の次に４２７番地以降がくる場合（川東）", "", actNextBanchi},
		{"丸の内", "丸の内", actKeep},
	}
	for _, c := range cases {
		got, act := cleanseTown(c.in)
		if got != c.want || act != c.act {
			t.Errorf("cleanseTown(%q) = %q,%s want %q,%s", c.in, got, act, c.want, c.act)
		}
	}
}

func TestParseKenAllCSV(t *testing.T) {
	csv := `01101,"060  ","0600000","ﾎｯｶｲﾄﾞｳ","ｻｯﾎﾟﾛｼﾁｭｳｵｳｸ","ｲｶﾆｹｲｻｲｶﾞﾅｲﾊﾞｱｲ","北海道","札幌市中央区","以下に掲載がない場合",0,0,0,0,0,0
01101,"064  ","0640941","ﾎｯｶｲﾄﾞｳ","ｻｯﾎﾟﾛｼﾁｭｳｵｳｸ","ｱｻﾋｶﾞｵｶ","北海道","札幌市中央区","旭ケ丘",0,0,1,0,0,0
40133,"810  ","8100001","ﾌｸｵｶｹﾝ","ﾌｸｵｶｼﾁｭｳｵｳｸ","ﾃﾝｼﾞﾝ","福岡県","福岡市中央区","天神（１丁目）",0,0,1,0,0,0
01224,"066  ","0660005","ﾎｯｶｲﾄﾞｳ","ﾁﾄｾｼ","ｱｽﾞﾏﾁｮｳ","北海道","千歳市","大江（１丁目、２丁目「６５１、６６２",0,0,1,0,0,0
`
	got, st, err := parseKenAll([]byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[2].Town != "天神" || got[0].Town != "" {
		t.Fatalf("unexpected: %+v", got)
	}
	if st.Rows != 4 || st.Actions[actFragment] != 1 || st.Actions[actStripParen] != 1 || st.Actions[actNoListing] != 1 {
		t.Fatalf("stats: %+v", st)
	}
}

// TestKenAllRealData は本物の utf_ken_all.zip でクレンジング漏れがないことを検証する。
// ファイルを用意して実行する:
//
//	KATASHIRO_KEN_ALL=/path/to/utf_ken_all.zip go test ./internal/masterbuild -run RealData -v
func TestKenAllRealData(t *testing.T) {
	path := os.Getenv("KATASHIRO_KEN_ALL")
	if path == "" {
		t.Skip("KATASHIRO_KEN_ALL が未設定のためスキップ")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	addrs, st, err := parseKenAll(b)
	if err != nil {
		t.Fatal(err)
	}
	ab, err := master.BuildAddresses(addrs)
	if err != nil {
		t.Fatal(err)
	}
	nb, _ := master.BuildNames(seedNamesFile())
	d, err := master.NewDict(ab, nb)
	if err != nil {
		t.Fatal(err)
	}
	if d.Addresses.Len() < 100_000 {
		t.Fatalf("addresses=%d, expected >100k", d.Addresses.Len())
	}
	if st.Actions[actFragment] == 0 || st.Actions[actStripParen] == 0 || st.Actions[actNoListing] == 0 {
		t.Fatalf("expected every cleansing rule to fire on real data: %v", st.Actions)
	}
	var buf bytes.Buffer
	if err := Report(&buf, d, st); err != nil {
		t.Fatalf("%v\n%s", err, buf.String())
	}
	// 既知の郵便番号が期待どおりに整形されていること
	want := map[string]master.Address{
		"1000001": {Prefecture: "東京都", City: "千代田区", Town: "千代田"},
		"8100001": {Prefecture: "福岡県", City: "福岡市中央区", Town: "天神"},
		"0600000": {Prefecture: "北海道", City: "札幌市中央区", Town: ""},
	}
	found := map[string]bool{}
	for i := 0; i < d.Addresses.Len(); i++ {
		a := d.Address(i)
		if w, ok := want[a.Zip]; ok {
			if a.Prefecture != w.Prefecture || a.City != w.City || a.Town != w.Town {
				t.Errorf("%s = %s %s %q, want %s %s %q", a.Zip, a.Prefecture, a.City, a.Town, w.Prefecture, w.City, w.Town)
			}
			found[a.Zip] = true
		}
	}
	for z := range want {
		if !found[z] {
			t.Errorf("zip %s not found", z)
		}
	}
}

// TestCheckDetectsIssues はレポートがクレンジング漏れを検出して失敗することを確認する。
func TestCheckDetectsIssues(t *testing.T) {
	entries, _ := seedAddressEntries()
	entries = append(entries, master.AddressEntry{Zip: "1000002", Prefecture: "東京都", City: "千代田区", Town: "皇居外苑（その他"})
	ab, err := master.BuildAddresses(entries)
	if err != nil {
		t.Fatal(err)
	}
	nb, _ := master.BuildNames(seedNamesFile())
	d, _ := master.NewDict(ab, nb)
	var buf bytes.Buffer
	if err := Report(&buf, d, nil); err == nil || !strings.Contains(buf.String(), "皇居外苑") {
		t.Fatalf("issue must be reported:\n%s", buf.String())
	}
}

func TestParseGimeiYAML(t *testing.T) {
	yml := `first_name:
  male:
  - - 蓮
    - れん
    - レン
  female:
  - - 陽菜
    - ひな
    - ヒナ
  - - 陽菜
    - はるな
    - ハルナ
last_name:
- - 佐藤
  - さとう
  - サトウ
`
	nf, err := parseGimeiYAML([]byte(yml))
	if err != nil {
		t.Fatal(err)
	}
	if len(nf.Male) != 1 || len(nf.Female) != 2 || len(nf.LastNames) != 1 || nf.Female[1].Hiragana != "はるな" {
		t.Fatalf("unexpected: %+v", nf)
	}
	flow := `first_name:
  male:
    - ['愛斗', 'あいと', 'アイト', 'Aito']
  female:
    - ['陽菜', 'ひな', 'ヒナ', 'Hina']
last_name:
  - ['佐藤', 'さとう', 'サトウ', 'Satou']
  - ['林', 'はやし', 'ハヤシ', 'Hayashi']
`
	nf, err = parseGimeiYAML([]byte(flow))
	if err != nil {
		t.Fatal(err)
	}
	if len(nf.Male) != 1 || len(nf.Female) != 1 || len(nf.LastNames) != 2 || nf.LastNames[0].Kanji != "佐藤" || nf.LastNames[0].Hiragana != "さとう" {
		t.Fatalf("flow style: %+v", nf)
	}
}

func TestSourceNameHidesLocalPath(t *testing.T) {
	for in, want := range map[string]string{
		"":                                 SourceBuiltinSeed,
		"/home/someone/dl/utf_ken_all.zip": "utf_ken_all.zip",
		DefaultKenAllURL:                   DefaultKenAllURL,
	} {
		if got := sourceName(in); got != want {
			t.Errorf("sourceName(%q)=%q want %q", in, got, want)
		}
	}
}

func TestWriteDirReplacesOnlyDictionaryFiles(t *testing.T) {
	r, err := Build(Sources{})
	if err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	dir := filepath.Join(parent, "master")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 共有用ディレクトリに置かれた他のファイルは残る
	writeFile(t, filepath.Join(dir, "README.md"), "team notes")
	writeFile(t, filepath.Join(dir, master.AddressesBinName), "old")
	if err := r.WriteDir(dir, time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	d, err := master.LoadDir(dir)
	if err != nil || d.Fingerprint() != r.Dict.Fingerprint() {
		t.Fatalf("dictionary not replaced: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "README.md")); string(b) != "team notes" {
		t.Fatal("other files must be kept")
	}
	assertNoStaging(t, parent)

	// 書き出しに失敗しても一時ディレクトリは残らず、既存の辞書もそのまま
	blocked := filepath.Join(parent, "blocked")
	writeFile(t, blocked, "not a dir")
	if err := r.WriteDir(blocked, time.Unix(0, 0)); err == nil {
		t.Fatal("writing into a regular file path must fail")
	}
	assertNoStaging(t, parent)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertNoStaging(t *testing.T, parent string) {
	t.Helper()
	es, _ := os.ReadDir(parent)
	for _, e := range es {
		if strings.HasPrefix(e.Name(), ".katashiro-master-") {
			t.Fatalf("staging dir left behind: %s", e.Name())
		}
	}
}
