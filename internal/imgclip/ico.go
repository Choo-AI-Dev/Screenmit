package imgclip

import "errors"

// IconEntry is one image inside an .ico file.
type IconEntry struct {
	Size int    // width in pixels (256 for 0)
	Data []byte // BMP (DIB with AND mask) or PNG bytes
}

// ParseICO splits an .ico file into its images.
func ParseICO(b []byte) ([]IconEntry, error) {
	if len(b) < 6 || le.Uint16(b[0:]) != 0 || le.Uint16(b[2:]) != 1 {
		return nil, errors.New("ico: bad header")
	}
	n := int(le.Uint16(b[4:]))
	if len(b) < 6+16*n {
		return nil, errors.New("ico: short directory")
	}
	out := make([]IconEntry, 0, n)
	for i := 0; i < n; i++ {
		e := b[6+16*i:]
		w := int(e[0])
		if w == 0 {
			w = 256
		}
		size := int(le.Uint32(e[8:]))
		off := int(le.Uint32(e[12:]))
		if off < 0 || size <= 0 || off+size > len(b) {
			return nil, errors.New("ico: entry out of range")
		}
		out = append(out, IconEntry{Size: w, Data: b[off : off+size]})
	}
	return out, nil
}

// PickIcon returns the smallest image at least want pixels wide, or the
// largest one if none is big enough.
func PickIcon(entries []IconEntry, want int) (IconEntry, bool) {
	best := -1
	for i, e := range entries {
		if e.Size >= want && (best < 0 || e.Size < entries[best].Size) {
			best = i
		}
	}
	if best < 0 {
		for i, e := range entries {
			if best < 0 || e.Size > entries[best].Size {
				best = i
			}
		}
	}
	if best < 0 {
		return IconEntry{}, false
	}
	return entries[best], true
}
