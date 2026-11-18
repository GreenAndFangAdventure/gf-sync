package icon

import (
	"encoding/binary"
	"image/color"
	"testing"
)

func TestTrayICO(t *testing.T) {
	for _, ico := range [][]byte{Normal, Paused, Error, Pairing} {
		if binary.LittleEndian.Uint16(ico[2:]) != 1 {
			t.Fatal("not an icon")
		}
		n := int(binary.LittleEndian.Uint16(ico[4:]))
		if n != len(TraySizes) {
			t.Fatalf("%d images, want %d", n, len(TraySizes))
		}
		end := 0
		for i := 0; i < n; i++ {
			e := ico[6+16*i:]
			if int(e[0]) != TraySizes[i] || int(e[1]) != TraySizes[i] {
				t.Fatalf("entry %d is %dx%d", i, e[0], e[1])
			}
			size := binary.LittleEndian.Uint32(e[8:])
			off := binary.LittleEndian.Uint32(e[12:])
			if int(off+size) > len(ico) {
				t.Fatal("entry past end of file")
			}
			if int(off+size) > end {
				end = int(off + size)
			}
		}
		if end != len(ico) {
			t.Fatalf("trailing bytes: %d != %d", end, len(ico))
		}
	}
	if string(Normal) == string(Error) || string(Paused) == string(Pairing) {
		t.Fatal("states must differ")
	}
}

func TestSimpleLooksLikeTheEmblem(t *testing.T) {
	for _, s := range []int{16, 32, 48} {
		img := Simple(s, None)
		if img.NRGBAAt(0, 0).A != 0 || img.NRGBAAt(s-1, s-1).A != 0 {
			t.Fatalf("%d: corners must be transparent", s)
		}
		if img.NRGBAAt(s/2, s/4).A != 255 {
			t.Fatalf("%d: shield must be opaque", s)
		}
		yellow, green := 0, 0
		for y := 0; y < s; y++ {
			for x := 0; x < s; x++ {
				c := img.NRGBAAt(x, y)
				if c.A == 255 && c.R > 180 && c.G > 150 && c.B < 140 {
					yellow++
				}
				if c.A == 255 && c.G < 110 && c.G > c.R && c.B > c.R {
					green++
				}
			}
		}
		if yellow < s*s/40 || green < s*s/10 {
			t.Fatalf("%d: yellow %d green %d", s, yellow, green)
		}
	}
}

func TestBadgeLowerRight(t *testing.T) {
	s := 32
	img := Simple(s, Red)
	at := func(u, v float64) color.NRGBA { return img.NRGBAAt(int(u*float64(s)), int(v*float64(s))) }
	if at(badgeC, badgeC) != Red {
		t.Fatalf("badge centre = %v", at(badgeC, badgeC))
	}
	if Simple(s, None).NRGBAAt(int(badgeC*float64(s)), int(badgeC*float64(s))) == Red {
		t.Fatal("no badge expected")
	}
}

func TestDeterministic(t *testing.T) {
	if string(TrayICO(Amber)) != string(Pairing) {
		t.Fatal("rendering must be deterministic")
	}
}
