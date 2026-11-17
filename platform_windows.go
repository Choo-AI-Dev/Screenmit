//go:build windows

package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/Choo-AI-Dev/screenmit/internal/config"
	"github.com/Choo-AI-Dev/screenmit/internal/drive"
	"github.com/Choo-AI-Dev/screenmit/internal/imgclip"
)

const logToStderr = false

const windowClass = "ScreenmitHiddenWindow"

//go:embed res/screenmit.ico
var icoNormal []byte

//go:embed res/screenmit-paused.ico
var icoPaused []byte

// The GUI must stay on one OS thread.
func init() { runtime.LockOSThread() }

func addPlatformFlags() {}

type winInfo struct {
	hwnd  uintptr
	app   string
	exe   string
	title string
}

type winPlatform struct {
	app        *App
	background bool
	mutex      uintptr

	hwnd     uintptr
	uiThread uintptr
	nid      wNotifyIconData
	icons    struct{ normal, paused, big uintptr }
	tray     bool

	taskbarCreated uint32
	pngFmt         uint32
	noHistoryFmt   uint32
	lastSeq        uintptr
	lastFg         winInfo
	hook           uintptr
	hotkeyOn       bool
	lastLaunch     time.Time
	descCache      map[string]string

	uiq         chan func()
	trayPending atomic.Bool
	trayMu      sync.Mutex
	trayState   TrayState
}

var (
	thePlatform  *winPlatform
	wndProcCB    = syscall.NewCallback(wndProc)
	winEventCB   = syscall.NewCallback(winEventProc)
	enumWindowCB = syscall.NewCallback(enumWindowProc)
	enumFound    uintptr
)

func newPlatform() Platform {
	p := &winPlatform{uiq: make(chan func(), 256), descCache: map[string]string{}}
	thePlatform = p
	return p
}

func (p *winPlatform) Name() string { return "windows" }

func (p *winPlatform) ConfigDir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		d = os.Getenv("APPDATA")
	}
	return filepath.Join(d, "Screenmit")
}

func (p *winPlatform) DefaultFolder() string {
	pics := picturesFolder()
	if pics == "" {
		h, _ := os.UserHomeDir()
		pics = filepath.Join(h, "Pictures")
	}
	return filepath.Join(pics, "Screenmit")
}

func (p *winPlatform) PCName() string {
	h, err := os.Hostname()
	if err != nil {
		return "이 PC"
	}
	return h
}

// Claim keeps one Screenmit per Windows session. A second start shows the
// first one's gallery instead.
func (p *winPlatform) Claim(background bool) bool {
	p.background = background
	h, _, err := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(utf16p(`Local\Screenmit-SingleInstance`))))
	if h == 0 {
		return true
	}
	if errno, ok := err.(syscall.Errno); ok && errno == errorAlreadyExists {
		pCloseHandle.Call(h)
		if !background {
			if w, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(utf16p(windowClass))), 0); w != 0 {
				pAllowSetForegroundWindow.Call(0xFFFFFFFF) // ASFW_ANY
				pPostMessageW.Call(w, wmShowGallery, 0, 0)
			}
		}
		return false
	}
	p.mutex = h
	return true
}

func (p *winPlatform) Fatal(err error) {
	messageBox("Screenmit", "Screenmit 을 시작하지 못했어요.\n\n"+err.Error())
}

// ---------- the OS thread ----------

