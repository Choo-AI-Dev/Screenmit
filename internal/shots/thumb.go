package shots

import (
	"bytes"
	"image"
	"image/jpeg"

	"github.com/Choo-AI-Dev/screenmit/internal/imgclip"
)

// ThumbMax is the longest side of a thumbnail.
const ThumbMax = 520

// MakeThumb shrinks img to fit ThumbMax (area averaging) and encodes JPEG,
// putting transparent pixels on white.
func MakeThumb(img image.Image) ([]byte, error) {
	src := imgclip.ToNRGBA(img)
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	dw, dh := sw, sh
	if sw > ThumbMax || sh > ThumbMax {
		if sw >= sh {
			dw, dh = ThumbMax, max(1, sh*ThumbMax/sw)
		} else {
			dw, dh = max(1, sw*ThumbMax/sh), ThumbMax
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	// Accumulate each source pixel into its destination cell.
	type acc struct{ r, g, b, n uint32 }
	cells := make([]acc, dw*dh)
	for y := 0; y < sh; y++ {
		dy := y * dh / sh
		row := src.Pix[y*src.Stride:]
		base := dy * dw
		for x := 0; x < sw; x++ {
			p := row[4*x : 4*x+4 : 4*x+4]
			r, g, b, a := uint32(p[0]), uint32(p[1]), uint32(p[2]), uint32(p[3])
			if a != 255 {
				r = (r*a + 255*(255-a)) / 255
				g = (g*a + 255*(255-a)) / 255
				b = (b*a + 255*(255-a)) / 255
			}
			c := &cells[base+x*dw/sw]
			c.r += r
			c.g += g
			c.b += b
			c.n++
		}
	}
	for i, c := range cells {
		if c.n == 0 {
			c = acc{255, 255, 255, 1}
		}
		d := dst.Pix[4*i : 4*i+4 : 4*i+4]
		d[0], d[1], d[2], d[3] = uint8(c.r/c.n), uint8(c.g/c.n), uint8(c.b/c.n), 255
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 84}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
