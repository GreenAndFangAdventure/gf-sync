// Package icon draws GF Sync's tray icons in code (no binary assets):
// a green disc, with a coloured badge for paused/error states.
package icon

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"math"
)

var (
	Green  = color.NRGBA{0x3f, 0xa8, 0x5a, 0xff}
	Gray   = color.NRGBA{0x8a, 0x8f, 0x94, 0xff}
	Red    = color.NRGBA{0xd9, 0x48, 0x3b, 0xff}
	Amber  = color.NRGBA{0xe8, 0xa3, 0x17, 0xff}
	tusk   = color.NRGBA{0xf4, 0xee, 0xdc, 0xff}
	shadow = color.NRGBA{0x1f, 0x5c, 0x30, 0xff}
)

// Normal, Paused, Error and Pairing are ready-made 32×32 ICO files.
var (
	Normal  = ICO(32, Green, color.NRGBA{})
	Paused  = ICO(32, Gray, color.NRGBA{})
	Error   = ICO(32, Green, Red)
	Pairing = ICO(32, Green, Amber)
)

// ICO renders a size×size 32-bit icon. badge.A == 0 means no badge.
func ICO(size int, body, badge color.NRGBA) []byte {
	return encode(size, pixels(size, body, badge))
}

// Image renders the same drawing as ICO, for embedding as an exe resource.
func Image(size int, body, badge color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for i, p := range pixels(size, body, badge) {
		img.SetNRGBA(i%size, i/size, p)
	}
	return img
}

func pixels(size int, body, badge color.NRGBA) []color.NRGBA {
	px := make([]color.NRGBA, size*size)
	s := float64(size)
	c := s / 2
	r := s/2 - 1
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			d := math.Hypot(fx-c, fy-c)
			var p color.NRGBA
			switch {
			case d <= r-2:
				p = body
			case d <= r:
				p = darker(body)
			case d <= r+1:
				p = darker(body)
				p.A = uint8(255 * (r + 1 - d))
			}
			// Two small tusks (the guild's "fang").
			for _, tx := range []float64{c - s*0.18, c + s*0.18} {
				if fy > c+s*0.02 && fy < c+s*0.30 && math.Abs(fx-tx) < s*0.07*(1-(fy-c)/(s*0.32)) {
					p = tusk
				}
			}
			if badge.A != 0 {
				bc := s * 0.78
				bd := math.Hypot(fx-bc, fy-s*0.22)
				if bd <= s*0.22 {
					p = badge
				}
			}
			px[y*size+x] = p
		}
	}
	return px
}

func darker(c color.NRGBA) color.NRGBA {
	if c == Green {
		return shadow
	}
	return color.NRGBA{c.R * 2 / 3, c.G * 2 / 3, c.B * 2 / 3, c.A}
}

func encode(size int, px []color.NRGBA) []byte {
	maskRow := ((size + 31) / 32) * 4
	imgBytes := 40 + size*size*4 + maskRow*size
	var b bytes.Buffer
	w := func(v any) { binary.Write(&b, binary.LittleEndian, v) }
	// ICONDIR
	w(uint16(0))
	w(uint16(1))
	w(uint16(1))
	// ICONDIRENTRY
	dim := uint8(size)
	if size >= 256 {
		dim = 0
	}
	b.WriteByte(dim)
	b.WriteByte(dim)
	b.WriteByte(0)
	b.WriteByte(0)
	w(uint16(1))
	w(uint16(32))
	w(uint32(imgBytes))
	w(uint32(22))
	// BITMAPINFOHEADER (height doubled: XOR + AND masks)
	w(uint32(40))
	w(int32(size))
	w(int32(size * 2))
	w(uint16(1))
	w(uint16(32))
	w(uint32(0))
	w(uint32(size*size*4 + maskRow*size))
	w(int32(0))
	w(int32(0))
	w(uint32(0))
	w(uint32(0))
	for y := size - 1; y >= 0; y-- {
		for x := 0; x < size; x++ {
			p := px[y*size+x]
			b.Write([]byte{p.B, p.G, p.R, p.A})
		}
	}
	b.Write(make([]byte, maskRow*size))
	return b.Bytes()
}
