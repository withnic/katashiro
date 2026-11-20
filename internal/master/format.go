package master

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unsafe"
)

// バイナリ辞書フォーマット（リトルエンディアン）:
//
//	[0:4)   magic "KTS1"
//	[4:8)   nfield  レコードあたりのフィールド数
//	[8:12)  nrec    レコード数
//	[12:16) ngroup  グループ数（住所=都道府県47、人名=姓/男名/女名の3）
//	ngroup+1 個の uint32: グループ開始レコード番号（最後は nrec）
//	nrec*nfield+1 個の uint32: 文字列プール内オフセット（単調増加）
//	文字列プール（UTF-8 連結）
//
// フィールド k = rec*nfield+field は pool[off[k]:off[k+1]]。
// 起動時は 16 バイトのヘッダを読むだけで、以後の参照はすべて O(1) かつ
// unsafe.String によるゼロコピー・ゼロアロケーション。
const magic = "KTS1"

const headerSize = 16

// Table はオフセットインデックス付きバイナリ辞書のビュー。
type Table struct {
	nfield, nrec, ngroup int
	groups               []byte
	offsets              []byte
	pool                 []byte
}

// OpenTable は data をコピーせずに Table として開く。data は以後変更してはならない。
func OpenTable(data []byte) (*Table, error) {
	if len(data) < headerSize || string(data[:4]) != magic {
		return nil, errors.New("master: invalid dictionary magic")
	}
	le := binary.LittleEndian
	t := &Table{
		nfield: int(le.Uint32(data[4:])),
		nrec:   int(le.Uint32(data[8:])),
		ngroup: int(le.Uint32(data[12:])),
	}
	if t.nfield == 0 {
		return nil, errors.New("master: nfield must be positive")
	}
	gEnd := headerSize + 4*(t.ngroup+1)
	oEnd := gEnd + 4*(t.nrec*t.nfield+1)
	if oEnd > len(data) {
		return nil, fmt.Errorf("master: truncated dictionary (%d bytes)", len(data))
	}
	t.groups = data[headerSize:gEnd]
	t.offsets = data[gEnd:oEnd]
	t.pool = data[oEnd:]
	if last := int(le.Uint32(t.offsets[len(t.offsets)-4:])); last != len(t.pool) {
		return nil, fmt.Errorf("master: pool size mismatch (%d != %d)", last, len(t.pool))
	}
	if int(le.Uint32(t.groups[len(t.groups)-4:])) != t.nrec {
		return nil, errors.New("master: group table mismatch")
	}
	return t, nil
}

// Len はレコード数を返す。
func (t *Table) Len() int { return t.nrec }

// Groups はグループ数を返す。
func (t *Table) Groups() int { return t.ngroup }

// Group はグループ g のレコード範囲 [start, end) を返す。
func (t *Table) Group(g int) (start, end int) {
	le := binary.LittleEndian
	return int(le.Uint32(t.groups[4*g:])), int(le.Uint32(t.groups[4*g+4:]))
}

// Field はレコード rec のフィールド field をゼロコピーで返す。
func (t *Table) Field(rec, field int) string {
	k := rec*t.nfield + field
	le := binary.LittleEndian
	s, e := le.Uint32(t.offsets[4*k:]), le.Uint32(t.offsets[4*k+4:])
	if s == e {
		return ""
	}
	return unsafe.String(&t.pool[s], int(e-s))
}

// BuildTable はグループ別レコード列からバイナリ辞書を組み立てる（genmaster / 外部マスタ用）。
func BuildTable(nfield int, groups [][][]string) ([]byte, error) {
	nrec := 0
	for _, g := range groups {
		nrec += len(g)
	}
	le := binary.LittleEndian
	head := make([]byte, headerSize, headerSize+4*(len(groups)+1)+4*(nrec*nfield+1))
	copy(head, magic)
	le.PutUint32(head[4:], uint32(nfield))
	le.PutUint32(head[8:], uint32(nrec))
	le.PutUint32(head[12:], uint32(len(groups)))
	start := 0
	for _, g := range groups {
		head = le.AppendUint32(head, uint32(start))
		start += len(g)
	}
	head = le.AppendUint32(head, uint32(nrec))
	var pool []byte
	for _, g := range groups {
		for _, rec := range g {
			if len(rec) != nfield {
				return nil, fmt.Errorf("master: record has %d fields, want %d", len(rec), nfield)
			}
			for _, f := range rec {
				head = le.AppendUint32(head, uint32(len(pool)))
				pool = append(pool, f...)
			}
		}
	}
	head = le.AppendUint32(head, uint32(len(pool)))
	return append(head, pool...), nil
}
