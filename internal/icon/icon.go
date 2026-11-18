// Package icon builds GF Sync's icons from the guild emblem
// (assets/brand/guild-emblem-source.png).
//
// The full emblem (wooden ring, shield, face) is too detailed for 16–32 px,
// so small icons and the tray icon use a simplified version: the dark green
// shield is redrawn in code and only the yellow fanged face is taken from
// the emblem. Status badges (paused, error, pairing) sit in the lower right.
//
// Everything is computed from the embedded PNG at run or build time, so the
// output is reproducible and no generated image is committed.
package icon

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"sync"

	"gfsync/assets/brand"
)

var (
	ShieldFill = color.NRGBA{0x25, 0x3a, 0x3a, 0xff} // emblem's banner green
	ShieldRim  = color.NRGBA{0x6f, 0xa8, 0x9c, 0xff} // lighter rim, visible on dark taskbars
	Outline    = color.NRGBA{0x10, 0x1c, 0x1c, 0xff}
	Yellow     = color.NRGBA{0xf1, 0xd3, 0x59, 0xff} // emblem's face yellow

	Gray  = color.NRGBA{0x9a, 0x9f, 0xa4, 0xff}
	Red   = color.NRGBA{0xe0, 0x45, 0x38, 0xff}
	Amber = color.NRGBA{0xf0, 0xa0, 0x18, 0xff}
	None  = color.NRGBA{}
)

// TraySizes are the sizes put in every tray ICO; Windows picks the one that
// matches the display scaling (16 px at 100 %, 20 at 125 %, 24 at 150 %, …).
var TraySizes = []int{16, 20, 24, 32, 40, 48}

// Normal, Paused, Error and Pairing are multi-size tray ICO files.
var Normal, Paused, Error, Pairing []byte

func init() {
	Normal = TrayICO(None)
	Paused = TrayICO(Gray)
	Error = TrayICO(Red)
	Pairing = TrayICO(Amber)
}

// TrayICO renders the simplified emblem at every TraySizes size.
func TrayICO(badge color.NRGBA) []byte {
	imgs := make([]*image.NRGBA, len(TraySizes))
	for i, s := range TraySizes {
		imgs[i] = Simple(s, badge)
	}
	return EncodeICO(imgs)
}

// ---- face mask from the emblem ---------------------------------------------

type faceMask struct {
	w, h   int
	on     []bool  // yellow pixels inside the face box
	x0, y0 float64 // face bounding box in source pixels
	x1, y1 float64
}

var (
	maskOnce sync.Once
	mask     *faceMask
)

// Source decodes the embedded emblem.
func Source() image.Image {
	img, err := png.Decode(bytes.NewReader(brand.EmblemPNG))
	if err != nil {
		panic("icon: bad embedded emblem: " + err.Error())
	}
	return img
}

func isYellow(c color.NRGBA) bool {
	return c.A >= 128 && c.R > 170 && c.G > 130 && c.B < 120
}

func face() *faceMask {
	maskOnce.Do(func() {
		img := Source()
		b := img.Bounds()
		m := &faceMask{w: b.Dx(), h: b.Dy(), on: make([]bool, b.Dx()*b.Dy())}
		// The face sits on the shield; ignore highlights on the wooden ring.
		fx0, fy0 := int(0.28*float64(m.w)), int(0.28*float64(m.h))
		fx1, fy1 := int(0.72*float64(m.w)), int(0.76*float64(m.h))
		m.x0, m.y0, m.x1, m.y1 = math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
		for y := fy0; y < fy1; y++ {
			for x := fx0; x < fx1; x++ {
				c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
				if !isYellow(c) {
					continue
				}
				m.on[y*m.w+x] = true
				m.x0, m.y0 = math.Min(m.x0, float64(x)), math.Min(m.y0, float64(y))
				m.x1, m.y1 = math.Max(m.x1, float64(x+1)), math.Max(m.y1, float64(y+1))
			}
		}
		if m.x0 > m.x1 {
			panic("icon: no face found in emblem")
		}
		mask = m
	})
	return mask
}

func (m *faceMask) at(x, y float64) bool {
	ix, iy := int(x), int(y)
	if x < 0 || y < 0 || ix >= m.w || iy >= m.h {
		return false
	}
	return m.on[iy*m.w+ix]
}

// ---- simplified emblem -------------------------------------------------------

// Shield geometry in unit canvas coordinates.
const (
	shX0, shX1 = 0.08, 0.92
	shY0, shY1 = 0.03, 0.97
	shStraight = 0.50 // fraction of the height with straight sides
	faceWidth  = 0.74 // face width as a fraction of the shield width
	faceCY     = 0.43 // face centre (canvas y)
	badgeR     = 0.21
	badgeC     = 0.78
)

