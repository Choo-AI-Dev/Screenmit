//go:build windows

package main

import (
	"errors"
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")
	versionD = syscall.NewLazyDLL("version.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")
	crypt32  = syscall.NewLazyDLL("crypt32.dll")

	pRegisterClassExW              = user32.NewProc("RegisterClassExW")
	pCreateWindowExW               = user32.NewProc("CreateWindowExW")
	pDefWindowProcW                = user32.NewProc("DefWindowProcW")
	pDestroyWindow                 = user32.NewProc("DestroyWindow")
	pGetMessageW                   = user32.NewProc("GetMessageW")
	pTranslateMessage              = user32.NewProc("TranslateMessage")
	pDispatchMessageW              = user32.NewProc("DispatchMessageW")
	pPostMessageW                  = user32.NewProc("PostMessageW")
	pPostQuitMessage               = user32.NewProc("PostQuitMessage")
	pRegisterWindowMessageW        = user32.NewProc("RegisterWindowMessageW")
	pFindWindowW                   = user32.NewProc("FindWindowW")
	pAddClipboardFormatListener    = user32.NewProc("AddClipboardFormatListener")
	pRemoveClipboardFormatListener = user32.NewProc("RemoveClipboardFormatListener")
	pOpenClipboard                 = user32.NewProc("OpenClipboard")
	pCloseClipboard                = user32.NewProc("CloseClipboard")
	pEmptyClipboard                = user32.NewProc("EmptyClipboard")
	pEnumClipboardFormats          = user32.NewProc("EnumClipboardFormats")
	pGetClipboardData              = user32.NewProc("GetClipboardData")
	pSetClipboardData              = user32.NewProc("SetClipboardData")
	pGetClipboardFormatNameW       = user32.NewProc("GetClipboardFormatNameW")
	pRegisterClipboardFormatW      = user32.NewProc("RegisterClipboardFormatW")
	pGetClipboardOwner             = user32.NewProc("GetClipboardOwner")
	pGetClipboardSequenceNumber    = user32.NewProc("GetClipboardSequenceNumber")
	pCreatePopupMenu               = user32.NewProc("CreatePopupMenu")
	pAppendMenuW                   = user32.NewProc("AppendMenuW")
	pTrackPopupMenu                = user32.NewProc("TrackPopupMenu")
	pDestroyMenu                   = user32.NewProc("DestroyMenu")
	pSetMenuDefaultItem            = user32.NewProc("SetMenuDefaultItem")
	pGetCursorPos                  = user32.NewProc("GetCursorPos")
	pSetForegroundWindow           = user32.NewProc("SetForegroundWindow")
	pAllowSetForegroundWindow      = user32.NewProc("AllowSetForegroundWindow")
	pRegisterHotKey                = user32.NewProc("RegisterHotKey")
	pUnregisterHotKey              = user32.NewProc("UnregisterHotKey")
	pSetWinEventHook               = user32.NewProc("SetWinEventHook")
	pUnhookWinEvent                = user32.NewProc("UnhookWinEvent")
	pGetForegroundWindow           = user32.NewProc("GetForegroundWindow")
	pGetWindowTextW                = user32.NewProc("GetWindowTextW")
	pGetClassNameW                 = user32.NewProc("GetClassNameW")
	pGetWindowThreadProcessId      = user32.NewProc("GetWindowThreadProcessId")
	pEnumWindows                   = user32.NewProc("EnumWindows")
	pIsWindowVisible               = user32.NewProc("IsWindowVisible")
	pIsIconic                      = user32.NewProc("IsIconic")
	pShowWindow                    = user32.NewProc("ShowWindow")
	pCreateIconFromResourceEx      = user32.NewProc("CreateIconFromResourceEx")
	pDestroyIcon                   = user32.NewProc("DestroyIcon")
	pGetSystemMetrics              = user32.NewProc("GetSystemMetrics")
	pMessageBoxW                   = user32.NewProc("MessageBoxW")
	pSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	pGetModuleHandleW              = kernel32.NewProc("GetModuleHandleW")
	pGetCurrentThreadId            = kernel32.NewProc("GetCurrentThreadId")
	pGlobalAlloc                   = kernel32.NewProc("GlobalAlloc")
	pGlobalFree                    = kernel32.NewProc("GlobalFree")
	pGlobalLock                    = kernel32.NewProc("GlobalLock")
	pGlobalUnlock                  = kernel32.NewProc("GlobalUnlock")
	pGlobalSize                    = kernel32.NewProc("GlobalSize")
	pLocalFree                     = kernel32.NewProc("LocalFree")
	pOpenProcess                   = kernel32.NewProc("OpenProcess")
	pCloseHandle                   = kernel32.NewProc("CloseHandle")
	pQueryFullProcessImageNameW    = kernel32.NewProc("QueryFullProcessImageNameW")
	pCreateMutexW                  = kernel32.NewProc("CreateMutexW")
	pShell_NotifyIconW             = shell32.NewProc("Shell_NotifyIconW")
	pShellExecuteW                 = shell32.NewProc("ShellExecuteW")
	pSHGetKnownFolderPath          = shell32.NewProc("SHGetKnownFolderPath")
	pRegOpenKeyExW                 = advapi32.NewProc("RegOpenKeyExW")
	pRegSetValueExW                = advapi32.NewProc("RegSetValueExW")
	pRegDeleteValueW               = advapi32.NewProc("RegDeleteValueW")
	pRegCloseKey                   = advapi32.NewProc("RegCloseKey")
	pGetFileVersionInfoSizeW       = versionD.NewProc("GetFileVersionInfoSizeW")
	pGetFileVersionInfoW           = versionD.NewProc("GetFileVersionInfoW")
	pVerQueryValueW                = versionD.NewProc("VerQueryValueW")
	pCoInitializeEx                = ole32.NewProc("CoInitializeEx")
	pCoTaskMemFree                 = ole32.NewProc("CoTaskMemFree")
	pCryptProtectData              = crypt32.NewProc("CryptProtectData")
	pCryptUnprotectData            = crypt32.NewProc("CryptUnprotectData")
)

