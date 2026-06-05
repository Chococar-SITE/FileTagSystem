package thumbnail

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func makePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 100, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestGenerateScalesDown(t *testing.T) {
	src := makePNG(t, 800, 400)
	out, err := Generate(src, 200)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 200 || cfg.Height != 100 {
		t.Fatalf("thumbnail size = %dx%d, want 200x100", cfg.Width, cfg.Height)
	}
}

func TestGenerateNeverUpscales(t *testing.T) {
	src := makePNG(t, 50, 30)
	out, err := Generate(src, 256)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := png.DecodeConfig(bytes.NewReader(out))
	if cfg.Width != 50 || cfg.Height != 30 {
		t.Fatalf("size = %dx%d, want unchanged 50x30", cfg.Width, cfg.Height)
	}
}

func TestGenerateRejectsNonImage(t *testing.T) {
	if _, err := Generate([]byte("not an image"), 100); err != ErrUnsupported {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}
