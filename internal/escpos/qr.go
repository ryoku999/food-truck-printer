package escpos

import qrcode "github.com/skip2/go-qrcode"

func receiptQR(url string, paperWidth int) []byte {
	if url == "" || len(url) > 512 {
		return nil
	}
	qr, err := qrcode.New(url, qrcode.Medium)
	if err != nil {
		return nil
	}
	bitmap := qr.Bitmap()
	size := 384
	if paperWidth == 58 {
		size = 320
	}
	// Center the whole-dot modules, including their quiet zone, on the white raster.
	scale := size / len(bitmap)
	symbolSize := len(bitmap) * scale
	offset := (size - symbolSize) / 2
	return packRaster(size, size, func(x, y int) bool {
		x -= offset
		y -= offset
		if x < 0 || y < 0 || x >= symbolSize || y >= symbolSize {
			return false
		}
		return bitmap[y/scale][x/scale]
	})
}