// shieldDist returns how far (in canvas units, approximately) p is inside the
// shield; negative means outside.
func shieldDist(u, v float64) float64 {
	w := (shX1 - shX0) / 2
	cx := (shX0 + shX1) / 2
	h := shY1 - shY0
	t := (v - shY0) / h
	if t < 0 || t > 1 {
		return -1
	}
	hw := w
	if t > shStraight {
		k := (t - shStraight) / (1 - shStraight)
		hw = w * (1 - k*k)
	}
	side := hw - math.Abs(u-cx)
	top := v - shY0
	bottom := (1 - t) * h * 0.8
	return math.Min(side, math.Min(top, bottom))
}

// Simple renders the simplified emblem (shield + face) at size×size, with an
// optional status badge (badge.A == 0 means none).
func Simple(size int, badge color.NRGBA) *image.NRGBA {
	m := face()
	s := float64(size)
	px := 1 / s
	rim := math.Max(px, 0.07)
	// Face transform: canvas -> source pixels.
	fw := faceWidth * (shX1 - shX0)
	scale := (m.x1 - m.x0) / fw
	fh := (m.y1 - m.y0) / scale
	fx0 := (shX0+shX1)/2 - fw/2
	fy0 := faceCY - fh/2
	// Small icons: thicken the face a little so fangs stay visible.
	grow := 0.0
	if size <= 24 {
		grow = 0.35 * px
	}

	const ss = 8 // supersampling per axis
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					u := (float64(x) + (float64(sx)+0.5)/ss) / s
					v := (float64(y) + (float64(sy)+0.5)/ss) / s
					c, ok := sample(m, u, v, rim, px, grow, scale, fx0, fy0, badge)
					if !ok {
						continue
					}
					af := float64(c.A) / 255
					r += float64(c.R) * af
					g += float64(c.G) * af
					b += float64(c.B) * af
					a += af
				}
			}
			if a == 0 {
				continue
			}
			img.SetNRGBA(x, y, color.NRGBA{
				uint8(r/a + 0.5), uint8(g/a + 0.5), uint8(b/a + 0.5),
				uint8(255*a/(ss*ss) + 0.5),
			})
		}
	}
	return img
}

func sample(m *faceMask, u, v, rim, px, grow, scale, fx0, fy0 float64, badge color.NRGBA) (color.NRGBA, bool) {
	if badge.A != 0 {
		d := math.Hypot(u-badgeC, v-badgeC)
		switch {
		case d <= badgeR-math.Max(px, 0.05):
			return badge, true
		case d <= badgeR:
			return Outline, true
		}
	}
	d := shieldDist(u, v)
	switch {
	case d < 0:
		return color.NRGBA{}, false
	case d < math.Max(px*0.7, 0.025):
		return Outline, true
	case d < rim:
		return ShieldRim, true
	}
	sxp := m.x0 + (u-fx0)*scale
	syp := m.y0 + (v-fy0)*scale
	if m.at(sxp, syp) {
		return Yellow, true
	}
	if grow > 0 {
		g := grow * scale
		for _, o := range [][2]float64{{g, 0}, {-g, 0}, {0, g}, {0, -g}} {
			if m.at(sxp+o[0], syp+o[1]) {
				return Yellow, true
			}
		}
	}
	return ShieldFill, true
}

// ---- ICO encoding ------------------------------------------------------------

// EncodeICO writes a multi-image 32-bit BMP ICO (sizes up to 255 px; use
// winres for 256 px PNG entries).
func EncodeICO(imgs []*image.NRGBA) []byte {
	var b bytes.Buffer
	w := func(v any) { binary.Write(&b, binary.LittleEndian, v) }
	w(uint16(0))
	w(uint16(1))
	w(uint16(len(imgs)))
	offset := 6 + 16*len(imgs)
	var bodies [][]byte
	for _, img := range imgs {
		body := bmp(img)
		size := img.Bounds().Dx()
		b.WriteByte(uint8(size))
		b.WriteByte(uint8(size))
		b.WriteByte(0)
		b.WriteByte(0)
		w(uint16(1))
		w(uint16(32))
		w(uint32(len(body)))
		w(uint32(offset))
		offset += len(body)
		bodies = append(bodies, body)
	}
	for _, body := range bodies {
		b.Write(body)
	}
	return b.Bytes()
}

func bmp(img *image.NRGBA) []byte {
	size := img.Bounds().Dx()
	maskRow := ((size + 31) / 32) * 4
	var b bytes.Buffer
	w := func(v any) { binary.Write(&b, binary.LittleEndian, v) }
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
			p := img.NRGBAAt(x, y)
			b.Write([]byte{p.B, p.G, p.R, p.A})
		}
	}
	b.Write(make([]byte, maskRow*size))
	return b.Bytes()
}
