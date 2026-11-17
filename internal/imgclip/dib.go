// Package imgclip turns Windows clipboard image data into Go images and back,
// and decides which clipboard contents count as a screenshot.
// Nothing here calls Windows, so it is tested on any OS.
package imgclip

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math/bits"
)

const (
	biRGB            = 0
	biBitfields      = 3
	biJPEG           = 4
	biPNG            = 5
	biAlphaBitfields = 6
)

// MaxPixels guards against absurd headers (about 16000 x 8000).
const MaxPixels = 128_000_000

var le = binary.LittleEndian

// DecodeDIB reads a packed DIB (CF_DIB or CF_DIBV5 clipboard data:
// BITMAPINFOHEADER or a later header, optional masks and palette, then pixels).
func DecodeDIB(b []byte) (*image.NRGBA, error) {
	if len(b) < 40 {
		return nil, errors.New("dib: too short")
	}
	hs := int(le.Uint32(b[0:]))
	if hs < 40 || hs > len(b) {
		return nil, fmt.Errorf("dib: unsupported header size %d", hs)
	}
	w := int(int32(le.Uint32(b[4:])))
	h := int(int32(le.Uint32(b[8:])))
	bpp := int(le.Uint16(b[14:]))
	comp := le.Uint32(b[16:])
	clrUsed := int(le.Uint32(b[32:]))

	topDown := h < 0
	if topDown {
		h = -h
	}
	if w <= 0 || h <= 0 || w*h > MaxPixels {
		return nil, fmt.Errorf("dib: bad size %dx%d", w, h)
	}

	// Embedded JPEG/PNG (rare on the clipboard but legal).
	if comp == biJPEG || comp == biPNG {
		img, _, err := image.Decode(bytes.NewReader(b[hs:]))
		if err != nil {
			return nil, err
		}
		return ToNRGBA(img), nil
	}
	if comp != biRGB && comp != biBitfields && comp != biAlphaBitfields {
		return nil, fmt.Errorf("dib: unsupported compression %d", comp)
	}

	off := hs
	var rm, gm, bm, am uint32
	useMasks := comp == biBitfields || comp == biAlphaBitfields
	if useMasks {
		if hs == 40 {
			n := 3
			if comp == biAlphaBitfields {
				n = 4
			}
			if len(b) < off+4*n {
				return nil, errors.New("dib: missing masks")
			}
			rm, gm, bm = le.Uint32(b[off:]), le.Uint32(b[off+4:]), le.Uint32(b[off+8:])
			if n == 4 {
				am = le.Uint32(b[off+12:])
			}
			off += 4 * n
		} else {
			if hs < 52 {
				return nil, errors.New("dib: header too short for masks")
			}
			rm, gm, bm = le.Uint32(b[40:]), le.Uint32(b[44:]), le.Uint32(b[48:])
			if hs >= 56 {
				am = le.Uint32(b[52:])
			}
		}
	}
	// With BI_RGB the alpha byte is undefined (often 0 or junk): opaque.

	var palette []color.NRGBA
	if bpp <= 8 {
		n := clrUsed
		if n <= 0 || n > 1<<bpp {
			n = 1 << bpp
		}
		if len(b) < off+4*n {
			return nil, errors.New("dib: missing palette")
		}
		palette = make([]color.NRGBA, n)
		for i := range palette {
			p := b[off+4*i:]
			palette[i] = color.NRGBA{p[2], p[1], p[0], 255}
		}
		off += 4 * n
	}

	switch bpp {
	case 1, 4, 8, 16, 24, 32:
	default:
		return nil, fmt.Errorf("dib: unsupported bit depth %d", bpp)
	}
	stride := ((w*bpp + 31) / 32) * 4
	need := stride * h
	if len(b)-off < need {
		// Some writers omit the 12 mask bytes after a 40-byte header.
		if useMasks && hs == 40 && len(b)-hs >= need {
			off = hs
		} else {
			return nil, fmt.Errorf("dib: pixel data short (%d < %d)", len(b)-off, need)
		}
	}
	px := b[off:]

	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	if bpp == 16 && !useMasks {
		rm, gm, bm = 0x7C00, 0x03E0, 0x001F // 5-5-5
	}
	if bpp == 32 && !useMasks {
		rm, gm, bm = 0x00FF0000, 0x0000FF00, 0x000000FF
	}
	rs, gs, bs, as := newField(rm), newField(gm), newField(bm), newField(am)

	anyAlpha := false
	for y := 0; y < h; y++ {
		sy := y
		if !topDown {
			sy = h - 1 - y
		}
		row := px[sy*stride : sy*stride+stride]
		dst := img.Pix[y*img.Stride : y*img.Stride+4*w]
		switch bpp {
		case 32:
			for x := 0; x < w; x++ {
				v := le.Uint32(row[4*x:])
				d := dst[4*x : 4*x+4 : 4*x+4]
				d[0], d[1], d[2] = rs.get(v), gs.get(v), bs.get(v)
				if am != 0 {
					a := as.get(v)
					d[3] = a
					if a != 0 {
						anyAlpha = true
					}
				} else {
					d[3] = 255
				}
			}
		case 24:
			for x := 0; x < w; x++ {
				s := row[3*x : 3*x+3 : 3*x+3]
				d := dst[4*x : 4*x+4 : 4*x+4]
				d[0], d[1], d[2], d[3] = s[2], s[1], s[0], 255
			}
		case 16:
			for x := 0; x < w; x++ {
				v := uint32(le.Uint16(row[2*x:]))
				d := dst[4*x : 4*x+4 : 4*x+4]
				d[0], d[1], d[2], d[3] = rs.get(v), gs.get(v), bs.get(v), 255
			}
		default: // 1, 4, 8 with palette
			per := 8 / bpp
			mask := byte(1<<bpp - 1)
			for x := 0; x < w; x++ {
				by := row[x/per]
				shift := uint(8 - bpp*(x%per+1))
				idx := int((by >> shift) & mask)
				c := color.NRGBA{0, 0, 0, 255}
				if idx < len(palette) {
					c = palette[idx]
				}
				d := dst[4*x : 4*x+4 : 4*x+4]
				d[0], d[1], d[2], d[3] = c.R, c.G, c.B, 255
			}
		}
	}
	// A real alpha mask but every pixel at 0 means "no alpha": make opaque.
	if bpp == 32 && am != 0 && !anyAlpha {
		for i := 3; i < len(img.Pix); i += 4 {
			img.Pix[i] = 255
		}
	}
	return img, nil
}