const (
	wmNull              = 0x0000
	wmDestroy           = 0x0002
	wmClose             = 0x0010
	wmQueryEndSession   = 0x0011
	wmEndSession        = 0x0016
	wmContextMenu       = 0x007B
	wmHotkey            = 0x0312
	wmClipboardUpdate   = 0x031D
	wmLButtonUp         = 0x0202
	wmRButtonUp         = 0x0205
	wmUser              = 0x0400
	ninBalloonUserClick = wmUser + 5
	wmApp               = 0x8000
	wmTray              = wmApp + 1
	wmShowGallery       = wmApp + 2
	wmRunQueue          = wmApp + 3

	nimAdd        = 0
	nimModify     = 1
	nimDelete     = 2
	nifMessage    = 0x01
	nifIcon       = 0x02
	nifTip        = 0x04
	nifInfo       = 0x10
	niifUser      = 0x04
	niifLargeIcon = 0x20

	mfString       = 0x0000
	mfGrayed       = 0x0001
	mfChecked      = 0x0008
	mfSeparator    = 0x0800
	tpmRightButton = 0x0002
	tpmNoNotify    = 0x0080
	tpmReturnCmd   = 0x0100

	modNoRepeat = 0x4000

	eventSystemForeground  = 0x0003
	wineventOutOfContext   = 0x0000
	wineventSkipOwnProcess = 0x0002

	gmemMoveable = 0x0002
	smCxIcon     = 11
	smCxSmIcon   = 49
	swRestore    = 9
	swShowNormal = 1
	mbIconError  = 0x10

	processQueryLimitedInformation = 0x1000
	errorAlreadyExists             = 183

	hkeyCurrentUser = 0x80000001
	keySetValue     = 0x0002
	keyQueryValue   = 0x0001
	regSZ           = 1
)

func utf16p(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		p, _ = syscall.UTF16PtrFromString("")
	}
	return p
}

// has reports whether an optional API exists on this Windows version.
func has(p *syscall.LazyProc) bool { return p.Find() == nil }

func currentThreadID() uintptr {
	r, _, _ := pGetCurrentThreadId.Call()
	return r
}

func windowText(h uintptr) string {
	var buf [512]uint16
	n, _, _ := pGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:n])
}

func className(h uintptr) string {
	var buf [128]uint16
	n, _, _ := pGetClassNameW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:n])
}

