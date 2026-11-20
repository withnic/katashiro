package state

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

func TestResolveDir(t *testing.T) {
	t.Setenv("KATASHIRO_STATE_DIR", "/tmp/k1")
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg")
	if d, _ := ResolveDir(); d != "/tmp/k1" {
		t.Fatalf("got %s", d)
	}
	t.Setenv("KATASHIRO_STATE_DIR", "")
	if d, _ := ResolveDir(); d != filepath.Join("/tmp/xdg", "katashiro") {
		t.Fatalf("got %s", d)
	}
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/home/x")
	if d, _ := ResolveDir(); d != "/home/x/.local/state/katashiro" {
		t.Fatalf("got %s", d)
	}
}

func testAllocator(t *testing.T, a Allocator) {
	start, err := a.Reserve("p", 10, false)
	if err != nil || start != 0 {
		t.Fatalf("start=%d err=%v", start, err)
	}
	if start, _ = a.Reserve("p", 5, false); start != 10 {
		t.Fatalf("start=%d", start)
	}
	if start, _ = a.Reserve("other", 1, false); start != 0 {
		t.Fatal("profiles must be independent")
	}
	if _, err := a.Reserve("p", 999_990, false); !errors.Is(err, ErrExhausted) {
		t.Fatalf("want ErrExhausted, got %v", err)
	}
	if start, err = a.Reserve("p", 999_985, false); err != nil || start != 15 {
		t.Fatalf("exact fill: start=%d err=%v", start, err)
	}
	if _, err := a.Reserve("p", 1, false); !errors.Is(err, ErrExhausted) {
		t.Fatal("must be exhausted at 1,000,000")
	}
	if start, err = a.Reserve("p", 3, true); err != nil || start != 0 {
		t.Fatalf("loop wrap: start=%d err=%v", start, err)
	}
	if err := a.Reset("p"); err != nil {
		t.Fatal(err)
	}
	if start, _ = a.Reserve("p", 1, false); start != 0 {
		t.Fatal("reset failed")
	}
	if _, err := a.Reserve("../evil", 1, false); err == nil {
		t.Fatal("path traversal must be rejected")
	}
}

func TestFileStore(t *testing.T) { testAllocator(t, NewFileStore(t.TempDir())) }

func TestMemoryStore(t *testing.T) { testAllocator(t, NewMemoryStore()) }

func TestLoopWrapsPartially(t *testing.T) {
	m := NewMemoryStore()
	if _, err := m.Reserve("p", 999_998, false); err != nil {
		t.Fatal(err)
	}
	start, err := m.Reserve("p", 5, true)
	if err != nil || start != 999_998 {
		t.Fatalf("start=%d err=%v", start, err)
	}
	if start, _ = m.Reserve("p", 1, true); start != 3 {
		t.Fatalf("cursor after wrap = %d, want 3", start)
	}
}

// 複数 FileStore（別プロセス相当）から並行に確保しても区間が重ならないこと。
func TestFileStoreConcurrent(t *testing.T) {
	dir := t.TempDir()
	const workers, per = 8, 25
	var mu sync.Mutex
	seen := map[uint32]bool{}
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := NewFileStore(dir) // ロックは flock で共有
			for i := 0; i < per; i++ {
				start, err := s.Reserve("c", 3, false)
				if err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				for k := uint32(0); k < 3; k++ {
					if seen[start+k] {
						t.Errorf("duplicate index %d", start+k)
					}
					seen[start+k] = true
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if c, _ := NewFileStore(dir).Cursor("c"); c != workers*per*3 {
		t.Fatalf("cursor=%d", c)
	}
}