func (p *winPlatform) Run(a *App) error {
	p.app = a
	p.uiThread = currentThreadID()
	if has(pSetProcessDpiAwarenessContext) {
		pSetProcessDpiAwarenessContext.Call(^uintptr(3)) // PER_MONITOR_AWARE_V2 (-4)
	}
	pCoInitializeEx.Call(0, 2) // apartment threaded, for ShellExecute

	hinst, _, _ := pGetModuleHandleW.Call(0)
	wc := wWndClassEx{WndProc: wndProcCB, Instance: hinst, ClassName: utf16p(windowClass)}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return fmt.Errorf("RegisterClassEx: %v", err)
	}
	hwnd, _, err := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(utf16p(windowClass))), uintptr(unsafe.Pointer(utf16p("Screenmit"))),
		0, 0, 0, 0, 0, 0, 0, hinst, 0)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowEx: %v", err)
	}
	p.hwnd = hwnd
	r, _, _ := pRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(utf16p("TaskbarCreated"))))
	p.taskbarCreated = uint32(r)

	size, _, _ := pGetSystemMetrics.Call(smCxSmIcon)
	big, _, _ := pGetSystemMetrics.Call(smCxIcon)
	p.icons.normal = iconFromICO(icoNormal, int(size))
	p.icons.paused = iconFromICO(icoPaused, int(size))
	p.icons.big = iconFromICO(icoNormal, int(big))
	p.addTray()

	p.pngFmt = registerClipboardFormat("PNG")
	p.noHistoryFmt = registerClipboardFormat("CanIncludeInClipboardHistory")
	p.lastSeq, _, _ = pGetClipboardSequenceNumber.Call() // don't save what was copied before we started
	if r, _, err := pAddClipboardFormatListener.Call(hwnd); r == 0 {
		log.Printf("AddClipboardFormatListener: %v", err)
	}
	p.hook, _, _ = pSetWinEventHook.Call(eventSystemForeground, eventSystemForeground, 0, winEventCB, 0, 0, wineventOutOfContext|wineventSkipOwnProcess)
	if fg, _, _ := pGetForegroundWindow.Call(); fg != 0 {
		p.noteForeground(fg)
	}

	cfg := a.Config()
	if err := p.applyHotkey(cfg.HotkeySpec()); err != nil {
		a.SetHotkeyError(fmt.Sprintf("%s 은(는) 다른 프로그램이 쓰고 있어요. 다른 단축키를 골라 주세요", hotkeyLabel(cfg.HotkeySpec())))
	}
	if err := p.SetAutostart(cfg.AutostartOn()); err != nil {
		log.Printf("autostart: %v", err)
	}
	p.applyTray()
	if !p.background {
		p.openGallery()
	}

	var msg wMsg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	pRemoveClipboardFormatListener.Call(hwnd)
	if p.hook != 0 {
		pUnhookWinEvent.Call(p.hook)
	}
	if p.hotkeyOn {
		pUnregisterHotKey.Call(hwnd, 1)
	}
	p.removeTray()
	return nil
}

