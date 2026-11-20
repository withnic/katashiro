// Package state はプロファイルごとの払い出しカーソル（cursor_<profile>.json）を
// ファイルロック付きで永続化する。
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofrs/flock"

	"github.com/withnic/katashiro/internal/engine"
)

// ErrExhausted は 1,000,000 件の上限に達し、--loop なしでは払い出せないことを示す。
var ErrExhausted = errors.New("cursor exhausted: 1,000,000 personas already issued for this profile (use --loop to wrap around or reset)")

// DefaultProfile は --profile 未指定時のプロファイル名。
const DefaultProfile = "default"

var profileRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ValidateProfile はプロファイル名がファイル名として安全かを検証する。
func ValidateProfile(p string) error {
	if !profileRe.MatchString(p) {
		return fmt.Errorf("invalid profile name %q (allowed: A-Z a-z 0-9 _ -, max 64)", p)
	}
	return nil
}

// Allocator はカーソル払い出しの抽象（ファイル永続 / オンメモリ）。
type Allocator interface {
	// Reserve は profile のカーソルから n 件を確保し、開始カーソルを返す。
	// 確保したインデックスは (start + k) mod 1,000,000（k = 0..n-1）。
	Reserve(profile string, n int, loop bool) (start uint32, err error)
	// Reset は profile のカーソルを 0 に戻す。
	Reset(profile string) error
}

// advance は払い出しの共通規則。上限超過時は loop=false なら ErrExhausted。
func advance(cur uint64, n int, loop bool) (start uint32, next uint64, err error) {
	if n <= 0 || n > engine.SpaceSize {
		return 0, cur, fmt.Errorf("count must be 1..%d, got %d", engine.SpaceSize, n)
	}
	if !loop {
		if cur+uint64(n) > engine.SpaceSize {
			return 0, cur, ErrExhausted
		}
		return uint32(cur), cur + uint64(n), nil
	}
	cur %= engine.SpaceSize
	return uint32(cur), (cur + uint64(n)) % engine.SpaceSize, nil
}

// ResolveDir は状態保存ディレクトリを解決する。
//  1. KATASHIRO_STATE_DIR
//  2. $XDG_STATE_HOME/katashiro
//  3. Unix 系: ~/.local/state/katashiro、Windows: os.UserConfigDir()/katashiro
func ResolveDir() (string, error) {
	if d := os.Getenv("KATASHIRO_STATE_DIR"); d != "" {
		return d, nil
	}
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "katashiro"), nil
	}
	if runtime.GOOS == "windows" {
		d, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(d, "katashiro"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "katashiro"), nil
}

// FileStore は cursor_<profile>.json をファイルロックで保護して読み書きする。
// プロセス間は gofrs/flock、プロセス内は mutex で排他する。
type FileStore struct {
	dir string
	mu  sync.Mutex
}

// NewFileStore は dir を保存先とする FileStore を返す。
func NewFileStore(dir string) *FileStore { return &FileStore{dir: dir} }

type cursorFile struct {
	Profile   string `json:"profile"`
	Cursor    uint64 `json:"cursor"`
	UpdatedAt string `json:"updated_at"`
}

func (s *FileStore) path(profile string) string {
	return filepath.Join(s.dir, "cursor_"+profile+".json")
}

// withLock は profile のロックを取得して fn(現在値) を実行し、戻り値を保存する。
func (s *FileStore) withLock(profile string, fn func(cur uint64) (uint64, error)) error {
	if err := ValidateProfile(profile); err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lk := flock.New(s.path(profile) + ".lock")
	if err := lk.Lock(); err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	defer func() { _ = lk.Unlock() }() // ロックファイルはプロセス終了時にも解放される

	var cf cursorFile
	b, err := os.ReadFile(s.path(profile))
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(b, &cf); err != nil {
			return fmt.Errorf("corrupted cursor file %s: %w", s.path(profile), err)
		}
	}
	next, err := fn(cf.Cursor)
	if err != nil {
		return err
	}
	out, _ := json.MarshalIndent(cursorFile{Profile: profile, Cursor: next, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}, "", "  ")
	tmp, err := os.CreateTemp(s.dir, ".cursor_*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(out, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), s.path(profile))
}

// Reserve は Allocator を実装する。
func (s *FileStore) Reserve(profile string, n int, loop bool) (uint32, error) {
	var start uint32
	err := s.withLock(profile, func(cur uint64) (uint64, error) {
		st, next, err := advance(cur, n, loop)
		start = st
		return next, err
	})
	return start, err
}

// Reset は Allocator を実装する。
func (s *FileStore) Reset(profile string) error {
	return s.withLock(profile, func(uint64) (uint64, error) { return 0, nil })
}

// Cursor は profile の現在のカーソル値を返す（ロックなしの参照用）。
func (s *FileStore) Cursor(profile string) (uint64, error) {
	if err := ValidateProfile(profile); err != nil {
		return 0, err
	}
	b, err := os.ReadFile(s.path(profile))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var cf cursorFile
	if err := json.Unmarshal(b, &cf); err != nil {
		return 0, err
	}
	return cf.Cursor, nil
}

// MemoryStore はオンメモリのアトミックカウンタによる Allocator（serve --in-memory）。
type MemoryStore struct {
	mu       sync.RWMutex
	counters map[string]*atomic.Uint64
}

// NewMemoryStore は空の MemoryStore を返す。
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{counters: map[string]*atomic.Uint64{}}
}

func (m *MemoryStore) counter(profile string) *atomic.Uint64 {
	m.mu.RLock()
	c := m.counters[profile]
	m.mu.RUnlock()
	if c != nil {
		return c
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if c = m.counters[profile]; c == nil {
		c = new(atomic.Uint64)
		m.counters[profile] = c
	}
	return c
}

// Reserve は Allocator を実装する（CAS ループ）。
func (m *MemoryStore) Reserve(profile string, n int, loop bool) (uint32, error) {
	if err := ValidateProfile(profile); err != nil {
		return 0, err
	}
	c := m.counter(profile)
	for {
		cur := c.Load()
		start, next, err := advance(cur, n, loop)
		if err != nil {
			return 0, err
		}
		if c.CompareAndSwap(cur, next) {
			return start, nil
		}
	}
}

// Reset は Allocator を実装する。
func (m *MemoryStore) Reset(profile string) error {
	if err := ValidateProfile(profile); err != nil {
		return err
	}
	m.counter(profile).Store(0)
	return nil
}
