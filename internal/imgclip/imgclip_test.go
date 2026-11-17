package imgclip

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// makeDIB builds a DIB by hand the way Windows apps do.
func makeDIB(t *testing.T, w, h, bpp int, topDown bool, comp uint32, hs int, masks []uint32, palette []color.RGBA, pix func(x, y int) []byte) []byte {
	t.Helper()
	stride := ((w*bpp + 31) / 32) * 4
	var b bytes.Buffer
	hdr := make([]byte, hs)
	binary.LittleEndian.PutUint32(hdr[0:], uint32(hs))
	binary.LittleEndian.PutUint32(hdr[4:], uint32(int32(w)))
	hh := int32(h)
	if topDown {
		hh = -hh
	}
	binary.LittleEndian.PutUint32(hdr[8:], uint32(hh))
	binary.LittleEndian.PutUint16(hdr[12:], 1)
	binary.LittleEndian.PutUint16(hdr[14:], uint16(bpp))
	binary.LittleEndian.PutUint32(hdr[16:], comp)
	binary.LittleEndian.PutUint32(hdr[32:], uint32(len(palette)))
	if hs >= 56 {
		for i, m := range masks {
			binary.LittleEndian.PutUint32(hdr[40+4*i:], m)
		}
	}
	b.Write(hdr)
	if hs == 40 {
		for _, m := range masks {
			binary.Write(&b, binary.LittleEndian, m)
		}
	}
	for _, c := range palette {
		b.Write([]byte{c.B, c.G, c.R, 0})
	}
	rows := make([][]byte, h)
	for y := 0; y < h; y++ {
		row := make([]byte, stride)
		if bpp >= 8 {
			bpb := bpp / 8
			for x := 0; x < w; x++ {
				copy(row[x*bpb:], pix(x, y))
			}
		} else {
			per := 8 / bpp
			for x := 0; x < w; x++ {
				v := pix(x, y)[0]
				row[x/per] |= v << uint(8-bpp*(x%per+1))
			}
		}
		rows[y] = row
	}
	for i := 0; i < h; i++ {
		y := i
		if !topDown {
			y = h - 1 - i
		}
		b.Write(rows[y])
	}
	return b.Bytes()
}

func want(x, y int) color.NRGBA {
	return color.NRGBA{uint8(10 + x*40), uint8(20 + y*50), uint8(200 - x*10 - y), 255}
}

func check(t *testing.T, img *image.NRGBA, w, h int, f func(x, y int) color.NRGBA) {
	t.Helper()
	if img.Rect.Dx() != w || img.Rect.Dy() != h {
		t.Fatalf("size %v", img.Rect)
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if got := img.NRGBAAt(x, y); got != f(x, y) {
				t.Fatalf("(%d,%d) = %v, want %v", x, y, got, f(x, y))
			}
		}
	}
}

func TestDIB24BottomUp(t *testing.T) {
	// width 3 makes rows need padding (9 -> 12 bytes).
	d := makeDIB(t, 3, 2, 24, false, biRGB, 40, nil, nil, func(x, y int) []byte {
		c := want(x, y)
		return []byte{c.B, c.G, c.R}
	})
	img, err := DecodeDIB(d)
	if err != nil {
		t.Fatal(err)
	}
	check(t, img, 3, 2, want)
}

func TestDIB32TopDownJunkAlphaIsOpaque(t *testing.T) {
	d := makeDIB(t, 4, 3, 32, true, biRGB, 40, nil, nil, func(x, y int) []byte {
		c := want(x, y)
		return []byte{c.B, c.G, c.R, uint8(x * 7)} // junk alpha is ignored for BI_RGB
	})
	img, err := DecodeDIB(d)
	if err != nil {
		t.Fatal(err)
	}
	check(t, img, 4, 3, want)
}