func wndProc(hwnd, msg, wp, lp uintptr) (ret uintptr) {
	p := thePlatform
	defer func() {
		if r := recover(); r != nil {
			log.Printf("wndproc panic: %v", r)
			ret = 0
		}
	}()
	switch uint32(msg) {
	case wmClipboardUpdate:
		p.onClipboard()
		return 0
	case wmTray:
		switch uint32(lp) & 0xFFFF {
		case wmLButtonUp, ninBalloonUserClick:
			p.openGallery()
		case wmRButtonUp, wmContextMenu:
			p.showMenu()
		}
		return 0
	case wmHotkey, wmShowGallery:
		p.openGallery()
		return 0
	case wmRunQueue:
		for {
			select {
			case f := <-p.uiq:
				f()
			default:
				return 0
			}
		}
	case wmQueryEndSession:
		return 1
	case wmEndSession:
		if wp != 0 && p.app != nil {
			p.app.Shutdown()
		}
		return 0
	case wmClose:
		pDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	if p != nil && p.taskbarCreated != 0 && uint32(msg) == p.taskbarCreated {
		p.tray = false // Explorer restarted: the icon is gone
		p.addTray()
		p.applyTray()
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}

// onUI runs f on the window thread and waits. Safe from any goroutine.
func (p *winPlatform) onUI(f func()) {
	if p.hwnd == 0 || currentThreadID() == p.uiThread {
		f()
		return
	}
	done := make(chan struct{})
	p.uiq <- func() { defer close(done); f() }
	pPostMessageW.Call(p.hwnd, wmRunQueue, 0, 0)
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		log.Printf("UI thread did not answer")
	}
}

// later runs f on the window thread without waiting.
func (p *winPlatform) later(f func()) {
	if p.hwnd == 0 {
		return
	}
	select {
	case p.uiq <- f:
		pPostMessageW.Call(p.hwnd, wmRunQueue, 0, 0)
	default:
	}
}

func (p *winPlatform) Quit() {
	if p.hwnd != 0 {
		pPostMessageW.Call(p.hwnd, wmClose, 0, 0)
	}
}

// ---------- clipboard ----------

func (p *winPlatform) onClipboard() {
	seq, _, _ := pGetClipboardSequenceNumber.Call()
	if seq == p.lastSeq {
		return
	}
	p.lastSeq = seq
	if p.app == nil || p.app.Paused() {
		return
	}
	if owner, _, _ := pGetClipboardOwner.Call(); owner != 0 && owner == p.hwnd {
		return // our own "copy" from the gallery
	}
	if !openClipboard(p.hwnd) {
		log.Printf("clipboard busy, skipped one change")
		return
	}
	var formats []imgclip.Format
	f := uintptr(0)
	for i := 0; i < 400; i++ {
		f, _, _ = pEnumClipboardFormats.Call(f)
		if f == 0 {
			break
		}
		ff := imgclip.Format{ID: uint32(f)}
		if f >= 0xC000 {
			ff.Name = clipboardFormatName(uint32(f))
		}
		formats = append(formats, ff)
	}
	noHistory := false
	for _, ff := range formats {
		if ff.ID == p.noHistoryFmt && p.noHistoryFmt != 0 {
			if b := readClipboard(ff.ID); len(b) >= 4 && b[0]|b[1]|b[2]|b[3] == 0 {
				noHistory = true
			}
		}
	}
	d := imgclip.Decide(formats, p.app.Config().SkipDocuments(), noHistory)
	var data []byte
	if d.Source != imgclip.SourceNone {
		data = readClipboard(d.FmtID)
		if len(data) == 0 && d.Source != imgclip.SourceDIB {
			// The app offered PNG/DIBV5 but did not render it: ask for a DIB.
			d.Source = imgclip.SourceDIB
			data = readClipboard(imgclip.CF_DIB)
		}
	}
	pCloseClipboard.Call()
	if d.Skip != "" {
		log.Printf("clipboard picture skipped (%s)", d.Skip)
		return
	}
	if len(data) == 0 {
		return
	}
	src := p.sourceWindow()
	p.app.Capture(Capture{Data: data, Kind: d.Source, App: src.app, Exe: src.exe, Title: src.title, Time: time.Now()})
}

// Windows that do the capturing; the screenshot is "from" the window before them.
var captureTools = map[string]bool{
	"screenclippinghost.exe": true, "snippingtool.exe": true, "screensketch.exe": true,
	"shellexperiencehost.exe": true, "shellhost.exe": true, "searchhost.exe": true,
	"startmenuexperiencehost.exe": true, "lockapp.exe": true, "screenmit.exe": true,
}

func winEventProc(hook, event, hwnd, idObject, idChild, thread, ms uintptr) uintptr {
	defer func() { _ = recover() }()
	// idObject is a 32-bit LONG: the upper half of the register is undefined.
	if p := thePlatform; p != nil && int32(idObject) == 0 {
		p.noteForeground(hwnd)
	}
	return 0
}

func (p *winPlatform) noteForeground(hwnd uintptr) {
	if hwnd == 0 || hwnd == p.hwnd {
		return
	}
	w := p.windowInfo(hwnd)
	if w.exe != "" && !captureTools[strings.ToLower(w.exe)] {
		p.lastFg = w
	}
}

func (p *winPlatform) sourceWindow() winInfo {
	if fg, _, _ := pGetForegroundWindow.Call(); fg != 0 && fg != p.hwnd {
		if w := p.windowInfo(fg); w.exe != "" && !captureTools[strings.ToLower(w.exe)] {
			return w
		}
	}
	if p.lastFg.hwnd != 0 {
		// Title may have changed since (new tab, saved file…): read it again.
		if t := windowText(p.lastFg.hwnd); t != "" {
			w := p.lastFg
			w.title = t
			return w
		}
	}
	return p.lastFg
}

func (p *winPlatform) windowInfo(hwnd uintptr) winInfo {
	path := processPath(hwnd)
	w := winInfo{hwnd: hwnd, title: windowText(hwnd)}
	if path == "" {
		return w
	}
	w.exe = filepath.Base(path)
	desc, ok := p.descCache[path]
	if !ok {
		desc = fileDescription(path)
		p.descCache[path] = desc
	}
	w.app = desc
	switch strings.ToLower(w.exe) {
	case "applicationframehost.exe": // store apps: the window title is the app
		w.app = w.title
	case "explorer.exe":
		if w.title == "" {
			w.app = "바탕 화면"
		}
	}
	if w.app == "" {
		w.app = strings.TrimSuffix(w.exe, filepath.Ext(w.exe))
	}
	return w
}

func (p *winPlatform) CopyImage(pngData []byte, img image.Image) error {
	dib := imgclip.EncodeDIB24(img)
	var err error
	p.onUI(func() {
		if !openClipboard(p.hwnd) {
			err = errors.New("다른 프로그램이 클립보드를 쓰고 있어요. 잠시 뒤 다시 해 주세요")
			return
		}
		defer pCloseClipboard.Call()
		pEmptyClipboard.Call()
		if err = writeClipboard(imgclip.CF_DIB, dib); err != nil {
			return
		}
		if p.pngFmt != 0 {
			_ = writeClipboard(p.pngFmt, pngData)
		}
	})
	return err
}

// ---------- tray ----------

func iconFromICO(ico []byte, size int) uintptr {
	if size <= 0 {
		size = 16
	}
	entries, err := imgclip.ParseICO(ico)
	if err != nil {
		return 0
	}
	e, ok := imgclip.PickIcon(entries, size)
	if !ok || len(e.Data) == 0 {
		return 0
	}
	h, _, _ := pCreateIconFromResourceEx.Call(uintptr(unsafe.Pointer(&e.Data[0])), uintptr(len(e.Data)), 1, 0x00030000, uintptr(size), uintptr(size), 0)
	return h
}

func (p *winPlatform) addTray() {
	p.nid = wNotifyIconData{Wnd: p.hwnd, ID: 1, Flags: nifMessage | nifIcon | nifTip, CallbackMessage: wmTray, Icon: p.icons.normal}
	p.nid.Size = uint32(unsafe.Sizeof(p.nid))
	putUTF16(p.nid.Tip[:], "Screenmit")
	for i := 0; i < 5; i++ { // Explorer may still be starting at sign-in
		if r, _, _ := pShell_NotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&p.nid))); r != 0 {
			p.tray = true
			return
		}
		time.Sleep(time.Second)
	}
	log.Printf("tray icon could not be added")
}