func processPath(h uintptr) string {
	var pid uint32
	pGetWindowThreadProcessId.Call(h, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return ""
	}
	ph, _, _ := pOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if ph == 0 {
		return ""
	}
	defer pCloseHandle.Call(ph)
	var buf [1024]uint16
	size := uint32(len(buf))
	r, _, _ := pQueryFullProcessImageNameW.Call(ph, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:size])
}

// fileDescription reads "FileDescription" from an exe's version resource
// ("Microsoft PowerPoint", "KakaoTalk"…).
func fileDescription(path string) string {
	if !has(pGetFileVersionInfoSizeW) {
		return ""
	}
	p := utf16p(path)
	size, _, _ := pGetFileVersionInfoSizeW.Call(uintptr(unsafe.Pointer(p)), 0)
	if size == 0 || size > 4<<20 {
		return ""
	}
	buf := make([]byte, size)
	if r, _, _ := pGetFileVersionInfoW.Call(uintptr(unsafe.Pointer(p)), 0, size, uintptr(unsafe.Pointer(&buf[0]))); r == 0 {
		return ""
	}
	base := uintptr(unsafe.Pointer(&buf[0]))
	query := func(key string) (int, int) {
		var ptr uintptr
		var n uint32
		r, _, _ := pVerQueryValueW.Call(base, uintptr(unsafe.Pointer(utf16p(key))), uintptr(unsafe.Pointer(&ptr)), uintptr(unsafe.Pointer(&n)))
		if r == 0 || ptr < base || ptr >= base+uintptr(len(buf)) {
			return 0, 0
		}
		return int(ptr - base), int(n)
	}
	var langs [][2]uint16
	if off, n := query(`\VarFileInfo\Translation`); n >= 4 {
		for i := 0; i+4 <= n && off+i+4 <= len(buf); i += 4 {
			langs = append(langs, [2]uint16{le16(buf[off+i:]), le16(buf[off+i+2:])})
		}
	}
	langs = append(langs, [2]uint16{0x0412, 0x04B0}, [2]uint16{0x0409, 0x04B0}, [2]uint16{0x0409, 0x04E4})
	for _, l := range langs {
		off, n := query(fmt.Sprintf(`\StringFileInfo\%04x%04x\FileDescription`, l[0], l[1]))
		if n <= 0 {
			continue
		}
		end := off + 2*n
		if end > len(buf) {
			end = len(buf)
		}
		u := make([]uint16, 0, n)
		for i := off; i+1 < end; i += 2 {
			c := le16(buf[i:])
			if c == 0 {
				break
			}
			u = append(u, c)
		}
		if s := syscall.UTF16ToString(u); s != "" {
			return s
		}
	}
	return ""
}

func le16(b []byte) uint16 { return uint16(b[0]) | uint16(b[1])<<8 }

func openClipboard(hwnd uintptr) bool {
	for i := 0; i < 10; i++ {
		if r, _, _ := pOpenClipboard.Call(hwnd); r != 0 {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return false
}

// readClipboard copies one format's bytes. The clipboard must be open.
func readClipboard(format uint32) []byte {
	h, _, _ := pGetClipboardData.Call(uintptr(format))
	if h == 0 {
		return nil
	}
	ptr, _, _ := pGlobalLock.Call(h)
	if ptr == 0 {
		return nil
	}
	defer pGlobalUnlock.Call(h)
	n, _, _ := pGlobalSize.Call(h)
	if n == 0 || n > 512<<20 {
		return nil
	}
	out := make([]byte, n)
	copy(out, unsafe.Slice((*byte)(unsafe.Pointer(ptr)), n))
	return out
}

// writeClipboard hands a copy of data to the clipboard. It must be open.
func writeClipboard(format uint32, data []byte) error {
	h, _, _ := pGlobalAlloc.Call(gmemMoveable, uintptr(len(data)))
	if h == 0 {
		return errors.New("GlobalAlloc failed")
	}
	ptr, _, _ := pGlobalLock.Call(h)
	if ptr == 0 {
		pGlobalFree.Call(h)
		return errors.New("GlobalLock failed")
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(ptr)), len(data)), data)
	pGlobalUnlock.Call(h)
	if r, _, err := pSetClipboardData.Call(uintptr(format), h); r == 0 {
		pGlobalFree.Call(h)
		return fmt.Errorf("SetClipboardData: %v", err)
	}
	return nil // the clipboard owns h now
}

