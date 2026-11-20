package main

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/withnic/katashiro/internal/model"
)

func katashiro(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var o, e bytes.Buffer
	code = run(args, &o, &e)
	return code, o.String(), e.String()
}

func decode(t *testing.T, s string) []model.Persona {
	t.Helper()
	var ps []model.Persona
	if err := json.Unmarshal([]byte(s), &ps); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, s)
	}
	return ps
}

func TestNextAdvancesCursorAndReset(t *testing.T) {
	t.Setenv("KATASHIRO_STATE_DIR", t.TempDir())
	code, out, errOut := katashiro(t, "next", "5", "--format=json")
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut)
	}
	first := decode(t, out)
	_, out, _ = katashiro(t, "next", "5", "--format=json")
	second := decode(t, out)
	seen := map[uint64]bool{}
	for _, p := range append(first, second...) {
		if seen[p.ID] {
			t.Fatalf("duplicate id %d across next calls", p.ID)
		}
		seen[p.ID] = true
	}
	// プロファイルは独立
	_, out, _ = katashiro(t, "next", "1", "--profile=other")
	if decode(t, out)[0].ID != first[0].ID {
		t.Fatal("new profile must start at cursor 0")
	}
	if code, _, _ := katashiro(t, "reset"); code != 0 {
		t.Fatal("reset failed")
	}
	_, out, _ = katashiro(t, "next", "--format", "json", "1")
	if decode(t, out)[0].ID != first[0].ID {
		t.Fatal("reset must rewind")
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("KATASHIRO_STATE_DIR"), "cursor_default.json")); err != nil {
		t.Fatal(err)
	}
}

func TestNextExhaustedExitCode2(t *testing.T) {
	t.Setenv("KATASHIRO_STATE_DIR", t.TempDir())
	if code, _, e := katashiro(t, "next", "999999", "--format=jsonl"); code != 0 {
		t.Fatalf("code=%d %s", code, e)
	}
	code, out, errOut := katashiro(t, "next", "2")
	if code != 2 || out != "" || !strings.Contains(errOut, "exhausted") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	code, out, _ = katashiro(t, "next", "2", "--loop")
	if code != 0 || len(decode(t, out)) != 2 {
		t.Fatalf("--loop: code=%d", code)
	}
}

func TestNextFormatsAndModules(t *testing.T) {
	t.Setenv("KATASHIRO_STATE_DIR", t.TempDir())
	_, out, _ := katashiro(t, "next", "3", "--format=csv", "--with-finance", "--with-kyc", "--with-profile")
	recs, err := csv.NewReader(strings.NewReader(out)).ReadAll()
	if err != nil || len(recs) != 4 || len(recs[0]) != len(model.Columns(true, true, true)) {
		t.Fatalf("csv: %v rows=%d", err, len(recs))
	}
	_, out, _ = katashiro(t, "next", "4", "--format=jsonl")
	if n := strings.Count(out, "\n"); n != 4 {
		t.Fatalf("jsonl lines=%d", n)
	}
	if code, _, _ := katashiro(t, "next", "1", "--format=xml"); code != 1 {
		t.Fatal("bad format must fail")
	}
}

func TestExportDeterministicAndIndependent(t *testing.T) {
	t.Setenv("KATASHIRO_STATE_DIR", t.TempDir())
	_, a, _ := katashiro(t, "export", "50", "--seed=42", "--base-date=2026-01-01", "--format=jsonl")
	katashiro(t, "next", "10") // カーソルを進めても export には影響しない
	_, b, _ := katashiro(t, "export", "50", "--seed=42", "--base-date=2026-01-01", "--format=jsonl")
	if a != b || a == "" {
		t.Fatal("export must be deterministic and cursor-independent")
	}
	_, c, _ := katashiro(t, "export", "50", "--seed=43", "--format=jsonl")
	if a == c {
		t.Fatal("seed must change output")
	}
}

