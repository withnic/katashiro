//go:build katashiro_genmaster

package master

// genmaster 自身のビルド時は辞書を埋め込まない（Default はエラーを返す）。
var addressesBin, namesBin []byte
