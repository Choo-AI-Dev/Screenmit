// Package fakedrive is an in-memory stand-in for the parts of Google OAuth
// and Drive v3 that Screenmit uses. Tests and the dev build talk to it.
package fakedrive

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type file struct {
	ID          string
	Name        string
	Mime        string
	Parents     []string
	Data        []byte
	Created     time.Time
	Modified    time.Time
	Description string
	Props       map[string]string
	Trashed     bool
	hiddenUntil time.Time
}

// Server is an http.Handler.
type Server struct {
	mu      sync.Mutex
	files   map[string]*file
	seq     int
	codes   map[string]string // code -> challenge
	access  map[string]bool
	refresh map[string]bool
	uploads map[string]*file

	Email string
	// PageSize caps list pages (to test paging).
	PageSize int
	// SearchLag hides new files from list results for a while.
	SearchLag time.Duration
	// Requests counts calls by "METHOD /path".
	Requests map[string]int
}

// New makes an empty Drive.
func New() *Server {
	return &Server{
		files: map[string]*file{}, codes: map[string]string{}, access: map[string]bool{},
		refresh: map[string]bool{}, uploads: map[string]*file{}, Email: "choo@example.com",
		PageSize: 1000, Requests: map[string]int{},
	}
}

func (s *Server) id(prefix string) string {
	s.seq++
	return fmt.Sprintf("%s%04d", prefix, s.seq)
}

// ExpireAccess makes every access token invalid (refresh still works).
func (s *Server) ExpireAccess() {
	s.mu.Lock()
	s.access = map[string]bool{}
	s.mu.Unlock()
}

// RevokeAll makes the saved login stop working.
func (s *Server) RevokeAll() {
	s.mu.Lock()
	s.access = map[string]bool{}
	s.refresh = map[string]bool{}
	s.mu.Unlock()
}

// Files returns copies of non-trashed files matching kind (appProperties screenmit=kind; "" for all).
func (s *Server) Files(kind string) []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []map[string]any
	for _, f := range s.sorted() {
		if f.Trashed || (kind != "" && f.Props["screenmit"] != kind) {
			continue
		}
		out = append(out, s.meta(f))
	}
	return out
}

// Content returns a file's bytes.
func (s *Server) Content(id string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f := s.files[id]; f != nil {
		return append([]byte(nil), f.Data...)
	}
	return nil
}

// Put adds a file directly (to fake another PC).
func (s *Server) Put(name, mimeType string, parents []string, props map[string]string, desc string, data []byte) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := &file{ID: s.id("f"), Name: name, Mime: mimeType, Parents: parents, Props: props, Description: desc, Data: data, Created: time.Now(), Modified: time.Now()}
	s.files[f.ID] = f
	return f.ID
}

// SetProps changes appProperties as another PC would.
func (s *Server) SetProps(id string, props map[string]string, desc *string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f := s.files[id]; f != nil {
		for k, v := range props {
			f.Props[k] = v
		}
		if desc != nil {
			f.Description = *desc
		}
	}
}

// Remove deletes a file as another PC would.
func (s *Server) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteTree(id)
}

func (s *Server) sorted() []*file {
	fs := make([]*file, 0, len(s.files))
	for _, f := range s.files {
		fs = append(fs, f)
	}
	sort.Slice(fs, func(i, j int) bool { return fs[i].ID < fs[j].ID })
	return fs
}

func (s *Server) meta(f *file) map[string]any {
	m := map[string]any{
		"id": f.ID, "name": f.Name, "mimeType": f.Mime, "parents": f.Parents,
		"createdTime":  f.Created.UTC().Format(time.RFC3339Nano),
		"modifiedTime": f.Modified.UTC().Format(time.RFC3339Nano),
		"trashed":      f.Trashed,
	}
	if f.Mime != "application/vnd.google-apps.folder" {
		m["size"] = strconv.Itoa(len(f.Data))
	}
	if f.Description != "" {
		m["description"] = f.Description
	}
	if len(f.Props) > 0 {
		m["appProperties"] = f.Props
	}
	return m
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Upload-Content-Type, X-Upload-Content-Length")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE")
	w.Header().Set("Access-Control-Expose-Headers", "Location")
	if r.Method == "OPTIONS" {
		w.WriteHeader(204)
		return
	}
	s.mu.Lock()
	s.Requests[r.Method+" "+pathKey(r.URL.Path)]++
	s.mu.Unlock()

	switch {
	case r.URL.Path == "/o/oauth2/auth":
		s.authorize(w, r)
		return
	case r.URL.Path == "/token":
		s.token(w, r)
		return
	case r.URL.Path == "/revoke":
		s.mu.Lock()
		delete(s.access, r.URL.Query().Get("token"))
		s.mu.Unlock()
		w.WriteHeader(200)
		return
	}
	if !s.authed(r) {
		jsonErr(w, 401, "authError", "Invalid Credentials")
		return
	}
	p := r.URL.Path
	switch {
	case p == "/drive/v3/about":
		writeJSON(w, map[string]any{"user": map[string]any{"emailAddress": s.Email}})
	case p == "/drive/v3/files" && r.Method == "GET":
		s.list(w, r)
	case p == "/drive/v3/files" && r.Method == "POST":
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		s.create(w, m, nil)
	case strings.HasPrefix(p, "/drive/v3/files/"):
		s.one(w, r, strings.TrimPrefix(p, "/drive/v3/files/"))
	case p == "/upload/drive/v3/files" && r.Method == "POST":
		s.upload(w, r)
	case strings.HasPrefix(p, "/upload/drive/v3/files/") && r.Method == "PATCH":
		s.mu.Lock()
		f := s.files[strings.TrimPrefix(p, "/upload/drive/v3/files/")]
		if f == nil {
			s.mu.Unlock()
			jsonErr(w, 404, "notFound", "File not found")
			return
		}
		f.Data, _ = io.ReadAll(r.Body)
		f.Modified = time.Now()
		m := s.meta(f)
		s.mu.Unlock()
		writeJSON(w, m)
	case strings.HasPrefix(p, "/upload/session/") && r.Method == "PUT":
		s.mu.Lock()
		f := s.uploads[strings.TrimPrefix(p, "/upload/session/")]
		s.mu.Unlock()
		if f == nil {
			jsonErr(w, 404, "notFound", "no session")
			return
		}
		data, _ := io.ReadAll(r.Body)
		s.finishCreate(w, f, data)
	default:
		jsonErr(w, 404, "notFound", "no route "+p)
	}
}

