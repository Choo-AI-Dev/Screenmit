// Package server is the local web server behind the gallery window. It only
// listens on 127.0.0.1 and every data request needs the per-install cookie.
package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Choo-AI-Dev/screenmit/internal/shots"
)

// SettingsPatch is a partial settings change from the gallery.
type SettingsPatch struct {
	RetentionDays *int    `json:"retentionDays,omitempty"`
	SkipDocs      *bool   `json:"skipDocs,omitempty"`
	Autostart     *bool   `json:"autostart,omitempty"`
	Notify        *bool   `json:"notify,omitempty"`
	Hotkey        *string `json:"hotkey,omitempty"`
	CaptureKey    *string `json:"captureKey,omitempty"`
	CaptureMode   *string `json:"captureMode,omitempty"`
	PCName        *string `json:"pcName,omitempty"`
	Folder        *string `json:"folder,omitempty"`
	Welcomed      *bool   `json:"welcomed,omitempty"`
}

// Backend is what the app gives the server.
type Backend interface {
	Store() *shots.Store
	// State is everything except the shot list (settings, status…).
	State() map[string]any
	SetPaused(bool)
	ApplySettings(SettingsPatch) error
	// ImageFile returns a local file for the picture, downloading another
	// PC's shot from Drive when needed.
	ImageFile(ctx context.Context, sh shots.Shot) (string, error)
	CopyImage(ctx context.Context, sh shots.Shot) error
	Reveal(sh shots.Shot) error
	OpenFolder() error
	OpenURL(u string) error
	// Edited is called after a shot changed through the gallery.
	Edited()
	DriveConnect() error
	DriveDisconnect() error
	DriveSync()
	DriveCredentials([]byte) error
}

// Server serves the gallery.
type Server struct {
	b     Backend
	token string
	web   fs.FS
	mux   *http.ServeMux
	port  int

	mu   sync.Mutex
	subs map[chan struct{}]bool

	// origins may read the API from another site (the web page on
	// GitHub Pages) with "Authorization: Bearer <token>".
	origins map[string]bool
}

// New builds the server; call Listen then Serve. webOrigins are sites
// (like "https://choo-ai-dev.github.io") allowed to use the API with the token.
func New(b Backend, token string, web fs.FS, webOrigins ...string) *Server {
	s := &Server{b: b, token: token, web: web, mux: http.NewServeMux(), subs: map[chan struct{}]bool{}, origins: map[string]bool{}}
	for _, o := range webOrigins {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			s.origins[o] = true
		}
	}
	s.routes()
	return s
}

// Listen binds 127.0.0.1, trying want and the next 20 ports.
func (s *Server) Listen(want int) (net.Listener, error) {
	var last error
	for p := want; p < want+20; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			s.port = p
			return ln, nil
		}
		last = err
	}
	return nil, last
}

// Handle adds a data route (cookie and change checks apply). The dev build
// uses it for test hooks.
func (s *Server) Handle(pattern string, h http.HandlerFunc) { s.mux.HandleFunc(pattern, s.api(h)) }

// Port is the bound port.
func (s *Server) Port() int { return s.port }

// OpenURL is the link that logs the gallery in.
func (s *Server) OpenURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/open?k=%s", s.port, s.token)
}

// Serve runs until the listener closes.
func (s *Server) Serve(ln net.Listener) error {
	srv := &http.Server{Handler: s, ReadHeaderTimeout: 10 * time.Second}
	return srv.Serve(ln)
}

// Broadcast tells open galleries that something changed.
func (s *Server) Broadcast() {
	s.mu.Lock()
	for c := range s.subs {
		select {
		case c <- struct{}{}:
		default:
		}
	}
	s.mu.Unlock()
}

// Watchers is how many gallery windows are connected.
func (s *Server) Watchers() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.subs)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// DNS rebinding guard: only our own host names.
	host := r.Host
	if h, p, err := net.SplitHostPort(host); err == nil {
		if (h != "127.0.0.1" && h != "localhost") || p != fmt.Sprint(s.port) {
			http.Error(w, "bad host", http.StatusForbidden)
			return
		}
	} else {
		http.Error(w, "bad host", http.StatusForbidden)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if o := r.Header.Get("Origin"); o != "" && s.origins[o] {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", o)
		h.Add("Vary", "Origin")
		if r.Method == "OPTIONS" {
			h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Screenmit")
			h.Set("Access-Control-Allow-Private-Network", "true") // Chrome: public site -> this PC
			h.Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) authed(r *http.Request) bool {
	if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
		return subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(a, "Bearer ")), []byte(s.token)) == 1
	}
	c, err := r.Cookie("screenmit")
	return err == nil && subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.token)) == 1
}

