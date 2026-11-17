package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/Choo-AI-Dev/screenmit/internal/config"
	"github.com/Choo-AI-Dev/screenmit/internal/drive"
	"github.com/Choo-AI-Dev/screenmit/internal/imgclip"
	"github.com/Choo-AI-Dev/screenmit/internal/server"
	"github.com/Choo-AI-Dev/screenmit/internal/shots"
	"github.com/Choo-AI-Dev/screenmit/web"
)

// Version is set by the build (-ldflags "-X main.Version=…").
var Version = "0.1.0"

// The web page (GitHub Pages). It may read this PC's gallery with the token,
// so it can show this PC's screenshots when Google sign-in is not there.
const defaultWebURL = "https://choo-ai-dev.github.io/Screenmit/"

func webURL() string {
	if u := os.Getenv("SCREENMIT_WEB_URL"); u != "" { // tests
		return u
	}
	return defaultWebURL
}

func webOrigins() []string {
	out := []string{}
	for _, u := range []string{webURL(), defaultWebURL} {
		if p, err := url.Parse(u); err == nil && p.Host != "" {
			out = append(out, p.Scheme+"://"+p.Host)
		}
	}
	return out
}

// Google OAuth client baked in by release builds (from repository secrets).
// A google-oauth.json in the settings folder works too.
var (
	googleClientID     = ""
	googleClientSecret = ""
)

// Platform is the OS side: tray, clipboard, browser, autostart.
type Platform interface {
	Name() string
	ConfigDir() string
	DefaultFolder() string
	PCName() string
	// Run blocks in the OS loop until Quit.
	Run(a *App) error
	Quit()
	OpenGallery(url string)
	OpenURL(u string) error
	CopyImage(pngData []byte, img image.Image) error
	Reveal(path string) error
	OpenPath(path string) error
	SetAutostart(on bool) error
	ApplyHotkey(spec string) error
	// ApplyCaptureKey registers the screenshot key ("off" removes it).
	ApplyCaptureKey(spec string) error
	// StartRegionCapture opens Windows' own snipping overlay; the result
	// arrives through the clipboard like any other capture.
	StartRegionCapture() error
	// Grab takes the screen (all monitors) or the window in front.
	Grab(window bool) (Grab, error)
	Notify(title, msg string)
	UpdateTray(t TrayState)
	TokenStore(dir string) drive.TokenStore
	// Proxy picks the proxy for a request (Windows: the system settings).
	Proxy(*http.Request) (*url.URL, error)
	// Claim makes this the only running copy; false means another one
	// was found (and asked to show its gallery unless background).
	Claim(background bool) bool
	Fatal(err error)
}

// TrayState is what the tray icon shows.
type TrayState struct {
	Paused bool
	Today  int
	Drive  string // "", "ok", "error"
}

// Grab is a screenshot Screenmit took itself, and where it was taken.
type Grab struct {
	Img   image.Image
	App   string
	Exe   string
	Title string
}

// Capture is one picture from the clipboard.
type Capture struct {
	Data  []byte
	Kind  imgclip.Source
	App   string
	Exe   string
	Title string
	Time  time.Time
}

// App is the program.
type App struct {
	plat Platform
	cfg  *config.File
	srv  *server.Server

	storeMu sync.RWMutex
	store   *shots.Store

	paused     atomic.Bool
	hotkeyErr  atomic.Value // string
	captureErr atomic.Value // string
	capq       chan Capture
	ctx        context.Context
	cancel     context.CancelFunc

	driveMu    sync.Mutex
	creds      *drive.Credentials
	api        *drive.API
	syncer     *drive.Syncer
	syncStop   context.CancelFunc
	loginBusy  bool
	loginErr   string
	httpClient *http.Client
}

// Options come from the command line.
type Options struct {
	Folder     string
	ConfigDir  string
	Port       int
	Background bool
}

