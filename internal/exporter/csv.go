package exporter

import (
	"bufio"
	"encoding/csv"
	"io"

	"github.com/withnic/katashiro/internal/model"
)

type csvWriter struct {
	bw     *bufio.Writer
	cw     *csv.Writer
	cols   []*model.Column
	row    []string
	header bool
}

// NewCSV はフラット列名ヘッダ付き CSV を出力する Writer を返す。
// bool は "true"/"false"、null は空文字で出力する。
func NewCSV(w io.Writer, cols []*model.Column) Writer {
	bw := bufio.NewWriterSize(w, 64<<10)
	return &csvWriter{bw: bw, cw: csv.NewWriter(bw), cols: cols, row: make([]string, len(cols))}
}

func (c *csvWriter) writeHeader() error {
	c.header = true
	for i, col := range c.cols {
		c.row[i] = col.Name
	}
	return c.cw.Write(c.row)
}

func (c *csvWriter) Write(p *model.Persona) error {
	if !c.header {
		if err := c.writeHeader(); err != nil {
			return err
		}
	}
	for i, col := range c.cols {
		c.row[i] = flatString(col.Value(p))
	}
	return c.cw.Write(c.row)
}

func (c *csvWriter) Close() error {
	if !c.header {
		if err := c.writeHeader(); err != nil {
			return err
		}
	}
	c.cw.Flush()
	if err := c.cw.Error(); err != nil {
		return err
	}
	return c.bw.Flush()
}
