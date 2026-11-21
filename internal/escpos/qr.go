package escpos

import qrcode "github.com/skip2/go-qrcode"

func receiptQR(url string) []byte {
	if url == "" || len(url) > 512 {
		return nil
	}
	qr, err := qrcode.New(url, qrcode.Medium)
	if err != nil {
		return nil
	}
	bitmap := qr.Bitmap()
	// Scale by whole modules and retain the quiet zone for reliable scanning.
	scale := min(5, 224/len(bitmap))
	size := len(bitmap) * scale
	return packRaster(size, size, func(x, y int) bool {
		return bitmap[y/scale][x/scale]
	})
}
