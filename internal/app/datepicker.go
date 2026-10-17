//go:build windows

package app

import (
	"syscall"

	"mailhelper/internal/winapi"
)

// The date and time picker has no dark visual style and ignores
// WM_CTLCOLOR*, so in the dark theme it is subclassed: it paints itself into
// an off-screen canvas as usual, and the canvas is recoloured before it
// reaches the screen. Recolouring inverts lightness while keeping hue and
// saturation, which turns the white field dark and the black text light, and
// keeps the selected part of the date recognisably highlighted.

// pickerProcs maps each subclassed picker to its original window procedure.
var pickerProcs = map[uintptr]uintptr{}

var pickerCallback = syscall.NewCallback(pickerProc)

func subclassPicker(hwnd uintptr) {
	pickerProcs[hwnd] = winapi.SubclassWindow(hwnd, pickerCallback)
}

func pickerProc(hwnd, message, wParam, lParam uintptr) uintptr {
	original := pickerProcs[hwnd]
	a := instance
	if a == nil || a.theme != themeDark {
		return winapi.CallWindowProc(original, hwnd, message, wParam, lParam)
	}

	switch message {
	case winapi.WM_ERASEBKGND:
		return 1 // the canvas covers every pixel

	case winapi.WM_PAINT:
		var ps winapi.PaintStruct
		hdc := winapi.BeginPaint(hwnd, &ps)
		r := winapi.GetClientRect(hwnd)
		if c := winapi.NewCanvas(hdc, r.Width(), r.Height()); c != nil {
			// Common controls paint into the DC passed in wParam when it is
			// not zero, instead of calling BeginPaint themselves.
			winapi.CallWindowProc(original, hwnd, winapi.WM_PAINT, c.DC, 0)
			recolor(c.Pixels, lightness(a.pal.field), lightness(a.pal.text))
			c.CopyTo(hdc)
			c.Release()
		}
		winapi.EndPaint(hwnd, &ps)
		return 0
	}

	return winapi.CallWindowProc(original, hwnd, message, wParam, lParam)
}

// lightness is the HSL lightness of a COLORREF, between 0 and 1.
func lightness(c uintptr) float64 {
	r, g, b := float64(c&0xFF)/255, float64(c>>8&0xFF)/255, float64(c>>16&0xFF)/255
	return (max(r, g, b) + min(r, g, b)) / 2
}

// recolor maps the lightness of every BGRA pixel so that white turns into
// the lightness of paper and black into that of ink, with hue and
// saturation unchanged.
func recolor(pix []byte, paper, ink float64) {
	for i := 0; i+3 < len(pix); i += 4 {
		h, s, l := toHSL(pix[i+2], pix[i+1], pix[i])
		r, g, b := fromHSL(h, s, ink+(paper-ink)*l)
		pix[i], pix[i+1], pix[i+2] = b, g, r
	}
}

func toHSL(r8, g8, b8 byte) (h, s, l float64) {
	r, g, b := float64(r8)/255, float64(g8)/255, float64(b8)/255
	hi, lo := max(r, g, b), min(r, g, b)
	l = (hi + lo) / 2
	if hi == lo {
		return 0, 0, l
	}
	d := hi - lo
	if l > 0.5 {
		s = d / (2 - hi - lo)
	} else {
		s = d / (hi + lo)
	}
	switch hi {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h / 6, s, l
}

func fromHSL(h, s, l float64) (r, g, b byte) {
	if s == 0 {
		v := byte(l*255 + 0.5)
		return v, v, v
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	channel := func(t float64) byte {
		switch {
		case t < 0:
			t++
		case t > 1:
			t--
		}
		var v float64
		switch {
		case t < 1.0/6:
			v = p + (q-p)*6*t
		case t < 0.5:
			v = q
		case t < 2.0/3:
			v = p + (q-p)*(2.0/3-t)*6
		default:
			v = p
		}
		return byte(v*255 + 0.5)
	}
	return channel(h + 1.0/3), channel(h), channel(h - 1.0/3)
}
