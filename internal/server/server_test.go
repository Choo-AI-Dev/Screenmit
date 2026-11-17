package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Choo-AI-Dev/screenmit/internal/shots"
)

type fakeBackend struct {
	st     *shots.Store
	paused bool
}

func (f *fakeBackend) Store() *shots.Store               { return f.st }
func (f *fakeBackend) State() map[string]any             { return map[string]any{"paused": f.paused} }
func (f *fakeBackend) SetPaused(p bool)                  { f.paused = p }
func (f *fakeBackend) ApplySettings(SettingsPatch) error { return nil }
func (f *fakeBackend) ImageFile(context.Context, shots.Shot) (string, error) {
	return "", shots.ErrNotFound
}
func (f *fakeBackend) CopyImage(context.Context, shots.Shot) error { return nil }
func (f *fakeBackend) Reveal(shots.Shot) error                     { return nil }
func (f *fakeBackend) OpenFolder() error                           { return nil }
func (f *fakeBackend) OpenURL(string) error                        { return nil }
func (f *fakeBackend) Edited()                                     {}
func (f *fakeBackend) DriveConnect() error                         { return nil }
func (f *fakeBackend) DriveDisconnect() error                      { return nil }
func (f *fakeBackend) DriveSync()                                  {}
func (f *fakeBackend) DriveCredentials([]byte) error               { return nil }

const tok = "0123456789abcdef0123456789abcdef0123456789abcdef"
const web = "https://choo-ai-dev.github.io"

func newTest(t *testing.T) (*Server, *fakeBackend) {
	st, err := shots.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b := &fakeBackend{st: st}
	s := New(b, tok, fstest.MapFS{"index.html": {Data: []byte("<html>")}}, web)
	s.port = 27839
	return s, b
}

func do(s *Server, method, path string, hdr map[string]string, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Host = "127.0.0.1:27839"
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestWebPageWithToken(t *testing.T) {
	s, b := newTest(t)

	// preflight from the web page
	w := do(s, "OPTIONS", "/api/pause", map[string]string{"Origin": web, "Access-Control-Request-Method": "POST", "Access-Control-Request-Private-Network": "true"}, "")
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != web || w.Header().Get("Access-Control-Allow-Private-Network") != "true" {
		t.Fatalf("preflight %d %v", w.Code, w.Header())
	}
	if !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Fatal("Authorization not allowed")
	}

	// reading with the token
	w = do(s, "GET", "/api/state", map[string]string{"Origin": web, "Authorization": "Bearer " + tok}, "")
	if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != web {
		t.Fatalf("state %d %v", w.Code, w.Header())
	}
	// a change with the token and our header
	w = do(s, "POST", "/api/pause", map[string]string{"Origin": web, "Authorization": "Bearer " + tok, "X-Screenmit": "1"}, `{"paused":true}`)
	if w.Code != 200 || !b.paused {
		t.Fatalf("pause %d", w.Code)
	}
}

func TestOtherSitesAndBadTokens(t *testing.T) {
	s, b := newTest(t)
	evil := "https://evil.example"

	w := do(s, "OPTIONS", "/api/state", map[string]string{"Origin": evil, "Access-Control-Request-Method": "GET"}, "")
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("other site got CORS")
	}
	// even with a stolen token, a change from another site is refused
	w = do(s, "POST", "/api/pause", map[string]string{"Origin": evil, "Authorization": "Bearer " + tok, "X-Screenmit": "1"}, `{"paused":true}`)
	if w.Code != 403 || b.paused {
		t.Fatalf("evil change %d", w.Code)
	}
	// wrong or missing token
	for _, h := range []map[string]string{{"Origin": web, "Authorization": "Bearer nope"}, {"Origin": web}} {
		if w := do(s, "GET", "/api/state", h, ""); w.Code != 401 {
			t.Fatalf("no token: %d", w.Code)
		}
	}
	// the page's own cookie still works, other hosts don't
	w = do(s, "GET", "/api/state", map[string]string{"Cookie": "screenmit=" + tok}, "")
	if w.Code != 200 {
		t.Fatalf("cookie %d", w.Code)
	}
	r := httptest.NewRequest("GET", "/api/state", nil)
	r.Host = "evil.example:27839"
	r.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, r)
	if rec.Code != 403 {
		t.Fatalf("rebinding %d", rec.Code)
	}
	// changes without our header are refused even with the token
	w = do(s, "POST", "/api/pause", map[string]string{"Origin": web, "Authorization": "Bearer " + tok}, `{"paused":true}`)
	if w.Code != 403 {
		t.Fatalf("no header %d", w.Code)
	}
}
