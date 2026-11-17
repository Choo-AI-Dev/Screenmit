//go:build !windows

// The non-Windows build is for development and tests: no tray and no
// clipboard, but the same gallery, store and Drive sync, plus test hooks.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Choo-AI-Dev/screenmit/internal/drive"
	"github.com/Choo-AI-Dev/screenmit/internal/drive/fakedrive"
	"github.com/Choo-AI-Dev/screenmit/internal/imgclip"
	"github.com/Choo-AI-Dev/screenmit/internal/shots"
)

const logToStderr = true

var (
	devFakeDrive = flag.Bool("dev-fake-drive", false, "use an in-process fake Google Drive")
	devPCName    = flag.String("dev-pc", "", "hostname to report")
)

func addPlatformFlags() {}

type devPlatform struct {
	quit chan struct{}
	once sync.Once

	mu        sync.Mutex
	copied    int
	lastURL   string
	autostart bool
	hotkey    string
	notes     []string
	fake      *fakedrive.Server
	fakeURL   string
}

func newPlatform() Platform { return &devPlatform{quit: make(chan struct{})} }

func (p *devPlatform) Name() string { return "dev" }

func (p *devPlatform) ConfigDir() string {
	d, _ := os.UserConfigDir()
	return filepath.Join(d, "Screenmit")
}

func (p *devPlatform) DefaultFolder() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Pictures", "Screenmit")
}

func (p *devPlatform) PCName() string {
	if *devPCName != "" {
		return *devPCName
	}
	h, _ := os.Hostname()
	return h
}

func (p *devPlatform) Claim(bool) bool { return true }

func (p *devPlatform) Fatal(err error) { fmt.Fprintln(os.Stderr, "fatal:", err) }

func (p *devPlatform) Run(a *App) error {
	if *devFakeDrive {
		p.startFakeDrive(a)
	}
	p.devRoutes(a)
	fmt.Printf("GALLERY %s\n", a.GalleryURL())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sig:
	case <-p.quit:
	}
	return nil
}

func (p *devPlatform) Quit() { p.once.Do(func() { close(p.quit) }) }

func (p *devPlatform) OpenGallery(u string) { log.Printf("open gallery: %s", u) }

func (p *devPlatform) OpenURL(u string) error {
	p.mu.Lock()
	p.lastURL = u
	fake := p.fakeURL
	p.mu.Unlock()
	if fake != "" && strings.HasPrefix(u, fake) {
		// Pretend the user clicked "Allow".
		go func() {
			time.Sleep(300 * time.Millisecond)
			if resp, err := http.Get(u); err == nil {
				resp.Body.Close()
			}
		}()
	}
	log.Printf("open url: %s", u)
	return nil
}

func (p *devPlatform) CopyImage(pngData []byte, img image.Image) error {
	p.mu.Lock()
	p.copied++
	p.mu.Unlock()
	return nil
}

func (p *devPlatform) Reveal(path string) error   { log.Printf("reveal %s", path); return nil }
func (p *devPlatform) OpenPath(path string) error { log.Printf("open %s", path); return nil }

func (p *devPlatform) SetAutostart(on bool) error {
	p.mu.Lock()
	p.autostart = on
	p.mu.Unlock()
	return nil
}

func (p *devPlatform) ApplyHotkey(spec string) error {
	if strings.EqualFold(spec, "Ctrl+Alt+X") {
		return errors.New("다른 프로그램이 이 단축키를 쓰고 있어요")
	}
	p.mu.Lock()
	p.hotkey = spec
	p.mu.Unlock()
	return nil
}

func (p *devPlatform) Notify(title, msg string) {
	p.mu.Lock()
	p.notes = append(p.notes, title)
	p.mu.Unlock()
	log.Printf("notify: %s — %s", title, msg)
}

func (p *devPlatform) UpdateTray(TrayState) {}

func (p *devPlatform) Proxy(r *http.Request) (*url.URL, error) { return http.ProxyFromEnvironment(r) }

func (p *devPlatform) TokenStore(dir string) drive.TokenStore {
	return fileTokens(filepath.Join(dir, "google-token.json"))
}

type fileTokens string

func (f fileTokens) Load() (drive.Token, error) {
	var t drive.Token
	b, err := os.ReadFile(string(f))
	if err != nil {
		return t, err
	}
	return t, json.Unmarshal(b, &t)
}

func (f fileTokens) Save(t drive.Token) error {
	b, _ := json.Marshal(t)
	return os.WriteFile(string(f), b, 0o600)
}

