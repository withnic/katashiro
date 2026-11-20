// Package master は //go:embed で埋め込んだゼロパース辞書（住所・人名）と、
// 全銀協カナ・ヘボン式ローマ字などの文字変換を提供する。
package master

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
)

// genmaster は data/*.bin が無い状態でもビルドできるよう、build tag
// katashiro_genmaster 付きでビルドする（埋め込みを外したスタブが使われる）。
//go:generate go run -tags katashiro_genmaster ../../cmd/genmaster -out data

// 住所レコードのフィールド番号。
const (
	AddrZip = iota
	AddrPrefecture
	AddrCity
	AddrTown
	AddrPSCCode
	AddrFields
)

// 人名レコードのフィールド番号。
const (
	NameKanji = iota
	NameHiragana
	NameKatakana
	NameZengin
	NameRomaji
	NameFields
)

// 人名辞書のグループ番号。
const (
	GroupLastName = iota
	GroupMale
	GroupFemale
	NameGroups
)

// Dict は住所・人名辞書の組。
type Dict struct {
	Addresses *Table // グループ g = JIS 都道府県コード - 1
	Names     *Table // グループ: 姓 / 男性名 / 女性名

	addressesRaw, namesRaw []byte
}

// 辞書ディレクトリのファイル名。
const (
	AddressesBinName = "addresses.bin"
	NamesBinName     = "names.bin"
	ManifestFile     = "manifest.json"
)

// Stats は辞書の件数。
type Stats struct {
	Addresses, LastNames, MaleNames, FemaleNames int
}

// Stats は辞書の件数を返す。
func (d *Dict) Stats() Stats {
	n := func(g int) int { s, e := d.Names.Group(g); return e - s }
	return Stats{Addresses: d.Addresses.Len(), LastNames: n(GroupLastName), MaleNames: n(GroupMale), FemaleNames: n(GroupFemale)}
}

// Fingerprint は辞書バイナリの SHA-256（addresses.bin と names.bin を順に連結したもの）の 16 進表記。
// 同じ値なら同じ seed で同じデータが生成される。
func Fingerprint(addresses, names []byte) string {
	h := sha256.New()
	h.Write(addresses)
	h.Write(names)
	return hex.EncodeToString(h.Sum(nil))
}

// Fingerprint はこの辞書の Fingerprint を返す。
func (d *Dict) Fingerprint() string { return Fingerprint(d.addressesRaw, d.namesRaw) }

// Address は住所レコード 1 件のゼロコピービュー。
type Address struct {
	Zip, Prefecture, City, Town, PSCCode string
}

// Name は人名レコード 1 件のゼロコピービュー。
type Name struct {
	Kanji, Hiragana, Katakana, Zengin, Romaji string
}

// Address は i 番目の住所レコードを返す。
func (d *Dict) Address(i int) Address {
	t := d.Addresses
	return Address{
		Zip: t.Field(i, AddrZip), Prefecture: t.Field(i, AddrPrefecture),
		City: t.Field(i, AddrCity), Town: t.Field(i, AddrTown), PSCCode: t.Field(i, AddrPSCCode),
	}
}

// Name は i 番目の人名レコードを返す。
func (d *Dict) Name(i int) Name {
	t := d.Names
	return Name{
		Kanji: t.Field(i, NameKanji), Hiragana: t.Field(i, NameHiragana),
		Katakana: t.Field(i, NameKatakana), Zengin: t.Field(i, NameZengin), Romaji: t.Field(i, NameRomaji),
	}
}

// PrefectureRange は JIS 都道府県コード code の住所レコード範囲 [start, end) を返す。
func (d *Dict) PrefectureRange(code int) (start, end int) {
	return d.Addresses.Group(code - 1)
}

// NewDict はバイナリ辞書から Dict を開き、構造を検証する。
func NewDict(addresses, names []byte) (*Dict, error) {
	a, err := OpenTable(addresses)
	if err != nil {
		return nil, fmt.Errorf("addresses: %w", err)
	}
	n, err := OpenTable(names)
	if err != nil {
		return nil, fmt.Errorf("names: %w", err)
	}
	if a.nfield != AddrFields || a.ngroup != len(Prefectures) {
		return nil, fmt.Errorf("addresses: unexpected layout (fields=%d groups=%d)", a.nfield, a.ngroup)
	}
	if n.nfield != NameFields || n.ngroup != NameGroups {
		return nil, fmt.Errorf("names: unexpected layout (fields=%d groups=%d)", n.nfield, n.ngroup)
	}
	for g := 0; g < a.ngroup; g++ {
		if s, e := a.Group(g); s == e {
			return nil, fmt.Errorf("addresses: prefecture %s has no records", Prefectures[g].Name)
		}
	}
	for g := 0; g < n.ngroup; g++ {
		if s, e := n.Group(g); s == e {
			return nil, fmt.Errorf("names: group %d has no records", g)
		}
	}
	return &Dict{Addresses: a, Names: n, addressesRaw: addresses, namesRaw: names}, nil
}

var (
	defaultOnce sync.Once
	defaultDict *Dict
	defaultErr  error
)

// Default は埋め込み辞書を返す（ヘッダ読み取りのみの O(1) オープン）。
func Default() (*Dict, error) {
	defaultOnce.Do(func() {
		defaultDict, defaultErr = NewDict(addressesBin, namesBin)
	})
	return defaultDict, defaultErr
}