// api wraps data routes: cookie required; changes also need our header and
// a same-origin Origin (so other web pages can't trigger them).
func (s *Server) api(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authed(r) {
			jsonError(w, http.StatusUnauthorized, "다시 열어 주세요")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if r.Header.Get("X-Screenmit") != "1" {
				jsonError(w, http.StatusForbidden, "bad request")
				return
			}
			if o := r.Header.Get("Origin"); o != "" && !s.origins[o] {
				u, err := url.Parse(o)
				if err != nil || u.Host != r.Host {
					jsonError(w, http.StatusForbidden, "bad origin")
					return
				}
			}
		}
		h(w, r)
	}
}

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /open", func(w http.ResponseWriter, r *http.Request) {
		k := r.URL.Query().Get("k")
		if subtle.ConstantTimeCompare([]byte(k), []byte(s.token)) != 1 {
			http.Error(w, "링크가 맞지 않아요. 트레이의 Screenmit 아이콘으로 열어 주세요.", http.StatusForbidden)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "screenmit", Value: s.token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 400 * 24 * 3600})
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})
	m.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		s.static(w, r, "index.html")
	})
	m.HandleFunc("GET /assets/{name}", func(w http.ResponseWriter, r *http.Request) {
		s.static(w, r, r.PathValue("name"))
	})

	m.HandleFunc("GET /api/state", s.api(s.state))
	m.HandleFunc("GET /api/events", s.api(s.events))
	m.HandleFunc("GET /img/{id}", s.api(s.image))
	m.HandleFunc("GET /thumb/{id}", s.api(s.thumb))
	m.HandleFunc("PATCH /api/shots/{id}", s.api(s.patchShot))
	m.HandleFunc("DELETE /api/shots/{id}", s.api(s.deleteShot))
	m.HandleFunc("POST /api/shots/{id}/copy", s.api(func(w http.ResponseWriter, r *http.Request) {
		sh, ok := s.shot(w, r)
		if ok {
			reply(w, s.b.CopyImage(r.Context(), sh))
		}
	}))
	m.HandleFunc("POST /api/shots/{id}/reveal", s.api(func(w http.ResponseWriter, r *http.Request) {
		sh, ok := s.shot(w, r)
		if ok {
			reply(w, s.b.Reveal(sh))
		}
	}))
	m.HandleFunc("POST /api/folder/open", s.api(func(w http.ResponseWriter, r *http.Request) { reply(w, s.b.OpenFolder()) }))
	m.HandleFunc("POST /api/open-url", s.api(func(w http.ResponseWriter, r *http.Request) {
		var in struct{ URL string }
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || !strings.HasPrefix(in.URL, "https://") {
			jsonError(w, 400, "bad url")
			return
		}
		reply(w, s.b.OpenURL(in.URL))
	}))
	m.HandleFunc("POST /api/settings", s.api(func(w http.ResponseWriter, r *http.Request) {
		var p SettingsPatch
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			jsonError(w, 400, "bad json")
			return
		}
		if err := s.b.ApplySettings(p); err != nil {
			jsonError(w, 400, err.Error())
			return
		}
		s.Broadcast()
		reply(w, nil)
	}))
	m.HandleFunc("POST /api/pause", s.api(func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Paused bool }
		_ = json.NewDecoder(r.Body).Decode(&in)
		s.b.SetPaused(in.Paused)
		s.Broadcast()
		reply(w, nil)
	}))
	m.HandleFunc("POST /api/drive/connect", s.api(func(w http.ResponseWriter, r *http.Request) { reply(w, s.b.DriveConnect()) }))
	m.HandleFunc("POST /api/drive/disconnect", s.api(func(w http.ResponseWriter, r *http.Request) { reply(w, s.b.DriveDisconnect()) }))
	m.HandleFunc("POST /api/drive/sync", s.api(func(w http.ResponseWriter, r *http.Request) { s.b.DriveSync(); reply(w, nil) }))
	m.HandleFunc("POST /api/drive/credentials", s.api(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
		if err == nil {
			err = s.b.DriveCredentials(b)
		}
		if err != nil {
			jsonError(w, 400, err.Error())
			return
		}
		s.Broadcast()
		reply(w, nil)
	}))
}

