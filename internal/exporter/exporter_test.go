package exporter

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/withnic/katashiro/internal/model"
)

func gen(t *testing.T, sets ...string) *model.Generator {
	t.Helper()
	o := model.DefaultOptions()
	o.WithFinance, o.WithKYC, o.WithProfile = true, true, true
	o.Sets = sets
	g, err := model.NewGenerator(o)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func writeAll(t *testing.T, w Writer, g *model.Generator, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := w.Write(g.At(uint32(i))); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestJSONStreaming(t *testing.T) {
	g := gen(t)
	for _, n := range []int{0, 1, 50} {
		var buf bytes.Buffer
		writeAll(t, NewJSON(&buf), g, n)
		var out []model.Persona
		if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
			t.Fatalf("n=%d invalid JSON: %v\n%s", n, err, buf.String())
		}
		if len(out) != n {
			t.Fatalf("len=%d want %d", len(out), n)
		}
	}
}

func TestJSONL(t *testing.T) {
	var buf bytes.Buffer
	writeAll(t, NewJSONL(&buf), gen(t), 20)
	sc := bufio.NewScanner(&buf)
	lines := 0
	for sc.Scan() {
		var p model.Persona
		if err := json.Unmarshal(sc.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		lines++
	}
	if lines != 20 {
		t.Fatalf("lines=%d", lines)
	}
}

func TestCSVFlatColumns(t *testing.T) {
	g := gen(t, "personal.age=17")
	var buf bytes.Buffer
	writeAll(t, NewCSV(&buf, g.Columns()), g, 10)
	recs, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 11 {
		t.Fatalf("rows=%d", len(recs))
	}
	h := recs[0]
	if h[0] != "id" || h[1] != "personal_birthday" || h[len(h)-1] != "profile_dependents_count" {
		t.Fatalf("header=%v", h)
	}
	idx := map[string]int{}
	for i, c := range h {
		idx[c] = i
	}
	for _, r := range recs[1:] {
		if r[idx["profile_is_married"]] != "false" || r[idx["kyc_drivers_license_number"]] != "" {
			t.Fatalf("row=%v", r)
		}
	}
	// モジュール未指定時は該当列を出力しない
	o := model.DefaultOptions()
	g2, _ := model.NewGenerator(o)
	for _, c := range g2.Columns() {
		if strings.HasPrefix(c.Name, "finance_") || strings.HasPrefix(c.Name, "kyc_") || strings.HasPrefix(c.Name, "profile_") {
			t.Fatalf("unexpected column %s", c.Name)
		}
	}
}

func TestSQLiteExport(t *testing.T) {
	g := gen(t)
	path := filepath.Join(t.TempDir(), "out.sqlite")
	w, err := NewSQLite(path, g.Columns(), false)
	if err != nil {
		t.Fatal(err)
	}
	writeAll(t, w, g, 2000)

	if _, err := NewSQLite(path, g.Columns(), false); err == nil {
		t.Fatal("existing file without --force must fail")
	}
	w, err = NewSQLite(path, g.Columns(), true)
	if err != nil {
		t.Fatal(err)
	}
	writeAll(t, w, g, 1500)

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n, distinctEmail, distinctCard int
	if err := db.QueryRow("SELECT COUNT(*), COUNT(DISTINCT contact_email), COUNT(DISTINCT finance_card_number) FROM personas").Scan(&n, &distinctEmail, &distinctCard); err != nil {
		t.Fatal(err)
	}
	if n != 1500 || distinctEmail != 1500 || distinctCard != 1500 {
		t.Fatalf("n=%d email=%d card=%d", n, distinctEmail, distinctCard)
	}
	rows, err := db.Query("SELECT name, type FROM pragma_table_info('personas') ORDER BY cid")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	types := map[string]string{}
	for rows.Next() {
		var name, typ string
		if err := rows.Scan(&name, &typ); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
		types[name] = typ
	}
	cols := g.Columns()
	if len(names) != len(cols) {
		t.Fatalf("columns=%d want %d", len(names), len(cols))
	}
	for i, c := range cols {
		if names[i] != c.Name {
			t.Fatalf("column %d = %s, want %s", i, names[i], c.Name)
		}
	}
	if types["profile_is_married"] != "INTEGER" || types["personal_age"] != "INTEGER" || types["contact_email"] != "TEXT" {
		t.Fatalf("types=%v", types)
	}
	var married int
	if err := db.QueryRow("SELECT COUNT(*) FROM personas WHERE profile_is_married NOT IN (0, 1)").Scan(&married); err != nil || married != 0 {
		t.Fatalf("is_married must be 0/1: %d %v", married, err)
	}
}

func TestSQLiteNullLicense(t *testing.T) {
	g := gen(t, "personal.age=17")
	path := filepath.Join(t.TempDir(), "minor.db")
	w, err := NewSQLite(path, g.Columns(), false)
	if err != nil {
		t.Fatal(err)
	}
	writeAll(t, w, g, 10)
	db, _ := sql.Open("sqlite", path)
	defer db.Close()
	var nulls int
	if err := db.QueryRow("SELECT COUNT(*) FROM personas WHERE kyc_drivers_license_number IS NULL").Scan(&nulls); err != nil {
		t.Fatal(err)
	}
	if nulls != 10 {
		t.Fatalf("nulls=%d", nulls)
	}
}