func (p *winPlatform) removeTray() {
	if p.tray {
		pShell_NotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&p.nid)))
		p.tray = false
	}
}

func (p *winPlatform) UpdateTray(t TrayState) {
	p.trayMu.Lock()
	p.trayState = t
	p.trayMu.Unlock()
	if p.trayPending.CompareAndSwap(false, true) {
		p.later(func() {
			p.trayPending.Store(false)
			p.applyTray()
		})
	}
}

func (p *winPlatform) applyTray() {
	if !p.tray {
		return
	}
	p.trayMu.Lock()
	t := p.trayState
	p.trayMu.Unlock()
	if p.app != nil {
		t.Paused = p.app.Paused()
	}
	tip := fmt.Sprintf("Screenmit · 오늘 %d장", t.Today)
	if t.Paused {
		tip = "Screenmit · 일시 정지됨"
	}
	p.nid.Flags = nifIcon | nifTip
	p.nid.Icon = p.icons.normal
	if t.Paused {
		p.nid.Icon = p.icons.paused
	}
	putUTF16(p.nid.Tip[:], tip)
	pShell_NotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&p.nid)))
}

func (p *winPlatform) Notify(title, msg string) {
	p.later(func() {
		if !p.tray {
			return
		}
		n := p.nid
		n.Flags = nifInfo
		putUTF16(n.InfoTitle[:], title)
		putUTF16(n.Info[:], msg)
		n.InfoFlags = niifUser | niifLargeIcon
		n.BalloonIcon = p.icons.big
		pShell_NotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&n)))
	})
}

