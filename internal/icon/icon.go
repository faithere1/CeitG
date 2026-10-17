// Package icon rasterizes the CeitG app icon: a runner chased by a catcher with a stick.
package icon

import (
	"image"
	"image/color"
	"math"
)

func Draw(n int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	f := float64(n)
	disc := func(cx, cy, r float64, c color.RGBA) {
		for y := 0; y < n; y++ {
			for x := 0; x < n; x++ {
				// 1px soft edge
				d := r - math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
				if d <= -0.5 {
					continue
				}
				a := math.Min(1, d+0.5)
				o := img.RGBAAt(x, y)
				mix := func(s, d uint8) uint8 { return uint8(float64(s)*a + float64(d)*(1-a)) }
				img.SetRGBA(x, y, color.RGBA{mix(c.R, o.R), mix(c.G, o.G), mix(c.B, o.B), mix(255, o.A)})
			}
		}
	}
	disc(f/2, f/2, f*0.48, color.RGBA{0x12, 0x14, 0x1c, 0xff})
	for t := 0.0; t <= 1; t += 0.02 {
		disc(f*(0.42+0.2*t), f*(0.5-0.28*t), f*0.045, color.RGBA{0xc8, 0xa2, 0x6b, 0xff})
	}
	disc(f*0.35, f*0.62, f*0.2, color.RGBA{0xff, 0x5d, 0x73, 0xff})
	disc(f*0.7, f*0.42, f*0.17, color.RGBA{0x4c, 0xc9, 0xf0, 0xff})
	return img
}
