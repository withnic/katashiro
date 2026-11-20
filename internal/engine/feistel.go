package engine

// SpaceSize は払い出し空間サイズ N（100万件）。
const SpaceSize = 1_000_000

const (
	halfBits = 10
	halfMask = 1<<halfBits - 1 // 0x3FF
	rounds   = 4
)

// Feistel は 20bit（10bit × 10bit、4ラウンド）の Feistel ネットワークに
// Cycle-Walking を組み合わせ、[0, SpaceSize) 上の全単射置換を提供する。
type Feistel struct {
	keys [rounds]uint64
}

// NewFeistel は seed からラウンド鍵を導出した置換を返す。
func NewFeistel(seed int64) *Feistel {
	f := &Feistel{}
	k := uint64(seed)
	for r := range f.keys {
		k = SplitMix64(k ^ uint64(r+1)*0xD6E8FEB86659FD93)
		f.keys[r] = k
	}
	return f
}

// roundF はラウンド関数 F(R, round, seed) & 0x3FF。
func (f *Feistel) roundF(r uint32, round int) uint32 {
	return uint32(SplitMix64(uint64(r)^f.keys[round]) & halfMask)
}

// encrypt は 20bit 空間 [0, 2^20) 上の置換。
func (f *Feistel) encrypt(x uint32) uint32 {
	l, r := x>>halfBits, x&halfMask
	for round := 0; round < rounds; round++ {
		l, r = r, l^f.roundF(r, round)
	}
	return l<<halfBits | r
}

// decrypt は encrypt の逆置換。
func (f *Feistel) decrypt(y uint32) uint32 {
	l, r := y>>halfBits, y&halfMask
	for round := rounds - 1; round >= 0; round-- {
		l, r = r^f.roundF(l, round), l
	}
	return l<<halfBits | r
}

// Permute はカーソル i ∈ [0, SpaceSize) を一意の seqID ∈ [0, SpaceSize) に写す。
// 出力が SpaceSize 以上になった場合は範囲内に入るまで置換を繰り返す（Cycle-Walking）。
// i が範囲外の場合は panic する。
func (f *Feistel) Permute(i uint32) uint32 {
	if i >= SpaceSize {
		panic("engine: Feistel input out of range")
	}
	y := f.encrypt(i)
	for y >= SpaceSize {
		y = f.encrypt(y)
	}
	return y
}

// Inverse は Permute の逆写像（seqID → カーソル）。
func (f *Feistel) Inverse(seqID uint32) uint32 {
	if seqID >= SpaceSize {
		panic("engine: Feistel input out of range")
	}
	x := f.decrypt(seqID)
	for x >= SpaceSize {
		x = f.decrypt(x)
	}
	return x
}
