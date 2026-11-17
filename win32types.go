package main

// Win32 structures, laid out like the Windows SDK on 64-bit. They have no
// build tag so win32types_test.go can check their sizes on any 64-bit OS.

type wPoint struct{ X, Y int32 }

type wMsg struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       wPoint
	LPrivate uint32
}

type wWndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type wGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// NOTIFYICONDATAW (Vista+ layout, 976 bytes on x64).
type wNotifyIconData struct {
	Size             uint32
	Wnd              uintptr
	ID               uint32
	Flags            uint32
	CallbackMessage  uint32
	Icon             uintptr
	Tip              [128]uint16
	State            uint32
	StateMask        uint32
	Info             [256]uint16
	TimeoutOrVersion uint32
	InfoTitle        [64]uint16
	InfoFlags        uint32
	GuidItem         wGUID
	BalloonIcon      uintptr
}

// DATA_BLOB for DPAPI.
type wDataBlob struct {
	Size uint32
	Data uintptr
}

// WinHTTP proxy structures.
type wIEProxyConfig struct {
	AutoDetect    int32
	AutoConfigURL uintptr
	Proxy         uintptr
	ProxyBypass   uintptr
}

type wAutoProxyOptions struct {
	Flags           uint32
	AutoDetectFlags uint32
	AutoConfigURL   uintptr
	Reserved        uintptr
	Reserved2       uint32
	AutoLogon       int32
}

type wProxyInfo struct {
	AccessType  uint32
	Proxy       uintptr
	ProxyBypass uintptr
}

// putUTF16 copies s into a fixed buffer, cut to fit and NUL-terminated.
func putUTF16(dst []uint16, s string) {
	i := 0
	for _, r := range s {
		if r >= 0x10000 {
			if i+2 >= len(dst) {
				break
			}
			r -= 0x10000
			dst[i] = uint16(0xD800 + (r >> 10))
			dst[i+1] = uint16(0xDC00 + (r & 0x3FF))
			i += 2
			continue
		}
		if i+1 >= len(dst) {
			break
		}
		dst[i] = uint16(r)
		i++
	}
	for ; i < len(dst); i++ {
		dst[i] = 0
	}
}
