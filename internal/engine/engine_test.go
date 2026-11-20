package engine

import "testing"

// TestFeistelBijection1M は 0..999,999 の 100 万件すべてが [0, 999999] の範囲内で
// 衝突ゼロの全単射になることをビットセットで検証する。
func TestFeistelBijection1M(t *testing.T) {
	for _, seed := range []int64{1, 42, -7} {
		f := NewFeistel(seed)
		var seen [SpaceSize/64 + 1]uint64
		for i := uint32(0); i < SpaceSize; i++ {
			y := f.Permute(i)
			if y >= SpaceSize {
				t.Fatalf("seed=%d: Permute(%d)=%d out of range", seed, i, y)
			}
			w, b := y/64, y%64
			if seen[w]&(1<<b) != 0 {
				t.Fatalf("seed=%d: collision at Permute(%d)=%d", seed, i, y)
			}
			seen[w] |= 1 << b
			if i%9973 == 0 && f.Inverse(y) != i {
				t.Fatalf("seed=%d: Inverse(Permute(%d)) mismatch", seed, i)
			}
		}
	}
}

func TestFeistelDeterministicAndSeedDependent(t *testing.T) {
	a, b, c := NewFeistel(1), NewFeistel(1), NewFeistel(2)
	diff := 0
	for i := uint32(0); i < 1000; i++ {
		if a.Permute(i) != b.Permute(i) {
			t.Fatal("same seed must give same permutation")
		}
		if a.Permute(i) != c.Permute(i) {
			diff++
		}
	}
	if diff < 900 {
		t.Fatalf("different seeds should differ widely, diff=%d", diff)
	}
}

func TestAttrRandSaltIndependence(t *testing.T) {
	same := 0
	for id := uint32(0); id < 10000; id++ {
		if AttrRand(1, id, SaltLastName)%2 == AttrRand(1, id, SaltGender)%2 {
			same++
		}
	}
	if same < 4700 || same > 5300 {
		t.Fatalf("attributes look correlated: %d/10000", same)
	}
}

func TestLuhn(t *testing.T) {
	for _, n := range []string{"4111111111111111", "5555555555554444", "378282246310005", "3530111333300000"} {
		if !LuhnValid(n) {
			t.Errorf("%s should be Luhn-valid", n)
		}
	}
	if LuhnValid("4111111111111112") {
		t.Error("invalid number accepted")
	}
}

func TestMyNumberCheckDigit(t *testing.T) {
	// 手計算: 12345678901 → Σ=197, 197 mod 11 = 10, 11-10 = 1
	if got := MyNumberCheckDigit("12345678901"); got != '1' {
		t.Fatalf("got %c, want 1", got)
	}
	// 余り 0/1 のとき 0
	if got := MyNumberCheckDigit("00000000000"); got != '0' {
		t.Fatalf("got %c, want 0", got)
	}
	if !MyNumberValid("123456789011") || MyNumberValid("123456789012") {
		t.Fatal("MyNumberValid mismatch")
	}
}

func TestLicenseCheckDigit(t *testing.T) {
	// 重みテーブルを明示した独立実装と突き合わせる
	payload := "9020000001"
	sum := 0
	weights := []int{2, 3, 4, 5, 6, 7, 2, 3, 4, 5}
	for n := 0; n < 10; n++ {
		sum += int(payload[9-n]-'0') * weights[n]
	}
	want := byte('0')
	if r := sum % 11; r > 1 {
		want = byte('0' + 11 - r)
	}
	if got := LicenseCheckDigit(payload); got != want {
		t.Fatalf("got %c want %c", got, want)
	}
}