func (s *Server) static(w http.ResponseWriter, r *http.Request, name string) {
	b, err := fs.ReadFile(s.web, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch filepath.Ext(name) {
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' blob: data:; style-src 'self' 'unsafe-inline'; connect-src 'self'")
	case ".js":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".ico":
		w.Header().Set("Content-Type", "image/x-icon")
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(b)
}

// ShotView is a shot as the gallery sees it.
type ShotView struct {
	ID     string `json:"id"`
	Day    string `json:"day"`
	Time   string `json:"time"`
	W      int    `json:"w"`
	H      int    `json:"h"`
	Bytes  int64  `json:"bytes"`
	App    string `json:"app,omitempty"`
	Exe    string `json:"exe,omitempty"`
	Title  string `json:"title,omitempty"`
	Note   string `json:"note,omitempty"`
	Pinned bool   `json:"pinned"`
	PC     string `json:"pc,omitempty"`
	Remote bool   `json:"remote,omitempty"`
	Synced bool   `json:"synced,omitempty"`
	Name   string `json:"name"`
}

func view(sh shots.Shot) ShotView {
	name := filepath.Base(filepath.FromSlash(sh.File))
	if sh.File == "" {
		name = "Screenmit_" + sh.Time.Local().Format("2006-01-02_15-04-05") + ".png"
	}
	return ShotView{
		ID: sh.ID, Day: sh.Time.Local().Format("2006-01-02"), Time: sh.Time.Local().Format(time.RFC3339),
		W: sh.W, H: sh.H, Bytes: sh.Bytes, App: sh.App, Exe: sh.Exe, Title: sh.Title, Note: sh.Note,
		Pinned: sh.Pinned, PC: sh.PC, Remote: sh.Remote, Synced: sh.DriveID != "", Name: name,
	}
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	st := s.b.Store()
	list := st.List()
	out := s.b.State()
	views := make([]ShotView, len(list))
	for i, sh := range list {
		views[i] = view(sh)
	}
	count, bytes, local := st.Stats()
	out["shots"] = views
	out["stats"] = map[string]any{"count": count, "bytes": bytes, "local": local}
	out["today"] = time.Now().Format("2006-01-02")
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, out)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no stream", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	c := make(chan struct{}, 1)
	s.mu.Lock()
	s.subs[c] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.subs, c)
		s.mu.Unlock()
	}()
	fmt.Fprint(w, "retry: 2000\n\nevent: hello\ndata: 1\n\n")
	fl.Flush()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-c:
			fmt.Fprint(w, "event: change\ndata: 1\n\n")
			fl.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

func (s *Server) shot(w http.ResponseWriter, r *http.Request) (shots.Shot, bool) {
	sh, ok := s.b.Store().Get(r.PathValue("id"))
	if !ok {
		jsonError(w, 404, "없는 스크린샷이에요")
	}
	return sh, ok
}

func (s *Server) image(w http.ResponseWriter, r *http.Request) {
	sh, ok := s.shot(w, r)
	if !ok {
		return
	}
	p, err := s.b.ImageFile(r.Context(), sh)
	if err != nil {
		log.Printf("image %s: %v", sh.ID, err)
		jsonError(w, http.StatusBadGateway, "원본을 가져오지 못했어요")
		return
	}
	f, err := os.Open(p)
	if err != nil {
		jsonError(w, 404, "파일이 없어요")
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	if r.URL.Query().Get("download") == "1" {
		name := view(sh).Name
		w.Header().Set("Content-Disposition", "attachment; filename=\"screenmit.png\"; filename*=UTF-8''"+url.PathEscape(name))
	}
	http.ServeContent(w, r, "", st.ModTime(), f)
}

func (s *Server) thumb(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.b.Store().EnsureThumb(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, p)
}

func (s *Server) patchShot(w http.ResponseWriter, r *http.Request) {
	sh, ok := s.shot(w, r)
	if !ok {
		return
	}
	var in struct {
		Pinned *bool   `json:"pinned"`
		Note   *string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		jsonError(w, 400, "bad json")
		return
	}
	st := s.b.Store()
	if in.Note != nil {
		n := *in.Note
		if len([]rune(n)) > 500 {
			n = string([]rune(n)[:500])
		}
		if err := st.SetNote(sh.ID, n, true); err != nil {
			jsonError(w, 500, err.Error())
			return
		}
	}
	if in.Pinned != nil {
		if err := st.SetPinned(sh.ID, *in.Pinned, true); err != nil {
			jsonError(w, 500, err.Error())
			return
		}
	}
	s.b.Edited()
	reply(w, nil)
}

func (s *Server) deleteShot(w http.ResponseWriter, r *http.Request) {
	err := s.b.Store().Delete(r.PathValue("id"))
	if errors.Is(err, shots.ErrNotFound) {
		err = nil // already gone (deleted twice, or on another PC)
	}
	if err == nil {
		s.b.Edited()
	}
	reply(w, err)
}

func reply(w http.ResponseWriter, err error) {
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": msg})
}
