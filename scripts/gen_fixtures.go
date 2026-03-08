//go:build ignore

// gen_fixtures.go generates placeholder PNG check images for testing.
// Run with: go run scripts/gen_fixtures.go
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

func main() {
	os.MkdirAll("scripts/fixtures", 0755)
	writeCheckPNG("scripts/fixtures/check-front.png", "SYNTHETIC CHECK - FRONT")
	writeCheckPNG("scripts/fixtures/check-back.png", "SYNTHETIC CHECK - BACK")
}

func writeCheckPNG(path, _ string) {
	img := image.NewRGBA(image.Rect(0, 0, 400, 200))
	white := color.RGBA{255, 255, 255, 255}
	gray := color.RGBA{200, 200, 200, 255}
	for y := 0; y < 200; y++ {
		for x := 0; x < 400; x++ {
			if x == 0 || x == 399 || y == 0 || y == 199 {
				img.Set(x, y, gray)
			} else {
				img.Set(x, y, white)
			}
		}
	}
	f, _ := os.Create(path)
	defer f.Close()
	png.Encode(f, img)
}
