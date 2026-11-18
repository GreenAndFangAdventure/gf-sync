package main

import (
	"image"
	"image/color"
	stddraw "image/draw"
	"image/png"
	"os"

	"github.com/tc-hib/winres"
	"golang.org/x/image/draw"

	"gfsync/internal/icon"
)

// App icon sizes. 16–32 px use the simplified emblem (shield + face), which
// stays readable in Explorer lists and the taskbar; 48 px and up use the full
// emblem with its wooden ring.
var (
	simpleSizes = []int{16, 24, 32}
	fullSizes   = []int{48, 64, 256}
)

func appImages() []image.Image {
	var imgs []image.Image
	for _, s := range simpleSizes {
		imgs = append(imgs, icon.Simple(s, icon.None))
	}
	for _, s := range fullSizes {
		imgs = append(imgs, emblem(s))
	}
	return imgs
}

func makeIcon() (*winres.Icon, error) {
	return winres.NewIconFromImages(appImages())
}

// emblem scales the full emblem (cropped to its visible pixels, centred on a
// square transparent canvas) to size×size.
func emblem(size int) *image.NRGBA {
	src := icon.Source()
	crop := opaqueBounds(src)
	side := crop.Dx()
	if crop.Dy() > side {
		side = crop.Dy()
	}
	sq := image.NewNRGBA(image.Rect(0, 0, side, side))
	off := image.Pt((side-crop.Dx())/2, (side-crop.Dy())/2)
	stddraw.Draw(sq, crop.Sub(crop.Min).Add(off), src, crop.Min, stddraw.Src)
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(dst, dst.Bounds(), sq, sq.Bounds(), draw.Src, nil)
	return dst
}

func opaqueBounds(img image.Image) image.Rectangle {
	b := img.Bounds()
	r := image.Rectangle{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0x0800 {
				r = r.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	if r.Empty() {
		return b
	}
	return r
}

// writePreview draws every icon at actual size and enlarged (nearest
// neighbour, so pixels stay visible) on a light and a dark strip, like a
// light and a dark Windows taskbar.
func writePreview(path string) error {
	type row struct{ imgs []*image.NRGBA }
	var rows []row
	var app []*image.NRGBA
	for _, im := range appImages() {
		app = append(app, im.(*image.NRGBA))
	}
	rows = append(rows, row{app})
	for _, badge := range []color.NRGBA{icon.None, icon.Gray, icon.Red, icon.Amber} {
		var r row
		for _, s := range icon.TraySizes {
			r.imgs = append(r.imgs, icon.Simple(s, badge))
		}
		rows = append(rows, r)
	}
	const zoom, pad = 4, 12
	width, height := pad, pad
	for _, r := range rows {
		w, h := pad, 0
		for _, im := range r.imgs {
			s := im.Bounds().Dx()
			disp := s * zoom
			if s >= 64 {
				disp = s // big ones at 1:1
			}
			w += s + pad + disp + pad
			if disp > h {
				h = disp
			}
		}
		if w > width {
			width = w
		}
		height += h + pad
	}
	out := image.NewNRGBA(image.Rect(0, 0, width, height*2))
	bgs := []color.NRGBA{{0xf3, 0xf3, 0xf3, 0xff}, {0x20, 0x20, 0x20, 0xff}}
	for strip, bg := range bgs {
		y0 := strip * height
		stddraw.Draw(out, image.Rect(0, y0, width, y0+height), image.NewUniform(bg), image.Point{}, stddraw.Src)
		y := y0 + pad
		for _, r := range rows {
			x, h := pad, 0
			for _, im := range r.imgs {
				s := im.Bounds().Dx()
				stddraw.Draw(out, image.Rect(x, y, x+s, y+s), im, image.Point{}, stddraw.Over)
				x += s + pad
				disp := s * zoom
				if s >= 64 {
					disp = s
				}
				big := image.NewNRGBA(image.Rect(0, 0, disp, disp))
				draw.NearestNeighbor.Scale(big, big.Bounds(), im, im.Bounds(), draw.Src, nil)
				stddraw.Draw(out, image.Rect(x, y, x+disp, y+disp), big, image.Point{}, stddraw.Over)
				x += disp + pad
				if disp > h {
					h = disp
				}
			}
			y += h + pad
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, out); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