func TestExportSmartOverrides(t *testing.T) {
	_, out, _ := katashiro(t, "export", "200", "--set", "personal.age=17", "--with-kyc", "--with-profile")
	for _, p := range decode(t, out) {
		if p.Personal.Age != 17 || p.KYC.DriversLicenseNumber != nil || p.Profile.IsMarried || p.Profile.DependentsCount != 0 {
			t.Fatalf("%+v", p)
		}
	}
	_, out, _ = katashiro(t, "export", "200", "--set=contact.prefecture=福岡県", "--with-kyc")
	for _, p := range decode(t, out) {
		if p.Contact.Prefecture != "福岡県" || (*p.KYC.DriversLicenseNumber)[:2] != "90" {
			t.Fatalf("%+v", p.Contact)
		}
	}
	_, out, _ = katashiro(t, "export", "3", "--set", "personal.full_name=山田 太郎", "--set", "personal.gender=male")
	for _, p := range decode(t, out) {
		if p.Personal.FullName != "山田 太郎" || p.Personal.Gender != "male" {
			t.Fatalf("%+v", p.Personal)
		}
	}
	if code, _, e := katashiro(t, "export", "1", "--set", "finance.my_number=1"); code != 1 || !strings.Contains(e, "--with-finance") {
		t.Fatalf("code=%d stderr=%s", code, e)
	}
}

func TestExportSQLite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "personas.sqlite")
	if code, _, e := katashiro(t, "export", "2000", "--out="+path, "--with-finance", "--with-kyc", "--with-profile"); code != 0 {
		t.Fatalf("code=%d %s", code, e)
	}
	if code, _, e := katashiro(t, "export", "10", "--out="+path); code != 1 || !strings.Contains(e, "--force") {
		t.Fatalf("overwrite without --force: code=%d %s", code, e)
	}
	if code, _, e := katashiro(t, "export", "1000", "--out="+path, "--format=sqlite", "--force", "--with-kyc"); code != 0 {
		t.Fatalf("code=%d %s", code, e)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n, cols int
	db.QueryRow("SELECT COUNT(*), COUNT(DISTINCT kyc_passport_number) FROM personas").Scan(&n, &cols)
	if n != 1000 || cols != 1000 {
		t.Fatalf("n=%d distinct=%d", n, cols)
	}
	db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('personas')").Scan(&cols)
	if want := len(model.Columns(false, true, false)); cols != want {
		t.Fatalf("columns=%d want %d (only selected modules)", cols, want)
	}
	if code, _, _ := katashiro(t, "export", "1", "--format=sqlite"); code != 1 {
		t.Fatal("sqlite without --out must fail")
	}
}

func TestExportFileOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	if code, _, e := katashiro(t, "export", "100", "--out", path); code != 0 {
		t.Fatalf("code=%d %s", code, e)
	}
	b, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(b), "id,personal_birthday,") || strings.Count(string(b), "\n") != 101 {
		t.Fatalf("csv inferred from extension expected:\n%.200s", b)
	}
	if code, _, _ := katashiro(t, "export", "1", "--out", path); code != 1 {
		t.Fatal("must refuse to overwrite without --force")
	}
	if code, _, _ := katashiro(t, "export", "1", "--out", path, "--force"); code != 0 {
		t.Fatal("--force must overwrite")
	}
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{{}, {"bogus"}, {"next"}, {"next", "0"}, {"next", "1000001"}, {"export", "x"}, {"next", "1", "--base-date=2026/01/01"}} {
		if code, _, _ := katashiro(t, args...); code != 1 {
			t.Errorf("%v: code=%d want 1", args, code)
		}
	}
	if code, _, _ := katashiro(t, "next", "-h"); code != 0 {
		t.Error("-h must exit 0")
	}
}

func TestBaseDateToday(t *testing.T) {
	if code, out, e := katashiro(t, "export", "1", "--base-date=today"); code != 0 || len(decode(t, out)) != 1 {
		t.Fatalf("code=%d %s", code, e)
	}
}

