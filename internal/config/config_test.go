package config

import "testing"

func TestHotkey(t *testing.T) {
	good := map[string]string{
		"Ctrl+Alt+S": "Ctrl+Alt+S", "alt+ctrl+s": "Ctrl+Alt+S", "Ctrl+Shift+Alt+9": "Ctrl+Alt+Shift+9",
		"F9": "F9", "Shift+F12": "Shift+F12", "Win+Alt+Q": "Alt+Win+Q",
	}
	for in, out := range good {
		hk, ok, err := ParseHotkey(in)
		if err != nil || !ok || FormatHotkey(hk) != out {
			t.Errorf("%s: %v %v %s", in, ok, err, FormatHotkey(hk))
		}
	}
	for _, in := range []string{"S", "Shift+S", "Ctrl+", "Ctrl+Alt", "Ctrl+Space", "Ctrl+S+D"} {
		if _, _, err := ParseHotkey(in); err == nil {
			t.Errorf("%s accepted", in)
		}
	}
	if _, ok, err := ParseHotkey("off"); ok || err != nil {
		t.Error("off")
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
