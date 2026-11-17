// Package config holds the user's settings (config.json in the app folder).
package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Config is saved as JSON. Zero values mean "default".
type Config struct {
	Folder        string `json:"folder"`
	RetentionDays int    `json:"retentionDays"`
	RetentionAt   int64  `json:"retentionAt,omitempty"` // unix ms of the last change (shared through Drive)
	SkipDocs      *bool  `json:"skipDocs,omitempty"`
	Autostart     *bool  `json:"autostart,omitempty"`
	Notify        bool   `json:"notify"`
	Hotkey        string `json:"hotkey"`                // "Ctrl+Alt+S", "" = default, "off"
	CaptureKey    string `json:"captureKey,omitempty"`  // takes a screenshot; "" = off
	CaptureMode   string `json:"captureMode,omitempty"` // CaptureRegion (default), CaptureScreen, CaptureWindow
	PCName        string `json:"pcName,omitempty"`
	Port          int    `json:"port,omitempty"`
	Token         string `json:"token"`
	Welcomed      bool   `json:"welcomed,omitempty"`
	DriveOn       bool   `json:"driveOn,omitempty"`
}

// Retention choices offered in the UI.
var RetentionChoices = []int{3, 7, 14, 30}

const (
	DefaultRetention = 14
	DefaultHotkey    = "Ctrl+Alt+S"
	DefaultPort      = 27839
)

// What the screenshot key does.
const (
	CaptureRegion = "region" // Windows' own snipping overlay (like Win + Shift + S)
	CaptureScreen = "screen" // every monitor, saved and copied at once
	CaptureWindow = "window" // the window in front, saved and copied at once
)

var CaptureModes = []string{CaptureRegion, CaptureScreen, CaptureWindow}

// CaptureKeySpec is the screenshot key, or "off".
func (c Config) CaptureKeySpec() string {
	if strings.TrimSpace(c.CaptureKey) == "" {
		return "off"
	}
	return c.CaptureKey
}

func (c Config) CaptureModeOr() string {
	for _, m := range CaptureModes {
		if c.CaptureMode == m {
			return m
		}
	}
	return CaptureRegion
}

func (c Config) Retention() int {
	if c.RetentionDays <= 0 {
		return DefaultRetention
	}
	return c.RetentionDays
}

func (c Config) SkipDocuments() bool { return c.SkipDocs == nil || *c.SkipDocs }
func (c Config) AutostartOn() bool   { return c.Autostart == nil || *c.Autostart }

func (c Config) HotkeySpec() string {
	if c.Hotkey == "" {
		return DefaultHotkey
	}
	return c.Hotkey
}

func Bool(b bool) *bool { return &b }

// File is a config on disk.
type File struct {
	mu   sync.Mutex
	path string
	cfg  Config
}

