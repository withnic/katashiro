package exporter

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"

	"github.com/withnic/katashiro/internal/model"
)

type jsonWriter struct {
	w     *bufio.Writer
	buf   bytes.Buffer
	enc   *json.Encoder
	lines bool
	n     int
}

// NewJSON は JSON 配列を 1 件ずつストリーミング出力する Writer を返す。
func NewJSON(w io.Writer) Writer { return newJSON(w, false) }

// NewJSONL は 1 行 1 件の JSON Lines を出力する Writer を返す。
func NewJSONL(w io.Writer) Writer { return newJSON(w, true) }

func newJSON(w io.Writer, lines bool) *jsonWriter {
	j := &jsonWriter{w: bufio.NewWriterSize(w, 64<<10), lines: lines}
	j.enc = json.NewEncoder(&j.buf)
	j.enc.SetEscapeHTML(false)
	return j
}

func (j *jsonWriter) Write(p *model.Persona) error {
	j.buf.Reset()
	if err := j.enc.Encode(p); err != nil {
		return err
	}
	b := j.buf.Bytes() // 末尾に改行付き
	if !j.lines {
		if j.n == 0 {
			j.w.WriteString("[\n")
		} else {
			j.w.WriteString(",\n")
		}
		b = b[:len(b)-1]
	}
	j.n++
	_, err := j.w.Write(b)
	return err
}

func (j *jsonWriter) Close() error {
	if !j.lines {
		if j.n == 0 {
			j.w.WriteString("[]\n")
		} else {
			j.w.WriteString("\n]\n")
		}
	}
	return j.w.Flush()
}