// NewApp loads settings and the collection and starts the server.
func NewApp(plat Platform, o Options) (*App, error) {
	dir := o.ConfigDir
	if dir == "" {
		dir = plat.ConfigDir()
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return nil, err
	}
	if o.Folder != "" {
		cfg.Update(func(c *config.Config) { c.Folder = o.Folder })
	}
	c := cfg.Get()
	folder := c.Folder
	if folder == "" {
		folder = plat.DefaultFolder()
	}
	st, err := shots.Open(folder)
	if err != nil {
		return nil, fmt.Errorf("저장 폴더를 열 수 없어요 (%s): %w", folder, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = plat.Proxy
	a := &App{plat: plat, cfg: cfg, store: st, capq: make(chan Capture, 16), ctx: ctx, cancel: cancel,
		httpClient: &http.Client{Timeout: 90 * time.Second, Transport: tr}}
	a.hotkeyErr.Store("")
	a.captureErr.Store("")
	a.srv = server.New(a, c.Token, web.Files, webOrigins()...)
	st.OnChange = a.changed

	port := c.Port
	if o.Port != 0 {
		port = o.Port
	}
	if port == 0 {
		port = config.DefaultPort
	}
	ln, err := a.srv.Listen(port)
	if err != nil {
		return nil, fmt.Errorf("갤러리 서버를 열 수 없어요: %w", err)
	}
	go func() {
		if err := a.srv.Serve(ln); err != nil {
			log.Printf("server: %v", err)
		}
	}()
	go a.captureLoop()
	go a.cleanupLoop()
	a.loadDrive()
	return a, nil
}

// Store is the current collection.
func (a *App) Store() *shots.Store {
	a.storeMu.RLock()
	defer a.storeMu.RUnlock()
	return a.store
}

// Server is the gallery server.
func (a *App) Server() *server.Server { return a.srv }

// GalleryURL opens the gallery logged in.
func (a *App) GalleryURL() string { return a.srv.OpenURL() }

// Config returns the settings.
func (a *App) Config() config.Config { return a.cfg.Get() }

// Paused reports whether capturing is paused.
func (a *App) Paused() bool { return a.paused.Load() }

// SetPaused stops or resumes capturing.
func (a *App) SetPaused(p bool) {
	a.paused.Store(p)
	a.refresh()
}

// SetHotkeyError records whether the shortcut could be registered.
func (a *App) SetHotkeyError(s string) { a.hotkeyErr.Store(s) }

// SetCaptureKeyError records whether the screenshot key could be registered.
func (a *App) SetCaptureKeyError(s string) { a.captureErr.Store(s) }

func (a *App) changed() {
	a.srv.Broadcast()
	a.plat.UpdateTray(a.trayState())
}

func (a *App) refresh() { a.changed() }

func (a *App) trayState() TrayState {
	ts := TrayState{Paused: a.Paused()}
	today := time.Now().Format("2006-01-02")
	for _, sh := range a.Store().List() {
		if !sh.Remote && sh.Time.Local().Format("2006-01-02") == today {
			ts.Today++
		}
	}
	a.driveMu.Lock()
	if a.syncer != nil {
		if st := a.syncer.Status(); st.Error != "" {
			ts.Drive = "error"
		} else {
			ts.Drive = "ok"
		}
	}
	a.driveMu.Unlock()
	return ts
}

// PCName is how this PC is labelled on Drive.
func (a *App) PCName() string {
	if n := strings.TrimSpace(a.cfg.Get().PCName); n != "" {
		return n
	}
	return a.plat.PCName()
}

// ---------- capturing ----------

// Capture queues a clipboard picture (called from the OS thread).
func (a *App) Capture(c Capture) {
	if a.Paused() {
		return
	}
	select {
	case a.capq <- c:
	default:
		log.Printf("capture queue full, dropped one")
	}
}

func (a *App) captureLoop() {
	for c := range a.capq {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("capture panic: %v", r)
				}
			}()
			a.handleCapture(c)
		}()
	}
}

func (a *App) handleCapture(c Capture) {
	var img image.Image
	var pngData []byte
	var err error
	switch c.Kind {
	case imgclip.SourcePNG:
		pngData = trimPNG(c.Data)
		img, err = png.Decode(bytes.NewReader(pngData))
	default:
		img, err = imgclip.DecodeDIB(c.Data)
	}
	if err != nil {
		log.Printf("capture: cannot read picture: %v", err)
		return
	}
	if b := img.Bounds(); b.Dx() < 2 || b.Dy() < 2 {
		return
	}
	a.save(pngData, img, c, false)
}