func (p *winPlatform) showMenu() {
	m, _, _ := pCreatePopupMenu.Call()
	if m == 0 {
		return
	}
	defer pDestroyMenu.Call(m)
	add := func(flags uint32, id uintptr, text string) {
		var t uintptr
		if text != "" {
			t = uintptr(unsafe.Pointer(utf16p(text)))
		}
		pAppendMenuW.Call(m, uintptr(flags), id, t)
	}
	paused := p.app.Paused()
	cfg := p.app.Config()
	add(mfString, 1, "스크린샷 보기")
	add(mfSeparator, 0, "")
	if paused {
		add(mfString, 2, "다시 모으기")
	} else {
		add(mfString, 2, "잠깐 멈추기")
	}
	add(mfString, 3, "저장 폴더 열기")
	add(mfSeparator, 0, "")
	auto := uint32(mfString)
	if cfg.AutostartOn() {
		auto |= mfChecked
	}
	add(auto, 4, "컴퓨터 켤 때 실행")
	add(mfSeparator, 0, "")
	add(mfString, 9, "Screenmit 끝내기")
	pSetMenuDefaultItem.Call(m, 1, 0)

	var pt wPoint
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(p.hwnd)
	cmd, _, _ := pTrackPopupMenu.Call(m, tpmRightButton|tpmReturnCmd|tpmNoNotify, uintptr(pt.X), uintptr(pt.Y), 0, p.hwnd, 0)
	pPostMessageW.Call(p.hwnd, wmNull, 0, 0)

	switch cmd {
	case 1:
		p.openGallery()
	case 2:
		p.app.SetPaused(!paused)
		p.applyTray()
	case 3:
		_ = p.app.OpenFolder()
	case 4:
		on := !cfg.AutostartOn()
		go func() {
			if err := p.app.ApplySettings(settingsAutostart(on)); err != nil {
				log.Printf("autostart: %v", err)
			}
		}()
	case 9:
		p.Quit()
	}
}

// ---------- gallery window ----------

func enumWindowProc(hwnd, lparam uintptr) uintptr {
	if v, _, _ := pIsWindowVisible.Call(hwnd); v == 0 {
		return 1
	}
	if className(hwnd) != "Chrome_WidgetWin_1" {
		return 1
	}
	t := windowText(hwnd)
	if t == "Screenmit" || strings.HasPrefix(t, "Screenmit - ") || strings.HasPrefix(t, "Screenmit – ") {
		enumFound = hwnd
		return 0
	}
	return 1
}

func findGalleryWindow() uintptr {
	enumFound = 0
	pEnumWindows.Call(enumWindowCB, 0)
	return enumFound
}

func (p *winPlatform) OpenGallery(string) { p.later(p.openGallery) }

