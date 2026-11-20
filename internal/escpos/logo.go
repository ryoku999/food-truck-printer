package escpos

import (
	"bytes"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// logoRaster fits the logo to the receipt, composites transparency over white,
// and packs the black pixels high bit first for ESC/POS GS v 0.
func logoRaster(data []byte, paperWidth int) []byte {
	if len(data) == 0 {
		return nil
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 {
		return nil
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	maxWidth, maxHeight := 288, 144
	if paperWidth == 58 {
		maxWidth = 224
	}
	if width > maxWidth {
		width, height = maxWidth, max(1, height*maxWidth/width)
	}
	if height > maxHeight {
		width, height = max(1, width*maxHeight/height), maxHeight
	}
	resized := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(resized, resized.Bounds(), image.White, image.Point{}, draw.Src)
	draw.ApproxBiLinear.Scale(resized, resized.Bounds(), src, bounds, draw.Over, nil)

	return packRaster(width, height, func(x, y int) bool {
		return color.GrayModel.Convert(resized.RGBAAt(x, y)).(color.Gray).Y < 160
	})
}

func packRaster(width, height int, black func(x, y int) bool) []byte {
	stride := (width + 7) / 8
	raster := make([]byte, 8+stride*height)
	copy(raster, []byte{0x1d, 'v', '0', 0, byte(stride), byte(stride >> 8), byte(height), byte(height >> 8)})
	var hasInk bool
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if black(x, y) {
				raster[8+y*stride+x/8] |= 0x80 >> uint(x%8)
				hasInk = true
			}
		}
	}
	if !hasInk {
		return nil
	}
	return raster
}