// save stores one picture. explicit is a screenshot taken with Screenmit's
// own key: it always says so, since nothing else on screen does.
func (a *App) save(pngData []byte, img image.Image, c Capture, explicit bool) {
	sh, added, err := a.Store().Add(pngData, img, shots.Meta{Time: c.Time, App: shortApp(c.App), Exe: c.Exe, Title: c.Title, PC: a.PCName()})
	if err != nil {
		log.Printf("capture: save: %v", err)
		if explicit {
			a.plat.Notify("스크린샷을 저장하지 못했어요", err.Error())
		}
		return
	}
	cfg := a.cfg.Get()
	if !added {
		if explicit {
			a.plat.Notify("스크린샷을 복사했어요", "똑같은 화면이 이미 저장돼 있어요")
		}
		return
	}
	log.Printf("saved %s %dx%d from %s", sh.File, sh.W, sh.H, c.Exe)
	switch {
	case explicit:
		if !cfg.Welcomed {
			a.cfg.Update(func(c *config.Config) { c.Welcomed = true })
		}
		a.plat.Notify("스크린샷을 찍었어요", fmt.Sprintf("저장하고 복사해 뒀어요 · %s · %d×%d", nonEmpty(sh.App, "화면"), sh.W, sh.H))
	case !cfg.Welcomed:
		a.cfg.Update(func(c *config.Config) { c.Welcomed = true })
		a.plat.Notify("스크린샷을 모으기 시작했어요", fmt.Sprintf("%d일 동안 보관해요. 트레이의 Screenmit 아이콘을 누르면 볼 수 있어요.", cfg.Retention()))
	case cfg.Notify:
		a.plat.Notify("스크린샷 저장", fmt.Sprintf("%s · %d×%d", nonEmpty(sh.App, "스크린샷"), sh.W, sh.H))
	}
	a.kickSync()
}

// TakeScreenshot is the screenshot key. "Region" hands over to Windows'
// snipping overlay (its result comes back through the clipboard); the screen
// and window modes take the picture here, copy it like Print Screen does,
// and save it unless collecting is paused.
func (a *App) TakeScreenshot() {
	mode := a.cfg.Get().CaptureModeOr()
	if mode == config.CaptureRegion {
		err := a.plat.StartRegionCapture()
		if err == nil {
			return
		}
		log.Printf("region capture: %v; taking the whole screen", err)
		mode = config.CaptureScreen
	}
	g, err := a.plat.Grab(mode == config.CaptureWindow)
	if err != nil {
		log.Printf("grab: %v", err)
		a.plat.Notify("스크린샷을 찍지 못했어요", err.Error())
		return
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, g.Img); err != nil {
		log.Printf("grab: encode: %v", err)
		return
	}
	if err := a.plat.CopyImage(buf.Bytes(), g.Img); err != nil {
		log.Printf("grab: copy: %v", err)
	}
	if a.Paused() {
		a.plat.Notify("스크린샷을 복사했어요", "모으기를 멈춘 중이라 저장하지는 않았어요")
		return
	}
	a.save(buf.Bytes(), g.Img, Capture{App: g.App, Exe: g.Exe, Title: g.Title, Time: time.Now()}, true)
}

// shortApp drops the vendor prefix ("Microsoft PowerPoint" -> "PowerPoint").
func shortApp(s string) string {
	s = strings.TrimSpace(s)
	for _, p := range []string{"Microsoft ", "Google "} {
		if strings.HasPrefix(s, p) && len(s) > len(p)+1 {
			s = strings.TrimPrefix(s, p)
		}
	}
	return s
}

