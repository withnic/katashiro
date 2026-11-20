package exporter

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	_ "modernc.org/sqlite" // Pure Go SQLite ドライバ（CGO 不要）

	"github.com/withnic/katashiro/internal/model"
)

// TableName は SQLite 出力のテーブル名。
const TableName = "personas"

type sqliteWriter struct {
	db   *sql.DB
	tx   *sql.Tx
	stmt *sql.Stmt
	cols []*model.Column
	args []any
}

// SQLiteSchema は cols に対応する CREATE TABLE 文を返す。
func SQLiteSchema(cols []*model.Column) string {
	defs := make([]string, len(cols))
	for i, c := range cols {
		typ := "TEXT"
		if c.Kind == model.KindInt || c.Kind == model.KindBool {
			typ = "INTEGER"
		}
		def := c.Name + " " + typ
		switch {
		case c.Name == "id":
			def += " PRIMARY KEY"
		case c.Kind != model.KindNullableString:
			def += " NOT NULL"
		}
		defs[i] = def
	}
	return fmt.Sprintf("CREATE TABLE %s (\n  %s\n)", TableName, strings.Join(defs, ",\n  "))
}

// NewSQLite は path に personas テーブルを作成し、単一トランザクション + Prepared Statement で
// 一括 INSERT する Writer を返す。既存ファイルは force=true のときのみ DROP TABLE して再作成する。
func NewSQLite(path string, cols []*model.Column, force bool) (Writer, error) {
	if _, err := os.Stat(path); err == nil && !force {
		return nil, fmt.Errorf("%s already exists (use --force to overwrite)", path)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	fail := func(err error) (Writer, error) {
		db.Close()
		return nil, err
	}
	for _, q := range []string{
		"PRAGMA journal_mode=MEMORY",
		"PRAGMA synchronous=OFF",
		"DROP TABLE IF EXISTS " + TableName,
		SQLiteSchema(cols),
	} {
		if _, err := db.Exec(q); err != nil {
			return fail(fmt.Errorf("%s: %w", q, err))
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return fail(err)
	}
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.Name
	}
	q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", TableName,
		strings.Join(names, ", "), strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", "))
	stmt, err := tx.Prepare(q)
	if err != nil {
		tx.Rollback()
		return fail(err)
	}
	return &sqliteWriter{db: db, tx: tx, stmt: stmt, cols: cols, args: make([]any, len(cols))}, nil
}

func (s *sqliteWriter) Write(p *model.Persona) error {
	for i, c := range s.cols {
		switch v := c.Value(p).(type) {
		case bool:
			if v {
				s.args[i] = 1
			} else {
				s.args[i] = 0
			}
		case *string:
			if v == nil {
				s.args[i] = nil
			} else {
				s.args[i] = *v
			}
		default:
			s.args[i] = v
		}
	}
	_, err := s.stmt.Exec(s.args...)
	return err
}

func (s *sqliteWriter) Close() error {
	err := s.stmt.Close()
	if err == nil {
		err = s.tx.Commit()
	} else {
		s.tx.Rollback()
	}
	if cerr := s.db.Close(); err == nil {
		err = cerr
	}
	return err
}
