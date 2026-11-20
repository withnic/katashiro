// Package exporter は Persona を 1 件ずつストリーミング出力する（全件をメモリに保持しない）。
package exporter

import (
	"fmt"
	"strconv"

	"github.com/withnic/katashiro/internal/model"
)

// Writer は Persona のストリーミング出力先。
type Writer interface {
	Write(p *model.Persona) error
	// Close は未出力のバッファをフラッシュし、フォーマットを閉じる（下位の io.Writer は閉じない）。
	Close() error
}

// Formats は出力フォーマット名。
const (
	FormatJSON   = "json"
	FormatJSONL  = "jsonl"
	FormatCSV    = "csv"
	FormatSQLite = "sqlite"
)

// flatString はフラット出力（CSV）用に値を文字列化する。nil は空文字。
func flatString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case int:
		return strconv.Itoa(x)
	case bool:
		return strconv.FormatBool(x)
	case *string:
		if x == nil {
			return ""
		}
		return *x
	default:
		panic(fmt.Sprintf("exporter: unsupported value %T", v))
	}
}
