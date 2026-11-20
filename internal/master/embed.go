//go:build !katashiro_genmaster

package master

import _ "embed"

//go:embed data/addresses.bin
var addressesBin []byte

//go:embed data/names.bin
var namesBin []byte
