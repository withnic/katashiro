package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/withnic/katashiro/internal/model"
	"github.com/withnic/katashiro/internal/state"
)

func post(t *testing.T, url string) (*http.Response, []model.Persona) {
	t.Helper()
	resp, err := http.Post(url, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var ps []model.Persona
	if resp.StatusCode == http.StatusOK && strings.Contains(url, "/next") {
		if err := json.NewDecoder(resp.Body).Decode(&ps); err != nil {
			t.Fatal(err)
		}
	}
	return resp, ps
}

func TestNextResetAndConflict(t *testing.T) {
	for name, alloc := range map[string]state.Allocator{
		"memory": state.NewMemoryStore(),
		"file":   state.NewFileStore(t.TempDir()),
	} {
		t.Run(name, func(t *testing.T) {
			ts := httptest.NewServer(New(model.DefaultOptions(), alloc).Handler())
			defer ts.Close()
			resp, ps := post(t, ts.URL+"/api/v1/personas/next?count=10&profile=e2e&with_kyc=true&with_profile=true")
			if resp.StatusCode != 200 || len(ps) != 10 || ps[0].KYC == nil || ps[0].Profile == nil || ps[0].Finance != nil {
				t.Fatalf("status=%d len=%d", resp.StatusCode, len(ps))
			}
			_, ps2 := post(t, ts.URL+"/api/v1/personas/next?count=1&profile=e2e")
			if ps2[0].ID == ps[0].ID {
				t.Fatal("cursor must advance")
			}
			if resp, _ := post(t, ts.URL+"/api/v1/personas/reset?profile=e2e"); resp.StatusCode != 200 {
				t.Fatalf("reset status=%d", resp.StatusCode)
			}
			_, ps3 := post(t, ts.URL+"/api/v1/personas/next?profile=e2e")
			if ps3[0].ID != ps[0].ID {
				t.Fatal("reset must rewind to cursor 0")
			}
			// 上限到達で 409、loop=true でラップアラウンド
			if _, err := alloc.Reserve("full", 1_000_000, false); err != nil {
				t.Fatal(err)
			}
			if resp, _ := post(t, ts.URL+"/api/v1/personas/next?profile=full"); resp.StatusCode != http.StatusConflict {
				t.Fatalf("want 409, got %d", resp.StatusCode)
			}
			if resp, ps := post(t, ts.URL+"/api/v1/personas/next?profile=full&loop=true"); resp.StatusCode != 200 || ps[0].ID != ps3[0].ID {
				t.Fatalf("loop: status=%d", resp.StatusCode)
			}
			for _, bad := range []string{"count=0", "count=abc", "with_kyc=maybe", "profile=../x", "set=unknown=1"} {
				if resp, _ := post(t, ts.URL+"/api/v1/personas/next?"+bad); resp.StatusCode != 400 {
					t.Errorf("%s: want 400, got %d", bad, resp.StatusCode)
				}
			}
			if resp, err := http.Get(ts.URL + "/api/v1/personas/next"); err != nil || resp.StatusCode != http.StatusMethodNotAllowed {
				t.Fatalf("GET must be 405")
			}
		})
	}
}

func TestSetOverrideViaQuery(t *testing.T) {
	ts := httptest.NewServer(New(model.DefaultOptions(), state.NewMemoryStore()).Handler())
	defer ts.Close()
	_, ps := post(t, ts.URL+"/api/v1/personas/next?count=20&with_kyc=true&set=contact.prefecture%3D%E7%A6%8F%E5%B2%A1%E7%9C%8C")
	for _, p := range ps {
		if p.Contact.Prefecture != "福岡県" || (*p.KYC.DriversLicenseNumber)[:2] != "90" {
			t.Fatalf("%+v", p.Contact)
		}
	}
}

// 並行リクエストでも払い出しが重複しないこと（オンメモリ / ファイル永続の両方）。
func TestConcurrentNoDuplicates(t *testing.T) {
	for name, alloc := range map[string]state.Allocator{
		"memory": state.NewMemoryStore(),
		"file":   state.NewFileStore(t.TempDir()),
	} {
		t.Run(name, func(t *testing.T) {
			ts := httptest.NewServer(New(model.DefaultOptions(), alloc).Handler())
			defer ts.Close()
			const workers, reqs, count = 16, 10, 25
			var mu sync.Mutex
			ids := map[uint64]bool{}
			emails := map[string]bool{}
			var wg sync.WaitGroup
			for w := 0; w < workers; w++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for i := 0; i < reqs; i++ {
						resp, err := http.Post(fmt.Sprintf("%s/api/v1/personas/next?count=%d&profile=par", ts.URL, count), "", nil)
						if err != nil {
							t.Error(err)
							return
						}
						var ps []model.Persona
						err = json.NewDecoder(resp.Body).Decode(&ps)
						resp.Body.Close()
						if err != nil {
							t.Error(err)
							return
						}
						mu.Lock()
						for _, p := range ps {
							if ids[p.ID] || emails[p.Contact.Email] {
								t.Errorf("duplicate persona id=%d", p.ID)
							}
							ids[p.ID], emails[p.Contact.Email] = true, true
						}
						mu.Unlock()
					}
				}()
			}
			wg.Wait()
			if len(ids) != workers*reqs*count {
				t.Fatalf("got %d unique, want %d", len(ids), workers*reqs*count)
			}
		})
	}
}

func TestSetRawViaQuery(t *testing.T) {
	ts := httptest.NewServer(New(model.DefaultOptions(), state.NewMemoryStore()).Handler())
	defer ts.Close()
	_, ps := post(t, ts.URL+"/api/v1/personas/next?count=2&with_profile=true&set=personal.age%3D17&set_raw=profile.is_married%3Dtrue")
	for _, p := range ps {
		if p.Personal.Age != 17 || !p.Profile.IsMarried {
			t.Fatalf("%+v %+v", p.Personal, p.Profile)
		}
	}
}