func TestDIB32BitfieldsAfterHeader(t *testing.T) {
	d := makeDIB(t, 2, 2, 32, false, biBitfields, 40, []uint32{0xFF0000, 0xFF00, 0xFF}, nil, func(x, y int) []byte {
		c := want(x, y)
		return []byte{c.B, c.G, c.R, 0}
	})
	img, err := DecodeDIB(d)
	if err != nil {
		t.Fatal(err)
	}
	check(t, img, 2, 2, want)
}

func TestDIBV5WithAlpha(t *testing.T) {
	d := makeDIB(t, 2, 2, 32, false, biBitfields, 124, []uint32{0xFF0000, 0xFF00, 0xFF, 0xFF000000}, nil, func(x, y int) []byte {
		c := want(x, y)
		return []byte{c.B, c.G, c.R, uint8(100 + x)}
	})
	img, err := DecodeDIB(d)
	if err != nil {
		t.Fatal(err)
	}
	check(t, img, 2, 2, func(x, y int) color.NRGBA { c := want(x, y); c.A = uint8(100 + x); return c })
}

func TestDIBV5AlphaAllZeroIsOpaque(t *testing.T) {
	d := makeDIB(t, 2, 2, 32, false, biBitfields, 124, []uint32{0xFF0000, 0xFF00, 0xFF, 0xFF000000}, nil, func(x, y int) []byte {
		c := want(x, y)
		return []byte{c.B, c.G, c.R, 0}
	})
	img, err := DecodeDIB(d)
	if err != nil {
		t.Fatal(err)
	}
	check(t, img, 2, 2, want)
}

func TestDIB16And8(t *testing.T) {
	d := makeDIB(t, 3, 2, 16, false, biBitfields, 40, []uint32{0xF800, 0x07E0, 0x001F}, nil, func(x, y int) []byte {
		v := uint16(31<<11 | uint16(x*20)<<5 | uint16(y*31))
		return []byte{byte(v), byte(v >> 8)}
	})
	img, err := DecodeDIB(d)
	if err != nil {
		t.Fatal(err)
	}
	if c := img.NRGBAAt(2, 1); c.R != 255 || c.B != 255 || c.G != uint8((40*255+31)/63) {
		t.Fatalf("565 = %v", c)
	}

	pal := []color.RGBA{{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}}
	d = makeDIB(t, 5, 2, 8, false, biRGB, 40, nil, pal, func(x, y int) []byte { return []byte{byte((x + y) % 3)} })
	img, err = DecodeDIB(d)
	if err != nil {
		t.Fatal(err)
	}
	if c := img.NRGBAAt(4, 1); c != (color.NRGBA{0, 0, 255, 255}) { // (4+1)%3 = 2 = blue
		t.Fatalf("8bpp = %v", c)
	}

	d = makeDIB(t, 9, 1, 1, true, biRGB, 40, nil, []color.RGBA{{0, 0, 0, 255}, {255, 255, 255, 255}}, func(x, y int) []byte { return []byte{byte(x % 2)} })
	img, err = DecodeDIB(d)
	if err != nil {
		t.Fatal(err)
	}
	if img.NRGBAAt(8, 0).R != 0 || img.NRGBAAt(7, 0).R != 255 {
		t.Fatal("1bpp wrong")
	}
}

func TestDIBRejectsGarbage(t *testing.T) {
	for _, b := range [][]byte{nil, make([]byte, 39), append([]byte{40, 0, 0, 0}, make([]byte, 60)...)} {
		if _, err := DecodeDIB(b); err == nil {
			t.Fatal("want error")
		}
	}
	// claims 100x100 but has no pixels
	d := makeDIB(t, 1, 1, 24, false, biRGB, 40, nil, nil, func(x, y int) []byte { return []byte{1, 2, 3} })
	binary.LittleEndian.PutUint32(d[4:], 100)
	binary.LittleEndian.PutUint32(d[8:], 100)
	if _, err := DecodeDIB(d); err == nil {
		t.Fatal("want short error")
	}
}

