package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// API is a small Drive v3 client.
type API struct {
	HTTP   *http.Client
	Base   string // "https://www.googleapis.com"
	Creds  Credentials
	Tokens TokenStore

	mu  sync.Mutex
	tok Token
}

// File is the part of a Drive file we use.
type File struct {
	ID            string            `json:"id,omitempty"`
	Name          string            `json:"name,omitempty"`
	MimeType      string            `json:"mimeType,omitempty"`
	Parents       []string          `json:"parents,omitempty"`
	Size          string            `json:"size,omitempty"`
	CreatedTime   string            `json:"createdTime,omitempty"`
	ModifiedTime  string            `json:"modifiedTime,omitempty"`
	Description   string            `json:"description,omitempty"`
	AppProperties map[string]string `json:"appProperties,omitempty"`
}

const fileFields = "id,name,size,createdTime,modifiedTime,description,appProperties"

// HTTPError is a non-2xx answer from Drive.
type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("drive: %d %s", e.Status, e.Body) }

// IsNotFound reports a 404 (already deleted, for example).
func IsNotFound(err error) bool {
	var h *HTTPError
	return errors.As(err, &h) && h.Status == 404
}

// NewAPI loads the saved token.
func NewAPI(hc *http.Client, base string, c Credentials, ts TokenStore) *API {
	a := &API{HTTP: hc, Base: strings.TrimRight(base, "/"), Creds: c.withDefaults(), Tokens: ts}
	if t, err := ts.Load(); err == nil {
		a.tok = t
	}
	return a
}

// HasLogin reports whether a refresh token is saved.
func (a *API) HasLogin() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.tok.Refresh != ""
}

// SetToken stores a fresh login.
func (a *API) SetToken(t Token) error {
	a.mu.Lock()
	a.tok = t
	a.mu.Unlock()
	return a.Tokens.Save(t)
}

// Email of the signed-in account (cached in the token).
func (a *API) Email() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.tok.Email
}

func (a *API) access(ctx context.Context, force bool) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.tok.Refresh == "" {
		return "", ErrNeedLogin
	}
	if force || a.tok.Access == "" || time.Now().After(a.tok.Expiry) {
		t, err := refresh(ctx, a.HTTP, a.Creds, a.tok)
		if err != nil {
			return "", err
		}
		a.tok = t
		_ = a.Tokens.Save(t)
	}
	return a.tok.Access, nil
}

// do sends one request, refreshing the token once on 401 and retrying
// rate limits and server errors with backoff.
func (a *API) do(ctx context.Context, method, u string, hdr http.Header, body []byte, out any) (*http.Response, error) {
	var lastErr error
	refreshed := false
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt*attempt) * 700 * time.Millisecond):
			}
		}
		tok, err := a.access(ctx, false)
		if err != nil {
			return nil, err
		}
		var rd io.Reader
		if body != nil {
			rd = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, u, rd)
		if err != nil {
			return nil, err
		}
		for k, v := range hdr {
			req.Header[k] = v
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := a.HTTP.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == 401 && !refreshed {
			resp.Body.Close()
			refreshed = true
			if _, err := a.access(ctx, true); err != nil {
				return nil, err
			}
			attempt--
			continue
		}
		if resp.StatusCode == 429 || resp.StatusCode >= 500 || (resp.StatusCode == 403 && isRateLimit(resp)) {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			resp.Body.Close()
			lastErr = &HTTPError{resp.StatusCode, string(b)}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			resp.Body.Close()
			return nil, &HTTPError{resp.StatusCode, strings.TrimSpace(string(b))}
		}
		if out != nil {
			defer resp.Body.Close()
			if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
				return nil, err
			}
		}
		return resp, nil
	}
	return nil, lastErr
}

func isRateLimit(resp *http.Response) bool {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	resp.Body = io.NopCloser(bytes.NewReader(b))
	s := string(b)
	return strings.Contains(s, "rateLimitExceeded") || strings.Contains(s, "userRateLimitExceeded")
}

func jsonHdr() http.Header {
	return http.Header{"Content-Type": {"application/json; charset=UTF-8"}}
}

// About returns the account e-mail and remembers it.
func (a *API) About(ctx context.Context) (string, error) {
	var out struct {
		User struct {
			Email string `json:"emailAddress"`
		} `json:"user"`
	}
	if _, err := a.do(ctx, "GET", a.Base+"/drive/v3/about?fields=user(emailAddress)", nil, nil, &out); err != nil {
		return "", err
	}
	a.mu.Lock()
	a.tok.Email = out.User.Email
	t := a.tok
	a.mu.Unlock()
	_ = a.Tokens.Save(t)
	return out.User.Email, nil
}