func nonEmpty(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func (a *App) cleanupLoop() {
	t := time.NewTicker(15 * time.Minute)
	defer t.Stop()
	for {
		if n := a.Store().Cleanup(a.cfg.Get().Retention()); n > 0 {
			log.Printf("cleanup: removed %d old screenshots", n)
		}
		select {
		case <-a.ctx.Done():
			return
		case <-t.C:
		}
	}
}

// ---------- server.Backend ----------

// State is what the gallery shows besides the list.
func (a *App) State() map[string]any {
	c := a.cfg.Get()
	st := map[string]any{
		"version":  Version,
		"platform": a.plat.Name(),
		"paused":   a.Paused(),
		"pc":       a.PCName(),
		"settings": map[string]any{
			"retentionDays":    c.Retention(),
			"retentionChoices": config.RetentionChoices,
			"skipDocs":         c.SkipDocuments(),
			"autostart":        c.AutostartOn(),
			"notify":           c.Notify,
			"hotkey":           c.HotkeySpec(),
			"captureKey":       c.CaptureKeySpec(),
			"captureMode":      c.CaptureModeOr(),
			"pcName":           a.PCName(),
			"folder":           a.Store().Root(),
			"welcomed":         c.Welcomed,
		},
		"hotkeyError":     a.hotkeyErr.Load(),
		"captureKeyError": a.captureErr.Load(),
		// Opening this link once lets the web page show this PC's
		// screenshots without Google (the part after # never leaves the browser).
		"webLink": fmt.Sprintf("%s#local=%d.%s", webURL(), a.srv.Port(), c.Token),
	}
	a.driveMu.Lock()
	d := map[string]any{"configured": a.creds != nil, "on": c.DriveOn, "connecting": a.loginBusy, "loginError": a.loginErr}
	if a.syncer != nil {
		d["status"] = a.syncer.Status()
	}
	a.driveMu.Unlock()
	st["drive"] = d
	return st
}

// ApplySettings changes settings from the gallery.
func (a *App) ApplySettings(p server.SettingsPatch) error {
	if p.RetentionDays != nil {
		ok := false
		for _, d := range config.RetentionChoices {
			ok = ok || d == *p.RetentionDays
		}
		if !ok {
			return errors.New("보관 기간이 맞지 않아요")
		}
		a.cfg.Update(func(c *config.Config) { c.RetentionDays, c.RetentionAt = *p.RetentionDays, time.Now().UnixMilli() })
		a.Store().Cleanup(*p.RetentionDays)
		a.kickSync()
	}
	if p.SkipDocs != nil {
		a.cfg.Update(func(c *config.Config) { c.SkipDocs = config.Bool(*p.SkipDocs) })
	}
	if p.Notify != nil {
		a.cfg.Update(func(c *config.Config) { c.Notify = *p.Notify })
	}
	if p.Welcomed != nil {
		a.cfg.Update(func(c *config.Config) { c.Welcomed = *p.Welcomed })
	}
	if p.Autostart != nil {
		if err := a.plat.SetAutostart(*p.Autostart); err != nil {
			return fmt.Errorf("자동 실행을 바꾸지 못했어요: %w", err)
		}
		a.cfg.Update(func(c *config.Config) { c.Autostart = config.Bool(*p.Autostart) })
	}
	if p.Hotkey != nil {
		spec := strings.TrimSpace(*p.Hotkey)
		if _, _, err := config.ParseHotkey(spec); err != nil {
			return err
		}
		if config.SameKey(spec, a.cfg.Get().CaptureKeySpec()) {
			return errors.New("스크린샷 찍는 단축키와 같아요. 다른 키를 골라 주세요")
		}
		if err := a.plat.ApplyHotkey(spec); err != nil {
			// put the old one back
			_ = a.plat.ApplyHotkey(a.cfg.Get().HotkeySpec())
			return err
		}
		a.cfg.Update(func(c *config.Config) { c.Hotkey = spec })
	}
	if p.CaptureMode != nil {
		ok := false
		for _, m := range config.CaptureModes {
			ok = ok || m == *p.CaptureMode
		}
		if !ok {
			return errors.New("찍는 방식이 맞지 않아요")
		}
		a.cfg.Update(func(c *config.Config) { c.CaptureMode = *p.CaptureMode })
	}
	if p.CaptureKey != nil {
		spec := strings.TrimSpace(*p.CaptureKey)
		if strings.EqualFold(spec, "off") {
			spec = ""
		}
		if _, _, err := config.ParseHotkey(spec); err != nil {
			return err
		}
		if config.SameKey(spec, a.cfg.Get().HotkeySpec()) {
			return errors.New("갤러리 여는 단축키와 같아요. 다른 키를 골라 주세요")
		}
		if err := a.plat.ApplyCaptureKey(nonEmpty(spec, "off")); err != nil {
			_ = a.plat.ApplyCaptureKey(a.cfg.Get().CaptureKeySpec())
			return err
		}
		a.SetCaptureKeyError("")
		a.cfg.Update(func(c *config.Config) { c.CaptureKey = spec })
	}
	if p.PCName != nil {
		name := strings.TrimSpace(*p.PCName)
		if utf8.RuneCountInString(name) > 30 {
			return errors.New("PC 이름은 30자까지예요")
		}
		old := a.PCName()
		a.cfg.Update(func(c *config.Config) { c.PCName = name })
		if nn := a.PCName(); nn != old {
			st := a.Store()
			for _, sh := range st.List() {
				if !sh.Remote && (sh.PC == old || sh.PC == "") {
					_ = st.Update(sh.ID, func(x *shots.Shot) {
						x.PC, x.Updated, x.Dirty = nn, time.Now().UnixMilli(), true
					})
				}
			}
			a.kickSync()
		}
	}
	if p.Folder != nil {
		if err := a.setFolder(*p.Folder); err != nil {
			return err
		}
	}
	a.refresh()
	return nil
}

func (a *App) setFolder(dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = a.plat.DefaultFolder()
	}
	if !filepath.IsAbs(dir) {
		return errors.New("전체 경로를 적어 주세요 (예: D:\\Screenmit)")
	}
	if filepath.Clean(dir) == filepath.Clean(a.Store().Root()) {
		return nil
	}
	ns, err := shots.Open(dir)
	if err != nil {
		return fmt.Errorf("그 폴더를 쓸 수 없어요: %w", err)
	}
	ns.OnChange = a.changed
	a.storeMu.Lock()
	old := a.store
	a.store = ns
	a.storeMu.Unlock()
	_ = old.Flush()
	a.cfg.Update(func(c *config.Config) {
		c.Folder = dir
		if dir == a.plat.DefaultFolder() {
			c.Folder = ""
		}
	})
	a.kickSync()
	return nil
}

