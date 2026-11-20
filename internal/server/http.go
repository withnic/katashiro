// Package server は E2E テスト向けの HTTP API（katashiro serve）を提供する。
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/withnic/katashiro/internal/engine"
	"github.com/withnic/katashiro/internal/exporter"
	"github.com/withnic/katashiro/internal/model"
	"github.com/withnic/katashiro/internal/state"
)

// Server は払い出し API のハンドラ。
type Server struct {
	base  model.Options
	alloc state.Allocator
}

// New は base（seed / base-date / 辞書など）と払い出しカーソル実装から Server を作る。
func New(base model.Options, alloc state.Allocator) *Server {
	return &Server{base: base, alloc: alloc}
}

// Handler はルーティング済みの http.Handler を返す。
//
//	POST /api/v1/personas/next?count=10&profile=e2e&with_kyc=true&with_profile=true&with_finance=true&loop=true&set=k=v&set_raw=k=v
//	POST /api/v1/personas/reset?profile=e2e
//	GET  /healthz
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/personas/next", s.handleNext)
	mux.HandleFunc("POST /api/v1/personas/reset", s.handleReset)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v) // ヘッダ送信後はクライアント切断以外で失敗しない
}

func writeError(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func boolParam(r *http.Request, name string) (bool, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return false, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %q", name, v)
	}
	return b, nil
}

func profileParam(r *http.Request) string {
	if p := r.URL.Query().Get("profile"); p != "" {
		return p
	}
	return state.DefaultProfile
}

func (s *Server) handleNext(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	count := 1
	if c := q.Get("count"); c != "" {
		n, err := strconv.Atoi(c)
		if err != nil || n < 1 || n > engine.SpaceSize {
			writeError(w, http.StatusBadRequest, fmt.Errorf("count must be 1..%d", engine.SpaceSize))
			return
		}
		count = n
	}
	opt := s.base
	var err error
	flags := []struct {
		name string
		dst  *bool
	}{{"with_finance", &opt.WithFinance}, {"with_kyc", &opt.WithKYC}, {"with_profile", &opt.WithProfile}}
	for _, f := range flags {
		if *f.dst, err = boolParam(r, f.name); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	loop, err := boolParam(r, "loop")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	opt.Sets = append(append([]string(nil), s.base.Sets...), q["set"]...)
	opt.RawSets = append(append([]string(nil), s.base.RawSets...), q["set_raw"]...)
	gen, err := model.NewGenerator(opt)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	profile := profileParam(r)
	if err := state.ValidateProfile(profile); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	start, err := s.alloc.Reserve(profile, count, loop)
	switch {
	case errors.Is(err, state.ErrExhausted):
		writeError(w, http.StatusConflict, err)
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Katashiro-Cursor-Start", strconv.FormatUint(uint64(start), 10))
	out := exporter.NewJSON(w)
	for k := 0; k < count; k++ {
		idx := uint32((uint64(start) + uint64(k)) % engine.SpaceSize)
		if err := out.Write(gen.At(idx)); err != nil {
			return // クライアント切断
		}
	}
	out.Close()
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	profile := profileParam(r)
	if err := s.alloc.Reset(profile); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": profile, "cursor": 0})
}