func TestEncodeDIBRoundTrip(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 5, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 5; x++ {
			src.SetNRGBA(x, y, want(x, y))
		}
	}
	back, err := DecodeDIB(EncodeDIB24(src))
	if err != nil {
		t.Fatal(err)
	}
	check(t, back, 5, 3, want)

	// transparent goes on white
	src.SetNRGBA(0, 0, color.NRGBA{0, 0, 0, 0})
	back, _ = DecodeDIB(EncodeDIB24(src))
	if c := back.NRGBAAt(0, 0); c != (color.NRGBA{255, 255, 255, 255}) {
		t.Fatalf("got %v", c)
	}
}

func TestDecodeImagePNG(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	src.Set(1, 1, color.RGBA{1, 2, 3, 255})
	var buf bytes.Buffer
	png.Encode(&buf, src)
	img, kind, err := DecodeImage(buf.Bytes())
	if err != nil || kind != "png" || img.Bounds().Dx() != 2 {
		t.Fatal(kind, err)
	}
}

func TestDecide(t *testing.T) {
	reg := func(id uint32, n string) Format { return Format{ID: id, Name: n} }
	cases := []struct {
		name     string
		f        []Format
		skipDocs bool
		noHist   bool
		src      Source
		skip     string
	}{
		{"text only", []Format{{ID: CF_UNICODETEXT}}, true, false, SourceNone, ""},
		{"snipping tool", []Format{reg(0xC1A0, "PNG"), {ID: CF_BITMAP}, {ID: CF_DIB}, {ID: CF_DIBV5}}, true, false, SourcePNG, ""},
		{"printscreen", []Format{{ID: CF_BITMAP}, {ID: CF_DIB}, {ID: CF_DIBV5}}, true, false, SourceDIB, ""},
		{"app put dibv5", []Format{{ID: CF_DIBV5}, {ID: CF_BITMAP}, {ID: CF_DIB}}, true, false, SourceDIBV5, ""},
		{"excel cells", []Format{reg(0xC009, "DataObject"), reg(0xC0AB, "Biff12"), {ID: CF_ENHMETAFILE}, {ID: CF_BITMAP}, {ID: CF_UNICODETEXT}, {ID: CF_DIB}}, true, false, SourceNone, "document"},
		{"excel cells, rule off", []Format{reg(0xC0AB, "Biff12"), {ID: CF_ENHMETAFILE}, {ID: CF_BITMAP}, {ID: CF_UNICODETEXT}, {ID: CF_DIB}}, false, false, SourceDIB, ""},
		{"browser image", []Format{reg(0xC0F0, "HTML Format"), {ID: CF_DIB}, reg(0xC1A0, "PNG")}, true, false, SourcePNG, ""},
		{"web text with picture", []Format{reg(0xC0F0, "HTML Format"), {ID: CF_UNICODETEXT}, {ID: CF_DIB}}, true, false, SourceNone, "document"},
		{"password manager", []Format{{ID: CF_DIB}, reg(0xC222, "ExcludeClipboardContentFromMonitorProcessing")}, false, false, SourceNone, "private"},
		{"no history flag", []Format{{ID: CF_DIB}}, true, true, SourceNone, "private"},
	}
	for _, c := range cases {
		d := Decide(c.f, c.skipDocs, c.noHist)
		if d.Source != c.src || d.Skip != c.skip {
			t.Errorf("%s: got %+v", c.name, d)
		}
	}
}

func TestPickIcon(t *testing.T) {
	es := []IconEntry{{Size: 16}, {Size: 32}, {Size: 24}, {Size: 256}}
	for want, got := range map[int]int{16: 16, 20: 24, 24: 24, 33: 256, 300: 256} {
		e, _ := PickIcon(es, want)
		if e.Size != got {
			t.Errorf("want %d: got %d, expected %d", want, e.Size, got)
		}
	}
}
