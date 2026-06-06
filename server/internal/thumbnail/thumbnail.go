// Package thumbnail generates small preview images (§9.8). It enforces a
// pixel-count cap BEFORE decoding to defend against decompression bombs (§9.5).
package thumbnail

import (
	"bytes"
	"errors"
	"image"
	"image/png"

	"golang.org/x/image/draw"

	// Register decoders for common formats.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

// MaxPixels is the largest image (width*height) we will decode.
const MaxPixels = 50_000_000

var (
	// ErrTooLarge means the image exceeds the pixel cap.
	ErrTooLarge = errors.New("thumbnail: image exceeds pixel limit")
	// ErrUnsupported means the bytes are not a decodable image.
	ErrUnsupported = errors.New("thumbnail: unsupported or invalid image")
)

// Generate decodes data, scales it to fit within maxDim×maxDim (preserving
// aspect ratio, never upscaling), and returns a PNG.
func Generate(data []byte, maxDim int) ([]byte, error) {
	if maxDim <= 0 {
		maxDim = 256
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, ErrUnsupported
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > MaxPixels {
		return nil, ErrTooLarge
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrUnsupported
	}

	w, h := scaledSize(src.Bounds().Dx(), src.Bounds().Dy(), maxDim)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// scaledSize fits (srcW, srcH) within maxDim without upscaling.
func scaledSize(srcW, srcH, maxDim int) (int, int) {
	if srcW <= maxDim && srcH <= maxDim {
		return srcW, srcH
	}
	if srcW >= srcH {
		return maxDim, max1(srcH * maxDim / srcW)
	}
	return max1(srcW * maxDim / srcH), maxDim
}

func max1(v int) int {
	if v < 1 {
		return 1
	}
	return v
}
