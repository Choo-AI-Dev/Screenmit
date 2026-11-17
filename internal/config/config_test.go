package config

import "testing"

func TestHotkey(t *testing.T) {
	good := map[string]string{
		"Ctrl+Alt+S": "Ctrl+Alt+S", "alt+ctrl+s": "Ctrl+Alt+S", "Ctrl+Shift+Alt+9": "Ctrl+Alt+Shift+9",
		"F9": "F9", "Shift+F12": "Shift+F12", "Win+Alt+Q": "Alt+Win+Q",
		"PrintScreen": "PrintScreen", "prtsc": "PrintScreen", "Ctrl+PrintScreen": "Ctrl+PrintScreen",
		"Pause": "Pause", "Scroll Lock": "ScrollLock", "Shift+Insert": "Shift+Insert", "F13": "F13", "Alt+F24": "Alt+F24",
	}
	for in, out := range good {
		hk, ok, err := ParseHotkey(in)
		if err != nil || !ok || FormatHotkey(hk) != out {
			t.Errorf("%s: %v %v %s", in, ok, err, FormatHotkey(hk))
		}
	}
	for _, in := range []string{"S", "Shift+S", "Ctrl+", "Ctrl+Alt", "Ctrl+Space", "Ctrl+S+D", "F25", "F09", "Ctrl+Home"} {
		if _, _, err := ParseHotkey(in); err == nil {
			t.Errorf("%s accepted", in)
		}
	}
	if _, ok, err := ParseHotkey("off"); ok || err != nil {
		t.Error("off")
	}
	if !SameKey("alt+ctrl+s", "Ctrl+Alt+S") || SameKey("Ctrl+Alt+S", "Ctrl+Alt+D") || SameKey("off", "off") {
		t.Error("SameKey")
	}
}

func TestCaptureDefaults(t *testing.T) {
	var c Config
	if c.CaptureKeySpec() != "off" || c.CaptureModeOr() != CaptureRegion {
		t.Errorf("%q %q", c.CaptureKeySpec(), c.CaptureModeOr())
	}
	c.CaptureKey, c.CaptureMode = "F9", "window"
	if c.CaptureKeySpec() != "F9" || c.CaptureModeOr() != CaptureWindow {
		t.Error("set")
	}
	c.CaptureMode = "nope"
	if c.CaptureModeOr() != CaptureRegion {
		t.Error("bad mode")
	}
}

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	f, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := f.Get()
	if len(c.Token) != 48 || c.Retention() != 14 || !c.SkipDocuments() || !c.AutostartOn() || c.HotkeySpec() != DefaultHotkey {
		t.Fatalf("%+v", c)
	}
	f.Update(func(c *Config) { c.RetentionDays = 7; c.SkipDocs = Bool(false) })
	f2, _ := Load(dir)
	if g := f2.Get(); g.Retention() != 7 || g.SkipDocuments() || g.Token != c.Token {
		t.Fatalf("%+v", g)
	}
}
