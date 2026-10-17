//go:build windows

// Package appicon turns the application's icon.png into the window icons
// Windows asks for. The artwork is rendered at the exact pixel size the
// shell wants at the current DPI rather than letting Windows stretch one
// bitmap, so the caption and taskbar icons stay crisp on every monitor.
package appicon

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"

	"mailhelper/internal/winapi"
)

// source holds the decoded artwork; Load must run before any other call.
var source *image.RGBA

// Load decodes the embedded artwork. It is cheap to call more than once.
func Load(data []byte) error {
	if source != nil {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode icon: %w", err)
	}
	b := img.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
	source = rgba
	return nil
}

// Image returns the artwork resampled to a square of the given size.
func Image(size int32) (*image.RGBA, error) {
	if source == nil {
		return nil, fmt.Errorf("icon artwork not loaded")
	}
	if size <= 0 {
		return nil, fmt.Errorf("bad icon size %d", size)
	}
	return resample(source, int(size)), nil
}

// New creates an HICON of the given size. The caller owns the handle.
func New(size int32) (uintptr, error) {
	img, err := Image(size)
	if err != nil {
		return 0, err
	}

	// image.RGBA is RGBA; a DIB section is BGRA.
	pix := make([]byte, len(img.Pix))
	for i := 0; i+3 < len(pix); i += 4 {
		pix[i+0] = img.Pix[i+2]
		pix[i+1] = img.Pix[i+1]
		pix[i+2] = img.Pix[i+0]
		pix[i+3] = img.Pix[i+3]
	}

	h := winapi.NewIconFromBGRA(pix, size)
	if h == 0 {
		return 0, fmt.Errorf("CreateIconIndirect failed")
	}
	return h, nil
}

// EncodePNG writes the artwork at the given size as PNG bytes, which is what
// tools/genicon feeds to go-winres for the executable's own icon resource.
func EncodePNG(size int32) ([]byte, error) {
	img, err := Image(size)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// resample scales src into a size x size image by averaging every source
// pixel that falls inside a destination pixel's box. Averaging in
// premultiplied space keeps transparent regions from bleeding colour into
// the edges, which a plain average would do.
func resample(src *image.RGBA, size int) *image.RGBA {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewRGBA(image.Rect(0, 0, size, size))

	for y := range size {
		y0 := y * sh / size
		y1 := (y + 1) * sh / size
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := range size {
			x0 := x * sw / size
			x1 := (x + 1) * sw / size
			if x1 <= x0 {
				x1 = x0 + 1
			}

			var sumR, sumG, sumB, sumA, n int
			for sy := y0; sy < y1 && sy < sh; sy++ {
				row := sy * src.Stride
				for sx := x0; sx < x1 && sx < sw; sx++ {
					i := row + sx*4
					a := int(src.Pix[i+3])
					sumR += int(src.Pix[i+0]) * a
					sumG += int(src.Pix[i+1]) * a
					sumB += int(src.Pix[i+2]) * a
					sumA += a
					n++
				}
			}
			if n == 0 || sumA == 0 {
				continue // nothing to average, leave the pixel transparent
			}

			o := y*dst.Stride + x*4
			dst.Pix[o+0] = byte(sumR / sumA)
			dst.Pix[o+1] = byte(sumG / sumA)
			dst.Pix[o+2] = byte(sumB / sumA)
			dst.Pix[o+3] = byte(sumA / n)
		}
	}
	return dst
}