// ImageFile finds (or downloads) the full picture.
func (a *App) ImageFile(ctx context.Context, sh shots.Shot) (string, error) {
	p := a.Store().ImagePath(sh)
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	a.driveMu.Lock()
	sy := a.syncer
	a.driveMu.Unlock()
	if sy == nil || !sh.Remote {
		return "", shots.ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return sy.Fetch(ctx, sh)
}

// CopyImage puts the picture on the clipboard.
func (a *App) CopyImage(ctx context.Context, sh shots.Shot) error {
	p, err := a.ImageFile(ctx, sh)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return err
	}
	return a.plat.CopyImage(b, img)
}

// Reveal shows the file in Explorer.
func (a *App) Reveal(sh shots.Shot) error {
	if sh.Remote {
		return errors.New("다른 PC의 스크린샷이라 이 PC 폴더에는 없어요")
	}
	return a.plat.Reveal(a.Store().ImagePath(sh))
}

// OpenFolder opens the collection folder.
func (a *App) OpenFolder() error { return a.plat.OpenPath(a.Store().Root()) }

// OpenURL opens a web page in the normal browser.
func (a *App) OpenURL(u string) error { return a.plat.OpenURL(u) }

// Edited is called after a change in the gallery.
func (a *App) Edited() { a.kickSync() }

// ---------- Google Drive ----------

func (a *App) credsPath() string { return filepath.Join(a.cfg.Dir(), "google-oauth.json") }

func (a *App) loadDrive() {
	var c *drive.Credentials
	if googleClientID != "" && googleClientSecret != "" {
		c = &drive.Credentials{ClientID: googleClientID, ClientSecret: googleClientSecret}
	}
	if b, err := os.ReadFile(a.credsPath()); err == nil {
		if pc, err := drive.ParseCredentials(b); err == nil {
			c = &pc
		} else {
			log.Printf("google-oauth.json: %v", err)
		}
	}
	a.driveMu.Lock()
	defer a.driveMu.Unlock()
	a.creds = c
	if c == nil {
		return
	}
	a.api = drive.NewAPI(a.httpClient, driveBase(), *c, a.plat.TokenStore(a.cfg.Dir()))
	if a.cfg.Get().DriveOn && a.api.HasLogin() {
		a.startSyncLocked()
	}
}

// driveBase can be pointed at a fake Drive for testing.
func driveBase() string {
	if u := os.Getenv("SCREENMIT_DRIVE_BASE"); u != "" {
		return u
	}
	return "https://www.googleapis.com"
}

