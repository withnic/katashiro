package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/withnic/katashiro/internal/master"
)

// TestSeedModeOffline は内蔵シードモードでネットワークなしに 47 都道府県の辞書を生成できることを検証する。
func TestSeedModeOffline(t *testing.T) {
	dir := t.TempDir()
	if err := run([]string{"-out", dir, "-json"}); err != nil {
		t.Fatal(err)
	}
	ab, _ := os.ReadFile(filepath.Join(dir, "addresses.bin"))
	nb, _ := os.ReadFile(filepath.Join(dir, "names.bin"))
	d, err := master.NewDict(ab, nb)
	if err != nil {
		t.Fatal(err)
	}
	if d.Addresses.Groups() != 47 {
		t.Fatalf("groups=%d", d.Addresses.Groups())
	}
	// 確認用 JSON は --master-dir 形式として読み戻せる
	d2, err := master.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if d2.Addresses.Len() != d.Addresses.Len() || d2.Names.Len() != d.Names.Len() {
		t.Fatal("master-dir JSON round trip mismatch")
	}
}
