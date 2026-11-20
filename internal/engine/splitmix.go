// Package engine は katashiro の決定論的生成コア（Cycle-Walking Feistel、
// SplitMix64、各種チェックディジット）を提供する。
package engine

// golden は SplitMix64 / attrRand で使う黄金比定数。
const golden = 0x9E3779B97F4A7C15

// SplitMix64 は SplitMix64 の finalizer（攪拌関数）を返す。
func SplitMix64(x uint64) uint64 {
	x += golden
	x = (x ^ (x >> 30)) * 0xBF58476D1CE4E5B9
	x = (x ^ (x >> 27)) * 0x94D049BB133111EB
	return x ^ (x >> 31)
}

// AttrRand は seqID と属性別ソルトから属性ごとに独立な決定論的乱数を返す。
//
//	attrRand(seqID, salt) = splitMix64(uint64(seed) ^ (uint64(seqID) * 0x9E3779B97F4A7C15) ^ salt)
func AttrRand(seed int64, seqID uint32, salt uint64) uint64 {
	return SplitMix64(uint64(seed) ^ (uint64(seqID) * golden) ^ salt)
}

// AttrSalt は属性ごとのソルト定数。値同士が独立であれば何でもよいが、
// 出力の再現性に直結するため既存値は変更しないこと。
const (
	SaltGender      uint64 = 0x6A09E667F3BCC908
	SaltBirthday    uint64 = 0xBB67AE8584CAA73B
	SaltLastName    uint64 = 0x3C6EF372FE94F82B
	SaltFirstName   uint64 = 0xA54FF53A5F1D36F1
	SaltAddress     uint64 = 0x510E527FADE682D1
	SaltStreet      uint64 = 0x9B05688C2B3E6C1F
	SaltBuilding    uint64 = 0x1F83D9ABFB41BD6B
	SaltPhone       uint64 = 0x5BE0CD19137E2179
	SaltEmailDomain uint64 = 0xCBBB9D5DC1059ED8
	SaltCardBrand   uint64 = 0x629A292A367CD507
	SaltCardMiddle  uint64 = 0x9159015A3070DD17
	SaltCardExp     uint64 = 0x152FECD8F70E5939
	SaltCardCVV     uint64 = 0x67332667FFC00B31
	SaltBank        uint64 = 0x8EB44A8768581511
	SaltBranch      uint64 = 0xDB0C2E0D64F98FA7
	SaltLicenseYear uint64 = 0x47B5481DBEFA4FA4
	SaltPassport    uint64 = 0xD1E0A3F2C4B59687
	SaltOccupation  uint64 = 0x2B7E151628AED2A6
	SaltIndustry    uint64 = 0xABF7158809CF4F3C
	SaltCompany     uint64 = 0x762E7160F38B4DA5
	SaltIncome      uint64 = 0x6A784D9045190CFE
	SaltMarried     uint64 = 0xF6A8C1B2D3E4F506
	SaltDependents  uint64 = 0x0F1E2D3C4B5A6978
	SaltKYCDocument uint64 = 0x3D1C5E7F9A2B4C6D
)