// export を条件ごとに分割して結合するとき、--offset で重複を避けられる。
func TestExportOffset(t *testing.T) {
	_, a, _ := katashiro(t, "export", "300", "--with-kyc", "--set", "kyc.document_type=passport")
	_, b, _ := katashiro(t, "export", "700", "--offset=300", "--with-kyc", "--set", "kyc.document_type=drivers_license")
	_, all, _ := katashiro(t, "export", "1000", "--with-kyc")
	pa, pb, pall := decode(t, a), decode(t, b), decode(t, all)
	ids := map[uint64]bool{}
	for _, p := range append(pa, pb...) {
		if ids[p.ID] {
			t.Fatalf("duplicate id %d", p.ID)
		}
		ids[p.ID] = true
	}
	// 分割しても通しで出したときと同じ人が同じ順に並ぶ
	if pb[0].ID != pall[300].ID || pa[0].ID != pall[0].ID {
		t.Fatal("offset must select the same persons as a full export")
	}
	if code, _, _ := katashiro(t, "export", "10", "--offset=999995"); code != 1 {
		t.Fatal("offset+N beyond 1,000,000 must fail")
	}
}

func TestSetRawCLI(t *testing.T) {
	code, out, e := katashiro(t, "export", "3", "--with-profile", "--set", "personal.age=18",
		"--set-raw", "profile.occupation=医師", "--set-raw", "profile.annual_income_man_yen=1500")
	if code != 0 {
		t.Fatalf("code=%d %s", code, e)
	}
	for _, p := range decode(t, out) {
		if p.Personal.Age != 18 || p.Profile.Occupation != "医師" || p.Profile.AnnualIncomeManYen != 1500 {
			t.Fatalf("%+v", p.Profile)
		}
	}
}

// master fetch で作った辞書ディレクトリを --master-dir / KATASHIRO_MASTER_DIR で使える。
// ネットワークを使わないよう内蔵シード（seed）から作る。
func TestMasterFetchAndInfo(t *testing.T) {
	t.Setenv(masterDirEnv, "")
	dir := filepath.Join(t.TempDir(), "master")
	code, out, e := katashiro(t, "master", "fetch", "--out="+dir, "--ken-all=seed", "--names=seed")
	if code != 0 {
		t.Fatalf("code=%d %s", code, e)
	}
	for _, f := range []string{"addresses.bin", "names.bin", "manifest.json", "NOTICE"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
	if !strings.Contains(out, "KATASHIRO_MASTER_DIR=") {
		t.Fatalf("fetch should explain how to use it: %s", out)
	}
	// 既存の辞書は --force なしでは上書きしない
	if code, _, _ := katashiro(t, "master", "fetch", "--out="+dir, "--ken-all=seed", "--names=seed"); code != 1 {
		t.Fatal("fetch must not overwrite without --force")
	}

	// 内蔵シードから作った辞書は埋め込み辞書と同じフィンガープリント・同じデータになる
	_, embedded, _ := katashiro(t, "master", "info")
	_, external, _ := katashiro(t, "master", "info", "--master-dir="+dir)
	fp := func(s string) string {
		i := strings.Index(s, "フィンガープリント: ")
		if i < 0 {
			t.Fatalf("no fingerprint: %s", s)
		}
		return strings.TrimSpace(strings.SplitN(s[i:], "\n", 2)[0])
	}
	if fp(embedded) != fp(external) || !strings.Contains(embedded, "埋め込み") || !strings.Contains(external, "作成日時") {
		t.Fatalf("embedded:\n%s\nexternal:\n%s", embedded, external)
	}
	_, a, _ := katashiro(t, "export", "5")
	_, b, _ := katashiro(t, "export", "5", "--master-dir="+dir)
	t.Setenv(masterDirEnv, dir)
	_, c, _ := katashiro(t, "export", "5")
	_, envInfo, _ := katashiro(t, "master", "info")
	if a != b || b != c || !strings.Contains(envInfo, dir) {
		t.Fatal("same dictionary must give the same data via flag and env")
	}

	// 壊れた・欠けた辞書はエラー
	os.Remove(filepath.Join(dir, "names.bin"))
	if code, _, e := katashiro(t, "export", "1"); code != 1 || !strings.Contains(e, masterDirEnv) {
		t.Fatalf("missing names.bin must fail: %d %s", code, e)
	}
}