// Load reads dir/config.json, filling defaults and a fresh token.
func Load(dir string) (*File, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f := &File{path: filepath.Join(dir, "config.json")}
	b, err := os.ReadFile(f.path)
	if err == nil {
		if err := json.Unmarshal(b, &f.cfg); err != nil {
			_ = os.Rename(f.path, f.path+".broken")
			f.cfg = Config{}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if len(f.cfg.Token) < 32 {
		f.cfg.Token = randomToken()
		if err := f.saveLocked(); err != nil {
			return nil, err
		}
	}
	return f, nil
}

func randomToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Get returns a copy.
func (f *File) Get() Config {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cfg
}

// Dir is the folder config.json lives in.
func (f *File) Dir() string { return filepath.Dir(f.path) }

// Update changes and saves.
func (f *File) Update(fn func(*Config)) (Config, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(&f.cfg)
	return f.cfg, f.saveLocked()
}

func (f *File) saveLocked() error {
	b, err := json.MarshalIndent(f.cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.path)
}

// Hotkey is a parsed global shortcut.
type Hotkey struct {
	Mods uint32 // MOD_ALT=1, MOD_CONTROL=2, MOD_SHIFT=4, MOD_WIN=8
	VK   uint32
}

const (
	ModAlt     = 1
	ModControl = 2
	ModShift   = 4
	ModWin     = 8
)

// ParseHotkey reads "Ctrl+Alt+S" style text. "off" gives ok=false, err=nil.
func ParseHotkey(spec string) (hk Hotkey, ok bool, err error) {
	spec = strings.TrimSpace(spec)
	if spec == "" || strings.EqualFold(spec, "off") {
		return Hotkey{}, false, nil
	}
	parts := strings.Split(spec, "+")
	for i, p := range parts {
		p = strings.TrimSpace(p)
		last := i == len(parts)-1
		switch strings.ToLower(p) {
		case "ctrl", "control":
			hk.Mods |= ModControl
		case "alt":
			hk.Mods |= ModAlt
		case "shift":
			hk.Mods |= ModShift
		case "win":
			hk.Mods |= ModWin
		default:
			if !last || hk.VK != 0 {
				return Hotkey{}, false, fmt.Errorf("모르는 키: %q", p)
			}
			vk, err := keyVK(p)
			if err != nil {
				return Hotkey{}, false, err
			}
			hk.VK = vk
		}
	}
	if hk.VK == 0 {
		return Hotkey{}, false, errors.New("글자 키가 없어요")
	}
	if hk.Mods&(ModControl|ModAlt|ModWin) == 0 && !soloKey(hk.VK) {
		return Hotkey{}, false, errors.New("Ctrl 이나 Alt 를 같이 눌러 주세요")
	}
	return hk, true, nil
}

// Keys that can be a shortcut on their own (and with Shift).
var namedKeys = []struct {
	name string
	vk   uint32
}{
	{"PrintScreen", 0x2C}, {"Pause", 0x13}, {"ScrollLock", 0x91}, {"Insert", 0x2D},
}

func soloKey(vk uint32) bool {
	if vk >= 0x70 && vk <= 0x87 { // F1–F24
		return true
	}
	for _, k := range namedKeys {
		if k.vk == vk {
			return true
		}
	}
	return false
}

// SameKey reports whether two shortcut texts are the same keys.
func SameKey(a, b string) bool {
	ha, oka, ea := ParseHotkey(a)
	hb, okb, eb := ParseHotkey(b)
	return oka && okb && ea == nil && eb == nil && ha == hb
}

func keyVK(k string) (uint32, error) {
	u := strings.ToUpper(k)
	if len(u) == 1 && (u[0] >= 'A' && u[0] <= 'Z' || u[0] >= '0' && u[0] <= '9') {
		return uint32(u[0]), nil
	}
	for _, nk := range namedKeys {
		if strings.EqualFold(strings.ReplaceAll(k, " ", ""), nk.name) {
			return nk.vk, nil
		}
	}
	switch strings.ToLower(strings.ReplaceAll(k, " ", "")) {
	case "prtsc", "prtscn", "printscrn":
		return 0x2C, nil
	}
	if strings.HasPrefix(u, "F") {
		var n int
		if _, err := fmt.Sscanf(u, "F%d", &n); err == nil && n >= 1 && n <= 24 && fmt.Sprintf("F%d", n) == u {
			return uint32(0x70 + n - 1), nil
		}
	}
	return 0, fmt.Errorf("쓸 수 없는 키: %q", k)
}

// FormatHotkey writes a hotkey back as "Ctrl+Alt+S".
func FormatHotkey(hk Hotkey) string {
	var p []string
	if hk.Mods&ModControl != 0 {
		p = append(p, "Ctrl")
	}
	if hk.Mods&ModAlt != 0 {
		p = append(p, "Alt")
	}
	if hk.Mods&ModShift != 0 {
		p = append(p, "Shift")
	}
	if hk.Mods&ModWin != 0 {
		p = append(p, "Win")
	}
	name := string(rune(hk.VK))
	if hk.VK >= 0x70 && hk.VK <= 0x87 {
		name = fmt.Sprintf("F%d", hk.VK-0x70+1)
	}
	for _, nk := range namedKeys {
		if nk.vk == hk.VK {
			name = nk.name
		}
	}
	p = append(p, name)
	return strings.Join(p, "+")
}