type field struct {
	mask  uint32
	shift int
	width int
}

func newField(m uint32) field {
	if m == 0 {
		return field{}
	}
	return field{mask: m, shift: bits.TrailingZeros32(m), width: bits.OnesCount32(m)}
}

func (f field) get(v uint32) uint8 {
	if f.mask == 0 {
		return 0
	}
	x := (v & f.mask) >> f.shift
	switch {
	case f.width == 8:
		return uint8(x)
	case f.width > 8:
		return uint8(x >> (f.width - 8))
	default:
		max := uint32(1)<<f.width - 1
		return uint8((x*255 + max/2) / max)
	}
}

// EncodeDIB24 makes a bottom-up 24-bit CF_DIB, the most widely understood
// clipboard bitmap. Transparent pixels are put on white.
func EncodeDIB24(img image.Image) []byte {
	n := ToNRGBA(img)
	w, h := n.Rect.Dx(), n.Rect.Dy()
	stride := ((w*24 + 31) / 32) * 4
	out := make([]byte, 40+stride*h)
	le.PutUint32(out[0:], 40)
	le.PutUint32(out[4:], uint32(int32(w)))
	le.PutUint32(out[8:], uint32(int32(h)))
	le.PutUint16(out[12:], 1)
	le.PutUint16(out[14:], 24)
	le.PutUint32(out[16:], biRGB)
	le.PutUint32(out[20:], uint32(stride*h))
	le.PutUint32(out[24:], 3780) // 96 dpi
	le.PutUint32(out[28:], 3780)
	px := out[40:]
	for y := 0; y < h; y++ {
		src := n.Pix[y*n.Stride : y*n.Stride+4*w]
		dst := px[(h-1-y)*stride:]
		for x := 0; x < w; x++ {
			r, g, b, a := src[4*x], src[4*x+1], src[4*x+2], src[4*x+3]
			if a != 255 {
				r, g, b = overWhite(r, a), overWhite(g, a), overWhite(b, a)
			}
			dst[3*x], dst[3*x+1], dst[3*x+2] = b, g, r
		}
	}
	return out
}

func overWhite(c, a uint8) uint8 {
	return uint8((uint32(c)*uint32(a) + 255*(255-uint32(a)) + 127) / 255)
}

// ToNRGBA returns img as a tightly packed *image.NRGBA starting at (0,0).
func ToNRGBA(img image.Image) *image.NRGBA {
	b := img.Bounds()
	if n, ok := img.(*image.NRGBA); ok && b.Min == (image.Point{}) && n.Stride == 4*b.Dx() {
		return n
	}
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	if r, ok := img.(*image.RGBA); ok {
		// Fast path: screenshots are opaque, where RGBA == NRGBA.
		opaque := true
		for y := 0; y < b.Dy() && opaque; y++ {
			row := r.Pix[(y+b.Min.Y-r.Rect.Min.Y)*r.Stride+(b.Min.X-r.Rect.Min.X)*4:]
			for x := 0; x < b.Dx(); x++ {
				if row[4*x+3] != 255 {
					opaque = false
					break
				}
			}
			copy(out.Pix[y*out.Stride:y*out.Stride+4*b.Dx()], row[:4*b.Dx()])
		}
		if opaque {
			return out
		}
	}
	draw.Draw(out, out.Rect, img, b.Min, draw.Src)
	return out
}

// DecodeImage decodes PNG, JPEG or a packed DIB.
func DecodeImage(data []byte) (image.Image, string, error) {
	if bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		img, err := png.Decode(bytes.NewReader(data))
		return img, "png", err
	}
	if bytes.HasPrefix(data, []byte{0xFF, 0xD8}) {
		img, err := jpeg.Decode(bytes.NewReader(data))
		return img, "jpeg", err
	}
	img, err := DecodeDIB(data)
	return img, "dib", err
}
