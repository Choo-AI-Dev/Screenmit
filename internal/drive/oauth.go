// Package drive syncs the collection through the user's own Google Drive
// (drive.file scope: the app only sees the files it made).
package drive

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Scope is the only permission we ask for.
const Scope = "https://www.googleapis.com/auth/drive.file"

// ErrNeedLogin means the saved login no longer works.
var ErrNeedLogin = errors.New("need login")

// Credentials is a "Desktop app" OAuth client from Google Cloud.
type Credentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	AuthURL      string `json:"auth_uri"`
	TokenURL     string `json:"token_uri"`
}

// ParseCredentials reads the JSON file Google Cloud lets you download.
func ParseCredentials(b []byte) (Credentials, error) {
	var f struct {
		Installed *Credentials `json:"installed"`
		Web       *Credentials `json:"web"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return Credentials{}, errors.New("JSON 파일이 아니에요")
	}
	if f.Installed == nil {
		if f.Web != nil {
			return Credentials{}, errors.New("웹 애플리케이션용 클라이언트예요. 유형을 '데스크톱 앱'으로 만들어 주세요")
		}
		return Credentials{}, errors.New("구글 클라이언트 JSON 이 아니에요")
	}
	c := *f.Installed
	if c.ClientID == "" || c.ClientSecret == "" {
		return Credentials{}, errors.New("client_id 또는 client_secret 이 비어 있어요")
	}
	return c.withDefaults(), nil
}

func (c Credentials) withDefaults() Credentials {
	if c.AuthURL == "" {
		c.AuthURL = "https://accounts.google.com/o/oauth2/v2/auth"
	}
	if c.TokenURL == "" {
		c.TokenURL = "https://oauth2.googleapis.com/token"
	}
	return c
}

// Token is what we keep between runs.
type Token struct {
	Access  string    `json:"access_token"`
	Refresh string    `json:"refresh_token"`
	Expiry  time.Time `json:"expiry"`
	Email   string    `json:"email,omitempty"`
}

// TokenStore keeps the token (encrypted with the Windows user's key on Windows).
type TokenStore interface {
	Load() (Token, error)
	Save(Token) error
	Clear() error
}

type tokenResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	Description  string `json:"error_description"`
}

// Login runs the browser sign-in: a one-shot listener on 127.0.0.1 gets the
// code back, PKCE protects it. open shows the Google page in a browser.
func Login(ctx context.Context, hc *http.Client, c Credentials, open func(string) error) (Token, error) {
	c = c.withDefaults()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Token{}, err
	}
	defer ln.Close()
	redirect := fmt.Sprintf("http://127.0.0.1:%d/", ln.Addr().(*net.TCPAddr).Port)
	verifier := randString(48)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	state := randString(16)

	q := url.Values{
		"client_id":             {c.ClientID},
		"redirect_uri":          {redirect},
		"response_type":         {"code"},
		"scope":                 {Scope},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"access_type":           {"offline"},
		"prompt":                {"consent"},
		"state":                 {state},
	}
	authURL := c.AuthURL + "?" + q.Encode()

	type result struct {
		code string
		err  error
	}
	done := make(chan result, 1)
	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		v := r.URL.Query()
		var res result
		switch {
		case v.Get("state") != state:
			res.err = errors.New("로그인 응답이 맞지 않아요 (state)")
		case v.Get("error") != "":
			res.err = fmt.Errorf("구글이 거절했어요: %s", v.Get("error"))
		case v.Get("code") == "":
			res.err = errors.New("로그인 코드가 없어요")
		default:
			res.code = v.Get("code")
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		msg := "구글 드라이브에 연결했어요. 이 창은 닫아도 돼요."
		if res.err != nil {
			msg = "연결하지 못했어요: " + res.err.Error()
		}
		fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>Screenmit</title>
<body style="font-family:'Segoe UI','Malgun Gothic',sans-serif;display:grid;place-items:center;height:90vh;color:#191f28">
<div style="text-align:center"><h2 style="font-weight:600">Screenmit</h2><p>%s</p></div>`, html.EscapeString(msg))
		select {
		case done <- res:
		default:
		}
	})}
	go srv.Serve(ln)
	defer srv.Close()

	if err := open(authURL); err != nil {
		return Token{}, err
	}
	var res result
	select {
	case res = <-done:
	case <-ctx.Done():
		return Token{}, errors.New("로그인 시간이 지났어요. 다시 눌러 주세요")
	}
	if res.err != nil {
		return Token{}, res.err
	}
	tr, err := postToken(ctx, hc, c.TokenURL, url.Values{
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
		"code":          {res.code},
		"code_verifier": {verifier},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirect},
	})
	if err != nil {
		return Token{}, err
	}
	if tr.RefreshToken == "" {
		return Token{}, errors.New("구글이 오래 쓰는 로그인(refresh token)을 주지 않았어요")
	}
	return Token{Access: tr.AccessToken, Refresh: tr.RefreshToken, Expiry: expiry(tr.ExpiresIn)}, nil
}

func refresh(ctx context.Context, hc *http.Client, c Credentials, t Token) (Token, error) {
	c = c.withDefaults()
	tr, err := postToken(ctx, hc, c.TokenURL, url.Values{
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
		"refresh_token": {t.Refresh},
		"grant_type":    {"refresh_token"},
	})
	if err != nil {
		return t, err
	}
	t.Access = tr.AccessToken
	t.Expiry = expiry(tr.ExpiresIn)
	if tr.RefreshToken != "" {
		t.Refresh = tr.RefreshToken
	}
	return t, nil
}

func postToken(ctx context.Context, hc *http.Client, u string, form url.Values) (tokenResp, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", u, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResp{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := hc.Do(req)
	if err != nil {
		return tokenResp{}, err
	}
	defer resp.Body.Close()
	var tr tokenResp
	_ = json.NewDecoder(resp.Body).Decode(&tr)
	if resp.StatusCode != 200 || tr.AccessToken == "" {
		if tr.Error == "invalid_grant" || tr.Error == "invalid_client" || tr.Error == "unauthorized_client" {
			return tr, ErrNeedLogin
		}
		return tr, fmt.Errorf("token: %d %s %s", resp.StatusCode, tr.Error, tr.Description)
	}
	return tr, nil
}

func expiry(sec int) time.Time {
	if sec <= 0 {
		sec = 3600
	}
	return time.Now().Add(time.Duration(sec-60) * time.Second)
}

func randString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)[:n]
}
