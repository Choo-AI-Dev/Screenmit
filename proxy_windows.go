//go:build windows

package main

import (
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Go does not read the Windows proxy settings by itself. Company networks
// often need them (fixed proxy or a PAC script), so we ask WinHTTP.

var (
	winhttp                         = syscall.NewLazyDLL("winhttp.dll")
	pWinHttpGetIEProxyConfigForUser = winhttp.NewProc("WinHttpGetIEProxyConfigForCurrentUser")
	pWinHttpOpen                    = winhttp.NewProc("WinHttpOpen")
	pWinHttpCloseHandle             = winhttp.NewProc("WinHttpCloseHandle")
	pWinHttpGetProxyForUrl          = winhttp.NewProc("WinHttpGetProxyForUrl")
	pGlobalFreeProxy                = kernel32.NewProc("GlobalFree")
)

type proxyCache struct {
	mu   sync.Mutex
	m    map[string]proxyEntry
	logd map[string]bool
}

type proxyEntry struct {
	u   *url.URL
	exp time.Time
}

var proxies = &proxyCache{m: map[string]proxyEntry{}, logd: map[string]bool{}}

func (p *winPlatform) Proxy(r *http.Request) (*url.URL, error) {
	if u, err := http.ProxyFromEnvironment(r); u != nil || err != nil {
		return u, err
	}
	host := r.URL.Hostname()
	if host == "127.0.0.1" || host == "localhost" {
		return nil, nil
	}
	key := r.URL.Scheme + "://" + r.URL.Host
	proxies.mu.Lock()
	if e, ok := proxies.m[key]; ok && time.Now().Before(e.exp) {
		proxies.mu.Unlock()
		return e.u, nil
	}
	proxies.mu.Unlock()
	u := systemProxy(key + "/")
	proxies.mu.Lock()
	proxies.m[key] = proxyEntry{u, time.Now().Add(10 * time.Minute)}
	if u != nil && !proxies.logd[u.Host] {
		proxies.logd[u.Host] = true
		log.Printf("using proxy %s for %s", u.Host, key)
	}
	proxies.mu.Unlock()
	return u, nil
}

func systemProxy(target string) *url.URL {
	if !has(pWinHttpGetIEProxyConfigForUser) {
		return nil
	}
	var ie wIEProxyConfig
	if r, _, _ := pWinHttpGetIEProxyConfigForUser.Call(uintptr(unsafe.Pointer(&ie))); r == 0 {
		return nil
	}
	defer func() {
		for _, s := range []uintptr{ie.AutoConfigURL, ie.Proxy, ie.ProxyBypass} {
			if s != 0 {
				pGlobalFreeProxy.Call(s)
			}
		}
	}()
	if ie.AutoDetect != 0 || ie.AutoConfigURL != 0 {
		if u := autoProxy(target, ie); u != nil {
			return u
		}
	}
	if ie.Proxy != 0 {
		return parseProxyList(utf16PtrToString(ie.Proxy), strings.HasPrefix(target, "https"))
	}
	return nil
}

func autoProxy(target string, ie wIEProxyConfig) *url.URL {
	s, _, _ := pWinHttpOpen.Call(uintptr(unsafe.Pointer(utf16p("Screenmit"))), 1 /* NO_PROXY */, 0, 0, 0)
	if s == 0 {
		return nil
	}
	defer pWinHttpCloseHandle.Call(s)
	var opt wAutoProxyOptions
	if ie.AutoConfigURL != 0 {
		opt.Flags |= 2 // CONFIG_URL
		opt.AutoConfigURL = ie.AutoConfigURL
	}
	if ie.AutoDetect != 0 {
		opt.Flags |= 1 // AUTO_DETECT
		opt.AutoDetectFlags = 1 | 2
	}
	opt.AutoLogon = 1
	var info wProxyInfo
	r, _, _ := pWinHttpGetProxyForUrl.Call(s, uintptr(unsafe.Pointer(utf16p(target))), uintptr(unsafe.Pointer(&opt)), uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		return nil
	}
	defer func() {
		if info.Proxy != 0 {
			pGlobalFreeProxy.Call(info.Proxy)
		}
		if info.ProxyBypass != 0 {
			pGlobalFreeProxy.Call(info.ProxyBypass)
		}
	}()
	if info.AccessType != 3 /* NAMED_PROXY */ || info.Proxy == 0 {
		return nil
	}
	return parseProxyList(utf16PtrToString(info.Proxy), true)
}
