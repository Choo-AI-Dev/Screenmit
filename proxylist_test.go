package main

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func TestParseProxyList(t *testing.T) {
	cases := []struct {
		in    string
		https bool
		want  string
	}{
		{"proxy.corp:8080", true, "proxy.corp:8080"},
		{"http=p1:80;https=p2:443", true, "p2:443"},
		{"http=p1:80;https=p2:443", false, "p1:80"},
		{"http=p1:80", true, "p1:80"},
		{"PROXY a:3128; PROXY b:3128", true, "a:3128"},
		{"10.1.2.3:8080; 10.1.2.4:8080", true, "10.1.2.3:8080"},
		{"DIRECT", true, ""},
		{"", true, ""},
	}
	for _, c := range cases {
		u := parseProxyList(c.in, c.https)
		got := ""
		if u != nil {
			got = u.Host
		}
		if got != c.want {
			t.Errorf("%q https=%v: %q, want %q", c.in, c.https, got, c.want)
		}
	}
}

func TestTrimPNG(t *testing.T) {
	var buf bytes.Buffer
	png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 3, 3)))
	clean := buf.Bytes()
	padded := append(append([]byte(nil), clean...), make([]byte, 37)...)
	if got := trimPNG(padded); !bytes.Equal(got, clean) {
		t.Fatalf("len %d want %d", len(got), len(clean))
	}
	if got := trimPNG([]byte("junk")); string(got) != "junk" {
		t.Fatal("non-png changed")
	}
}