func pathKey(p string) string {
	for _, pre := range []string{"/drive/v3/files/", "/upload/drive/v3/files/", "/upload/session/"} {
		if strings.HasPrefix(p, pre) {
			return pre + "*"
		}
	}
	return p
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("response_type") == "token" {
		// Browser sign-in (web page): the token comes back in the #fragment.
		if q.Get("client_id") == "" || q.Get("scope") == "" {
			http.Error(w, "bad request", 400)
			return
		}
		s.mu.Lock()
		at := s.id("at")
		s.access[at] = true
		s.mu.Unlock()
		v := url.Values{"access_token": {at}, "token_type": {"Bearer"}, "expires_in": {"3599"}, "state": {q.Get("state")}, "scope": {q.Get("scope")}}
		http.Redirect(w, r, q.Get("redirect_uri")+"#"+v.Encode(), http.StatusFound)
		return
	}
	if q.Get("code_challenge_method") != "S256" || q.Get("scope") == "" {
		http.Error(w, "bad request", 400)
		return
	}
	s.mu.Lock()
	code := s.id("code")
	s.codes[code] = q.Get("code_challenge")
	s.mu.Unlock()
	u, _ := url.Parse(q.Get("redirect_uri"))
	v := url.Values{"code": {code}, "state": {q.Get("state")}}
	u.RawQuery = v.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Form.Get("client_secret") == "" {
		jsonErr(w, 401, "invalid_client", "")
		return
	}
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		ch, ok := s.codes[r.Form.Get("code")]
		delete(s.codes, r.Form.Get("code"))
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != ch {
			w.WriteHeader(400)
			writeJSON(w, map[string]any{"error": "invalid_grant"})
			return
		}
		at, rt := s.id("at"), s.id("rt")
		s.access[at], s.refresh[rt] = true, true
		writeJSON(w, map[string]any{"access_token": at, "refresh_token": rt, "expires_in": 3600})
	case "refresh_token":
		if !s.refresh[r.Form.Get("refresh_token")] {
			w.WriteHeader(400)
			writeJSON(w, map[string]any{"error": "invalid_grant"})
			return
		}
		at := s.id("at")
		s.access[at] = true
		writeJSON(w, map[string]any{"access_token": at, "expires_in": 3600})
	default:
		w.WriteHeader(400)
		writeJSON(w, map[string]any{"error": "unsupported_grant_type"})
	}
}

func (s *Server) authed(r *http.Request) bool {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if tok == "" {
		tok = r.URL.Query().Get("access_token")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.access[tok]
}

var (
	reParent = regexp.MustCompile(`'([^']+)' in parents`)
	reProp   = regexp.MustCompile(`appProperties has \{ key='([^']*)' and value='([^']*)' \}`)
	reTrash  = regexp.MustCompile(`trashed = (true|false)`)
	reName   = regexp.MustCompile(`name = '([^']*)'`)
)

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	parent := ""
	if m := reParent.FindStringSubmatch(q); m != nil {
		parent = m[1]
	}
	props := reProp.FindAllStringSubmatch(q, -1)
	trash := "false"
	if m := reTrash.FindStringSubmatch(q); m != nil {
		trash = m[1]
	}
	name := ""
	if m := reName.FindStringSubmatch(q); m != nil {
		name = m[1]
	}
	s.mu.Lock()
	var hits []map[string]any
	now := time.Now()
	for _, f := range s.sorted() {
		if parent != "" && !contains(f.Parents, parent) {
			continue
		}
		if (trash == "true") != f.Trashed {
			continue
		}
		if name != "" && f.Name != name {
			continue
		}
		ok := true
		for _, p := range props {
			if f.Props[p[1]] != p[2] {
				ok = false
			}
		}
		if !ok || now.Before(f.hiddenUntil) {
			continue
		}
		hits = append(hits, s.meta(f))
	}
	size := s.PageSize
	s.mu.Unlock()
	start, _ := strconv.Atoi(r.URL.Query().Get("pageToken"))
	end := min(len(hits), start+size)
	out := map[string]any{"files": hits[start:end]}
	if end < len(hits) {
		out["nextPageToken"] = strconv.Itoa(end)
	}
	if hits == nil {
		out["files"] = []any{}
	}
	writeJSON(w, out)
}