// List returns every file matching q (all pages).
func (a *API) List(ctx context.Context, q string) ([]File, error) {
	var all []File
	page := ""
	for {
		v := url.Values{
			"q":        {q},
			"fields":   {"nextPageToken,files(" + fileFields + ")"},
			"pageSize": {"1000"},
			"spaces":   {"drive"},
			"orderBy":  {"createdTime"},
		}
		if page != "" {
			v.Set("pageToken", page)
		}
		var out struct {
			Next  string `json:"nextPageToken"`
			Files []File `json:"files"`
		}
		if _, err := a.do(ctx, "GET", a.Base+"/drive/v3/files?"+v.Encode(), nil, nil, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Files...)
		if out.Next == "" {
			return all, nil
		}
		page = out.Next
	}
}

// CreateFolder makes a folder.
func (a *API) CreateFolder(ctx context.Context, f File) (File, error) {
	f.MimeType = "application/vnd.google-apps.folder"
	b, _ := json.Marshal(f)
	var out File
	_, err := a.do(ctx, "POST", a.Base+"/drive/v3/files?fields="+fileFields, jsonHdr(), b, &out)
	return out, err
}

// Upload creates a file with content. Large files go resumable.
func (a *API) Upload(ctx context.Context, meta File, mime string, data []byte) (File, error) {
	meta.MimeType = mime
	mb, _ := json.Marshal(meta)
	var out File
	if len(data) > 4<<20 {
		h := jsonHdr()
		h.Set("X-Upload-Content-Type", mime)
		h.Set("X-Upload-Content-Length", fmt.Sprint(len(data)))
		resp, err := a.do(ctx, "POST", a.Base+"/upload/drive/v3/files?uploadType=resumable&fields="+fileFields, h, mb, nil)
		if err != nil {
			return out, err
		}
		loc := resp.Header.Get("Location")
		resp.Body.Close()
		if loc == "" {
			return out, errors.New("drive: no upload location")
		}
		_, err = a.do(ctx, "PUT", loc, http.Header{"Content-Type": {mime}}, data, &out)
		return out, err
	}
	const boundary = "screenmit-part-7d2a91"
	var body bytes.Buffer
	body.WriteString("--" + boundary + "\r\nContent-Type: application/json; charset=UTF-8\r\n\r\n")
	body.Write(mb)
	body.WriteString("\r\n--" + boundary + "\r\nContent-Type: " + mime + "\r\n\r\n")
	body.Write(data)
	body.WriteString("\r\n--" + boundary + "--\r\n")
	_, err := a.do(ctx, "POST", a.Base+"/upload/drive/v3/files?uploadType=multipart&fields="+fileFields,
		http.Header{"Content-Type": {"multipart/related; boundary=" + boundary}}, body.Bytes(), &out)
	return out, err
}

// UpdateContent replaces a file's bytes.
func (a *API) UpdateContent(ctx context.Context, id, mime string, data []byte) error {
	_, err := a.do(ctx, "PATCH", a.Base+"/upload/drive/v3/files/"+url.PathEscape(id)+"?uploadType=media&fields=id",
		http.Header{"Content-Type": {mime}}, data, &File{})
	return err
}

// Patch changes metadata (appProperties keys are merged by Drive).
func (a *API) Patch(ctx context.Context, id string, meta map[string]any) error {
	b, _ := json.Marshal(meta)
	_, err := a.do(ctx, "PATCH", a.Base+"/drive/v3/files/"+url.PathEscape(id)+"?fields=id", jsonHdr(), b, &File{})
	return err
}

// Delete removes a file for good (404 counts as done).
func (a *API) Delete(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	resp, err := a.do(ctx, "DELETE", a.Base+"/drive/v3/files/"+url.PathEscape(id), nil, nil, nil)
	if err != nil {
		if IsNotFound(err) {
			return nil
		}
		return err
	}
	resp.Body.Close()
	return nil
}

// Trash moves a file to the Drive trash (404 counts as done).
func (a *API) Trash(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	err := a.Patch(ctx, id, map[string]any{"trashed": true})
	if IsNotFound(err) {
		return nil
	}
	return err
}

// Gone reports whether a file is deleted or in the trash. Used before we
// trust a missing search result, since Drive search can lag behind.
func (a *API) Gone(ctx context.Context, id string) (bool, error) {
	var out struct {
		Trashed bool `json:"trashed"`
	}
	_, err := a.do(ctx, "GET", a.Base+"/drive/v3/files/"+url.PathEscape(id)+"?fields=id,trashed", nil, nil, &out)
	if IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return out.Trashed, nil
}

// Download reads a file's bytes.
func (a *API) Download(ctx context.Context, id string) ([]byte, error) {
	resp, err := a.do(ctx, "GET", a.Base+"/drive/v3/files/"+url.PathEscape(id)+"?alt=media", nil, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 200<<20))
}