// openGallery brings the gallery window forward, or opens it as an app
// window (no tabs or address bar) in Edge or Chrome.
func (p *winPlatform) openGallery() {
	if w := findGalleryWindow(); w != 0 {
		if ic, _, _ := pIsIconic.Call(w); ic != 0 {
			pShowWindow.Call(w, swRestore)
		}
		pSetForegroundWindow.Call(w)
		return
	}
	if time.Since(p.lastLaunch) < 3*time.Second {
		return // it is still starting (double click)
	}
	p.lastLaunch = time.Now()
	url := p.app.GalleryURL()
	if b := findBrowser(); b != "" {
		cmd := exec.Command(b, "--app="+url, "--window-size=1280,860")
		if err := cmd.Start(); err == nil {
			go cmd.Wait()
			return
		}
	}
	if err := shellOpen(url); err != nil {
		log.Printf("open gallery: %v", err)
	}
}

func findBrowser() string {
	pf, pf86, local := os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA")
	for _, c := range []string{
		filepath.Join(pf86, `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(pf, `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(local, `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(pf, `Google\Chrome\Application\chrome.exe`),
		filepath.Join(pf86, `Google\Chrome\Application\chrome.exe`),
		filepath.Join(local, `Google\Chrome\Application\chrome.exe`),
	} {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

func (p *winPlatform) OpenURL(u string) error {
	var err error
	p.onUI(func() { err = shellOpen(u) })
	return err
}

func explorer(args string) error {
	exe := filepath.Join(os.Getenv("WINDIR"), "explorer.exe")
	cmd := exec.Command(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe ` + args}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

func (p *winPlatform) Reveal(path string) error {
	if _, err := os.Stat(path); err != nil {
		return errors.New("파일이 없어요")
	}
	return explorer(`/select,"` + path + `"`)
}

func (p *winPlatform) OpenPath(path string) error { return explorer(`"` + path + `"`) }

// ---------- settings that touch Windows ----------

func (p *winPlatform) SetAutostart(on bool) error {
	if !on {
		return regSetRun("Screenmit", "")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return regSetRun("Screenmit", `"`+exe+`" --background`)
}

func hotkeyLabel(spec string) string { return strings.ReplaceAll(spec, "+", " + ") }

func (p *winPlatform) ApplyHotkey(spec string) error {
	var err error
	p.onUI(func() { err = p.applyHotkey(spec) })
	if err == nil {
		p.app.SetHotkeyError("")
	}
	return err
}

func (p *winPlatform) applyHotkey(spec string) error {
	if p.hotkeyOn {
		pUnregisterHotKey.Call(p.hwnd, 1)
		p.hotkeyOn = false
	}
	hk, ok, err := config.ParseHotkey(spec)
	if err != nil || !ok {
		return err
	}
	if r, _, _ := pRegisterHotKey.Call(p.hwnd, 1, uintptr(hk.Mods|modNoRepeat), uintptr(hk.VK)); r == 0 {
		return fmt.Errorf("%s 은(는) 다른 프로그램이 쓰고 있어요", hotkeyLabel(spec))
	}
	p.hotkeyOn = true
	return nil
}

func (p *winPlatform) TokenStore(dir string) drive.TokenStore {
	return dpapiTokens(filepath.Join(dir, "google-token.bin"))
}

// dpapiTokens keeps the Google login encrypted with the Windows account.
type dpapiTokens string

func (f dpapiTokens) Load() (drive.Token, error) {
	var t drive.Token
	b, err := os.ReadFile(string(f))
	if err != nil {
		return t, err
	}
	plain, err := dpapi(false, b)
	if err != nil {
		return t, err
	}
	return t, json.Unmarshal(plain, &t)
}

func (f dpapiTokens) Save(t drive.Token) error {
	b, _ := json.Marshal(t)
	enc, err := dpapi(true, b)
	if err != nil {
		return err
	}
	return os.WriteFile(string(f), enc, 0o600)
}

func (f dpapiTokens) Clear() error {
	err := os.Remove(string(f))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