func contains(a []string, s string) bool {
	for _, x := range a {
		if x == s {
			return true
		}
	}
	return false
}

func (s *Server) newFile(m map[string]any) *file {
	f := &file{Props: map[string]string{}}
	f.Name, _ = m["name"].(string)
	f.Mime, _ = m["mimeType"].(string)
	f.Description, _ = m["description"].(string)
	if ps, ok := m["parents"].([]any); ok {
		for _, p := range ps {
			f.Parents = append(f.Parents, fmt.Sprint(p))
		}
	}
	if pr, ok := m["appProperties"].(map[string]any); ok {
		for k, v := range pr {
			f.Props[k] = fmt.Sprint(v)
		}
	}
	return f
}

func (s *Server) create(w http.ResponseWriter, m map[string]any, data []byte) {
	s.finishCreate(w, s.newFile(m), data)
}

func (s *Server) finishCreate(w http.ResponseWriter, f *file, data []byte) {
	s.mu.Lock()
	for _, p := range f.Parents {
		if pf := s.files[p]; pf == nil || pf.Trashed {
			s.mu.Unlock()
			jsonErr(w, 404, "notFound", "File not found: "+p)
			return
		}
	}
	f.ID = s.id("f")
	f.Data = data
	f.Created, f.Modified = time.Now(), time.Now()
	if s.SearchLag > 0 {
		f.hiddenUntil = time.Now().Add(s.SearchLag)
	}
	s.files[f.ID] = f
	m := s.meta(f)
	s.mu.Unlock()
	writeJSON(w, m)
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Query().Get("uploadType") {
	case "multipart":
		mt, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mt != "multipart/related" {
			jsonErr(w, 400, "badRequest", "want multipart/related")
			return
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		var meta map[string]any
		var data []byte
		for i := 0; ; i++ {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			b, _ := io.ReadAll(part)
			if i == 0 {
				_ = json.Unmarshal(b, &meta)
			} else {
				data = b
			}
		}
		if meta == nil {
			jsonErr(w, 400, "badRequest", "no metadata")
			return
		}
		s.create(w, meta, data)
	case "resumable":
		var meta map[string]any
		_ = json.NewDecoder(r.Body).Decode(&meta)
		s.mu.Lock()
		sid := s.id("up")
		s.uploads[sid] = s.newFile(meta)
		s.mu.Unlock()
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		w.Header().Set("Location", scheme+"://"+r.Host+"/upload/session/"+sid)
		w.WriteHeader(200)
	default:
		jsonErr(w, 400, "badRequest", "uploadType")
	}
}

func (s *Server) one(w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	f := s.files[id]
	if f == nil {
		s.mu.Unlock()
		jsonErr(w, 404, "notFound", "File not found: "+id)
		return
	}
	switch r.Method {
	case "GET":
		if r.URL.Query().Get("alt") == "media" {
			data := append([]byte(nil), f.Data...)
			s.mu.Unlock()
			w.Header().Set("Content-Type", f.Mime)
			w.Write(data)
			return
		}
		m := s.meta(f)
		s.mu.Unlock()
		writeJSON(w, m)
	case "PATCH":
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		if pr, ok := m["appProperties"].(map[string]any); ok {
			for k, v := range pr {
				if v == nil {
					delete(f.Props, k)
				} else {
					f.Props[k] = fmt.Sprint(v)
				}
			}
		}
		if d, ok := m["description"].(string); ok {
			f.Description = d
		}
		if t, ok := m["trashed"].(bool); ok {
			f.Trashed = t
		}
		f.Modified = time.Now()
		out := s.meta(f)
		s.mu.Unlock()
		writeJSON(w, out)
	case "DELETE":
		s.deleteTree(id)
		s.mu.Unlock()
		w.WriteHeader(204)
	default:
		s.mu.Unlock()
		jsonErr(w, 405, "method", r.Method)
	}
}

func (s *Server) deleteTree(id string) {
	delete(s.files, id)
	for cid, c := range s.files {
		if contains(c.Parents, id) {
			s.deleteTree(cid)
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	_ = json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, code int, reason, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": msg, "errors": []any{map[string]any{"reason": reason}}}})
}
