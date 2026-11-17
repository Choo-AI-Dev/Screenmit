package main

import (
	"testing"
	"unicode/utf16"
	"unsafe"
)

func TestWin32StructSizes(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("64-bit layout only")
	}
	cases := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"MSG", unsafe.Sizeof(wMsg{}), 48},
		{"WNDCLASSEXW", unsafe.Sizeof(wWndClassEx{}), 80},
		{"NOTIFYICONDATAW", unsafe.Sizeof(wNotifyIconData{}), 976},
		{"GUID", unsafe.Sizeof(wGUID{}), 16},
		{"POINT", unsafe.Sizeof(wPoint{}), 8},
		{"DATA_BLOB", unsafe.Sizeof(wDataBlob{}), 16},
		{"WINHTTP_CURRENT_USER_IE_PROXY_CONFIG", unsafe.Sizeof(wIEProxyConfig{}), 32},
		{"WINHTTP_AUTOPROXY_OPTIONS", unsafe.Sizeof(wAutoProxyOptions{}), 32},
		{"WINHTTP_PROXY_INFO", unsafe.Sizeof(wProxyInfo{}), 24},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: %d, want %d", c.name, c.got, c.want)
		}
	}
	var n wNotifyIconData
	offs := map[string][2]uintptr{
		"hWnd":             {unsafe.Offsetof(n.Wnd), 8},
		"uCallbackMessage": {unsafe.Offsetof(n.CallbackMessage), 24},
		"hIcon":            {unsafe.Offsetof(n.Icon), 32},
		"szTip":            {unsafe.Offsetof(n.Tip), 40},
		"szInfo":           {unsafe.Offsetof(n.Info), 304},
		"szInfoTitle":      {unsafe.Offsetof(n.InfoTitle), 820},
		"dwInfoFlags":      {unsafe.Offsetof(n.InfoFlags), 948},
		"hBalloonIcon":     {unsafe.Offsetof(n.BalloonIcon), 968},
	}
	for k, v := range offs {
		if v[0] != v[1] {
			t.Errorf("NOTIFYICONDATAW.%s at %d, want %d", k, v[0], v[1])
		}
	}
	var m wMsg
	if unsafe.Offsetof(m.WParam) != 16 || unsafe.Offsetof(m.Pt) != 36 {
		t.Error("MSG offsets")
	}
}

func TestPutUTF16(t *testing.T) {
	var b [8]uint16
	putUTF16(b[:], "스크린😀abcdef")
	got := utf16.Decode(b[:])
	// 3 Hangul + surrogate pair (2) + "ab" = 7 units, last stays NUL
	if string(got[:6]) != "스크린😀ab" || b[7] != 0 {
		t.Fatalf("%q", string(got))
	}
	putUTF16(b[:], "")
	if b[0] != 0 {
		t.Fatal("not cleared")
	}
}