func clipboardFormatName(f uint32) string {
	var buf [256]uint16
	n, _, _ := pGetClipboardFormatNameW.Call(uintptr(f), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:n])
}

func registerClipboardFormat(name string) uint32 {
	r, _, _ := pRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(utf16p(name))))
	return uint32(r)
}

func shellOpen(target string) error {
	r, _, _ := pShellExecuteW.Call(0, uintptr(unsafe.Pointer(utf16p("open"))), uintptr(unsafe.Pointer(utf16p(target))), 0, 0, swShowNormal)
	if r <= 32 {
		return fmt.Errorf("열 수 없어요 (%d)", r)
	}
	return nil
}

func messageBox(title, text string) {
	pMessageBoxW.Call(0, uintptr(unsafe.Pointer(utf16p(text))), uintptr(unsafe.Pointer(utf16p(title))), mbIconError)
}

// picturesFolder is the user's Pictures folder (follows OneDrive/redirects).
func picturesFolder() string {
	if !has(pSHGetKnownFolderPath) {
		return ""
	}
	id := wGUID{0x33E28130, 0x4E1E, 0x4676, [8]byte{0x83, 0x5A, 0x98, 0x39, 0x5C, 0x3B, 0xC3, 0xBB}}
	var p uintptr
	r, _, _ := pSHGetKnownFolderPath.Call(uintptr(unsafe.Pointer(&id)), 0, 0, uintptr(unsafe.Pointer(&p)))
	if r != 0 || p == 0 {
		return ""
	}
	defer pCoTaskMemFree.Call(p)
	return utf16PtrToString(p)
}

func utf16PtrToString(p uintptr) string {
	if p == 0 {
		return ""
	}
	n := 0
	for ; n < 32768; n++ {
		if *(*uint16)(unsafe.Pointer(p + uintptr(2*n))) == 0 {
			break
		}
	}
	return syscall.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(p)), n))
}

// ---------- registry (HKCU\...\Run) ----------

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

func regSetRun(name, value string) error {
	var k uintptr
	if r, _, _ := pRegOpenKeyExW.Call(hkeyCurrentUser, uintptr(unsafe.Pointer(utf16p(runKey))), 0, keySetValue|keyQueryValue, uintptr(unsafe.Pointer(&k))); r != 0 {
		return fmt.Errorf("RegOpenKeyEx: %d", r)
	}
	defer pRegCloseKey.Call(k)
	if value == "" {
		r, _, _ := pRegDeleteValueW.Call(k, uintptr(unsafe.Pointer(utf16p(name))))
		if r != 0 && r != 2 { // 2 = not found
			return fmt.Errorf("RegDeleteValue: %d", r)
		}
		return nil
	}
	u, err := syscall.UTF16FromString(value)
	if err != nil {
		return err
	}
	r, _, _ := pRegSetValueExW.Call(k, uintptr(unsafe.Pointer(utf16p(name))), 0, regSZ, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)*2))
	if r != 0 {
		return fmt.Errorf("RegSetValueEx: %d", r)
	}
	return nil
}

// ---------- DPAPI ----------

func dpapi(protect bool, in []byte) ([]byte, error) {
	if len(in) == 0 {
		return nil, errors.New("empty")
	}
	inb := wDataBlob{Size: uint32(len(in)), Data: uintptr(unsafe.Pointer(&in[0]))}
	var out wDataBlob
	var r uintptr
	var err error
	if protect {
		r, _, err = pCryptProtectData.Call(uintptr(unsafe.Pointer(&inb)), uintptr(unsafe.Pointer(utf16p("Screenmit"))), 0, 0, 0, 1, uintptr(unsafe.Pointer(&out)))
	} else {
		r, _, err = pCryptUnprotectData.Call(uintptr(unsafe.Pointer(&inb)), 0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&out)))
	}
	if r == 0 {
		return nil, fmt.Errorf("DPAPI: %v", err)
	}
	defer pLocalFree.Call(out.Data)
	res := make([]byte, out.Size)
	copy(res, unsafe.Slice((*byte)(unsafe.Pointer(out.Data)), out.Size))
	return res, nil
}