func (a *App) startSyncLocked() {
	if a.syncer != nil {
		return
	}
	s := drive.NewSyncer(a.api)
	s.Store = a.Store
	s.PC = a.PCName
	s.Retention = func() drive.Settings {
		c := a.cfg.Get()
		return drive.Settings{RetentionDays: c.Retention(), UpdatedAt: c.RetentionAt}
	}
	s.Adopt = func(x drive.Settings) {
		a.cfg.Update(func(c *config.Config) { c.RetentionDays, c.RetentionAt = x.RetentionDays, x.UpdatedAt })
		a.Store().Cleanup(x.RetentionDays)
		a.refresh()
	}
	s.OnStatus = a.refresh
	ctx, cancel := context.WithCancel(a.ctx)
	a.syncer, a.syncStop = s, cancel
	go s.Run(ctx, func() time.Duration {
		if a.srv.Watchers() > 0 {
			return 20 * time.Second
		}
		return 90 * time.Second
	})
}

func (a *App) kickSync() {
	a.driveMu.Lock()
	s := a.syncer
	a.driveMu.Unlock()
	if s != nil {
		s.Kick()
	}
}

// DriveSync syncs now.
func (a *App) DriveSync() { a.kickSync() }

// DriveCredentials saves the OAuth client JSON from Google Cloud.
func (a *App) DriveCredentials(b []byte) error {
	c, err := drive.ParseCredentials(b)
	if err != nil {
		return err
	}
	if err := os.WriteFile(a.credsPath(), b, 0o600); err != nil {
		return err
	}
	a.driveMu.Lock()
	if a.syncStop != nil {
		a.syncStop()
		a.syncer, a.syncStop = nil, nil
	}
	a.creds = &c
	a.api = drive.NewAPI(a.httpClient, driveBase(), c, a.plat.TokenStore(a.cfg.Dir()))
	if a.cfg.Get().DriveOn && a.api.HasLogin() {
		a.startSyncLocked()
	}
	a.driveMu.Unlock()
	return nil
}

// DriveConnect starts the Google sign-in in the browser.
func (a *App) DriveConnect() error {
	a.driveMu.Lock()
	if a.creds == nil {
		a.driveMu.Unlock()
		return errors.New("먼저 Google Cloud 에서 받은 클라이언트 JSON 파일을 넣어 주세요")
	}
	if a.loginBusy {
		a.driveMu.Unlock()
		return nil
	}
	a.loginBusy, a.loginErr = true, ""
	creds, api := *a.creds, a.api
	a.driveMu.Unlock()
	a.refresh()

	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
		defer cancel()
		tok, err := drive.Login(ctx, a.httpClient, creds, a.plat.OpenURL)
		if err == nil {
			err = api.SetToken(tok)
		}
		if err == nil {
			_, err = api.About(ctx)
		}
		a.driveMu.Lock()
		a.loginBusy = false
		if err != nil {
			a.loginErr = err.Error()
			log.Printf("drive login: %v", err)
		} else {
			a.cfg.Update(func(c *config.Config) { c.DriveOn = true })
			if a.syncer != nil {
				a.syncer.ResetFolders()
				a.syncer.Kick()
			} else {
				a.startSyncLocked()
			}
		}
		a.driveMu.Unlock()
		a.refresh()
	}()
	return nil
}

// DriveDisconnect signs out. Our own files stay; other PCs' shots leave the list.
func (a *App) DriveDisconnect() error {
	a.driveMu.Lock()
	if a.syncStop != nil {
		a.syncStop()
	}
	a.syncer, a.syncStop = nil, nil
	api := a.api
	a.driveMu.Unlock()
	if api != nil {
		if tok, err := api.Tokens.Load(); err == nil && tok.Refresh != "" {
			go revoke(a.httpClient, tok.Refresh)
		}
		_ = api.Tokens.Clear()
		a.driveMu.Lock()
		a.api = drive.NewAPI(a.httpClient, driveBase(), api.Creds, api.Tokens)
		a.driveMu.Unlock()
	}
	a.cfg.Update(func(c *config.Config) { c.DriveOn = false })
	a.Store().ForgetAccount("")
	a.refresh()
	return nil
}

func revoke(hc *http.Client, token string) {
	if driveBase() != "https://www.googleapis.com" {
		return
	}
	resp, err := hc.PostForm("https://oauth2.googleapis.com/revoke", url.Values{"token": {token}})
	if err == nil {
		resp.Body.Close()
	}
}

// Shutdown saves everything.
func (a *App) Shutdown() {
	a.cancel()
	if err := a.Store().Flush(); err != nil {
		log.Printf("flush: %v", err)
	}
}

func settingsAutostart(on bool) server.SettingsPatch { return server.SettingsPatch{Autostart: &on} }