func (f fileTokens) Clear() error {
	err := os.Remove(string(f))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// startFakeDrive serves a fake Google on 127.0.0.1 and points the app at it.
func (p *devPlatform) startFakeDrive(a *App) {
	p.fake = fakedrive.New()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	base := "http://" + ln.Addr().String()
	go http.Serve(ln, p.fake)
	p.mu.Lock()
	p.fakeURL = base
	p.mu.Unlock()
	os.Setenv("SCREENMIT_DRIVE_BASE", base)
	fmt.Printf("FAKEDRIVE %s\n", base)
	// Credentials are dropped in by the test through the gallery, like a user would.
	_ = os.WriteFile(filepath.Join(a.cfg.Dir(), "fake-client.json"), []byte(fmt.Sprintf(
		`{"installed":{"client_id":"dev-client","client_secret":"dev-secret","auth_uri":"%s/o/oauth2/auth","token_uri":"%s/token"}}`, base, base)), 0o644)
	// Login credentials of an earlier run point at an old port: reload.
	if b, err := os.ReadFile(a.credsPath()); err == nil && strings.Contains(string(b), "dev-client") {
		_ = a.DriveCredentials([]byte(fmt.Sprintf(`{"installed":{"client_id":"dev-client","client_secret":"dev-secret","auth_uri":"%s/o/oauth2/auth","token_uri":"%s/token"}}`, base, base)))
	}
}

func (p *devPlatform) devRoutes(a *App) {
	srv := a.Server()
	// A screenshot "taken" on this PC.
	srv.Handle("POST /api/dev/capture", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(io.LimitReader(r.Body, 64<<20))
		q := r.URL.Query()
		t := time.Now()
		if v := q.Get("time"); v != "" {
			if pt, err := time.Parse(time.RFC3339, v); err == nil {
				t = pt
			}
		}
		kind := imgclip.SourcePNG
		if q.Get("kind") == "dib" {
			kind = imgclip.SourceDIB
		}
		a.paused.Store(false)
		a.handleCapture(Capture{Data: b, Kind: kind, App: q.Get("app"), Exe: q.Get("exe"), Title: q.Get("title"), Time: t})
		writeDevJSON(w, map[string]any{"ok": true})
	})
	// What the platform saw (copies, autostart, hotkey, notifications).
	srv.Handle("GET /api/dev/platform", func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		writeDevJSON(w, map[string]any{"copied": p.copied, "lastURL": p.lastURL, "autostart": p.autostart, "hotkey": p.hotkey, "notes": p.notes})
	})
	srv.Handle("GET /api/dev/fake-client", func(w http.ResponseWriter, r *http.Request) {
		b, err := os.ReadFile(filepath.Join(a.cfg.Dir(), "fake-client.json"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	})
	// A screenshot from "another PC": put straight onto the fake Drive.
	srv.Handle("POST /api/dev/remote", func(w http.ResponseWriter, r *http.Request) {
		if p.fake == nil {
			http.Error(w, "no fake drive", 400)
			return
		}
		b, _ := io.ReadAll(io.LimitReader(r.Body, 64<<20))
		img, _, err := imgclip.DecodeImage(b)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		id, _ := shots.HashImage(img)
		thumb, _ := shots.MakeThumb(img)
		roots := p.fake.Files("root")
		thumbs := p.fake.Files("thumbs")
		if len(roots) == 0 || len(thumbs) == 0 {
			http.Error(w, "drive not set up yet", 409)
			return
		}
		q := r.URL.Query()
		t := time.Now()
		if v := q.Get("time"); v != "" {
			if pt, err := time.Parse(time.RFC3339, v); err == nil {
				t = pt
			}
		}
		tid := p.fake.Put(id+".jpg", "image/jpeg", []string{thumbs[0]["id"].(string)}, map[string]string{"screenmit": "thumb", "id": id}, "", thumb)
		desc, _ := json.Marshal(map[string]string{"title": q.Get("title"), "note": q.Get("note")})
		fid := p.fake.Put("Screenmit_"+t.Format("2006-01-02_15-04-05")+".png", "image/png", []string{roots[0]["id"].(string)}, map[string]string{
			"screenmit": "shot", "id": id, "pc": q.Get("pc"), "t": strconv.FormatInt(t.UnixMilli(), 10),
			"w": strconv.Itoa(img.Bounds().Dx()), "h": strconv.Itoa(img.Bounds().Dy()), "app": q.Get("app"),
			"pin": "0", "u": "0", "thumb": tid,
		}, string(desc), b)
		a.kickSync()
		writeDevJSON(w, map[string]any{"ok": true, "id": id, "driveId": fid})
	})
	// Another PC pins / deletes.
	srv.Handle("POST /api/dev/remote-edit", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID     string `json:"id"`
			Pin    *bool  `json:"pin"`
			Delete bool   `json:"delete"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		for _, f := range p.fake.Files("shot") {
			props := f["appProperties"].(map[string]string)
			if props["id"] != in.ID {
				continue
			}
			if in.Delete {
				p.fake.Remove(f["id"].(string))
			} else if in.Pin != nil {
				v := "0"
				if *in.Pin {
					v = "1"
				}
				p.fake.SetProps(f["id"].(string), map[string]string{"pin": v, "u": strconv.FormatInt(time.Now().UnixMilli(), 10)}, nil)
			}
		}
		a.kickSync()
		writeDevJSON(w, map[string]any{"ok": true})
	})
	// Google stops accepting the login (revoked, or blocked).
	srv.Handle("POST /api/dev/drive-revoke", func(w http.ResponseWriter, r *http.Request) {
		if p.fake != nil {
			p.fake.RevokeAll()
		}
		a.kickSync()
		writeDevJSON(w, map[string]any{"ok": true})
	})
	srv.Handle("GET /api/dev/drive", func(w http.ResponseWriter, r *http.Request) {
		if p.fake == nil {
			writeDevJSON(w, map[string]any{})
			return
		}
		writeDevJSON(w, map[string]any{"shots": p.fake.Files("shot"), "thumbs": len(p.fake.Files("thumb")), "settings": p.fake.Files("settings")})
	})
}

func writeDevJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
