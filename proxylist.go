package main

import (
	"net/url"
	"strings"
)

// parseProxyList reads a Windows proxy string: "host:port",
// "http=h:p;https=h2:p2", or a PAC answer "h:p; h2:p2". It returns the
// first proxy that applies, or nil for a direct connection.
func parseProxyList(s string, https bool) *url.URL {
	var plain, httpP, httpsP string
	var parts []string
	for _, seg := range strings.Split(s, ";") {
		seg = strings.TrimSpace(seg)
		if up := strings.ToUpper(seg); strings.HasPrefix(up, "PROXY ") || strings.HasPrefix(up, "HTTPS ") {
			seg = strings.TrimSpace(seg[6:])
		}
		parts = append(parts, strings.Fields(seg)...)
	}
	for _, part := range parts {
		if k, v, ok := strings.Cut(part, "="); ok {
			switch strings.ToLower(k) {
			case "https":
				if httpsP == "" {
					httpsP = v
				}
			case "http":
				if httpP == "" {
					httpP = v
				}
			}
			continue
		}
		if strings.EqualFold(part, "DIRECT") {
			continue
		}
		if plain == "" {
			plain = part
		}
	}
	pick := plain
	if https && httpsP != "" {
		pick = httpsP
	} else if pick == "" && httpP != "" {
		pick = httpP
	}
	pick = strings.TrimSpace(pick)
	if pick == "" {
		return nil
	}
	if !strings.Contains(pick, "://") {
		pick = "http://" + pick
	}
	u, err := url.Parse(pick)
	if err != nil || u.Host == "" {
		return nil
	}
	return u
}

// trimPNG drops bytes after the IEND chunk (clipboard memory is often
// rounded up to a larger size).
func trimPNG(b []byte) []byte {
	const sig = 8
	if len(b) < sig {
		return b
	}
	i := sig
	for i+8 <= len(b) {
		n := int(uint32(b[i])<<24 | uint32(b[i+1])<<16 | uint32(b[i+2])<<8 | uint32(b[i+3]))
		typ := string(b[i+4 : i+8])
		end := i + 12 + n
		if n < 0 || end > len(b) {
			return b
		}
		if typ == "IEND" {
			return b[:end]
		}
		i = end
	}
	return b
}
