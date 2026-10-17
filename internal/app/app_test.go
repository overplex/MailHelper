//go:build windows

package app

import (
	"math"
	"testing"
)

func TestGuessServer(t *testing.T) {
	cases := map[string]string{
		"user@yandex.ru":      "imap.yandex.ru",
		"User@Gmail.com":      "imap.gmail.com",
		"someone@example.org": "imap.example.org",
		"no-at-sign":          "",
		"user@localhost":      "",
		"user@":               "",
	}
	for email, want := range cases {
		if got := guessServer(email); got != want {
			t.Errorf("guessServer(%q) = %q, want %q", email, got, want)
		}
	}
}

func TestRecolorInvertsLightnessOnly(t *testing.T) {
	const paper, ink = 0.2, 0.9
	// BGRA: white, black, and a saturated blue.
	pix := []byte{
		255, 255, 255, 255,
		0, 0, 0, 255,
		0xD7, 0x78, 0x00, 255,
	}
	recolor(pix, paper, ink)

	if _, _, l := toHSL(pix[2], pix[1], pix[0]); math.Abs(l-paper) > 0.01 {
		t.Errorf("white became lightness %.3f, want %.3f", l, paper)
	}
	if _, _, l := toHSL(pix[6], pix[5], pix[4]); math.Abs(l-ink) > 0.01 {
		t.Errorf("black became lightness %.3f, want %.3f", l, ink)
	}
	h0, _, _ := toHSL(0x00, 0x78, 0xD7)
	if h, _, _ := toHSL(pix[10], pix[9], pix[8]); math.Abs(h-h0) > 0.01 {
		t.Errorf("blue changed hue from %.3f to %.3f", h0, h)
	}
}

func TestHSLRoundTrip(t *testing.T) {
	for _, c := range [][3]byte{{0, 0, 0}, {255, 255, 255}, {0x2F, 0x6F, 0xEB}, {0xD9, 0x2D, 0x20}, {12, 200, 90}} {
		h, s, l := toHSL(c[0], c[1], c[2])
		r, g, b := fromHSL(h, s, l)
		if r != c[0] || g != c[1] || b != c[2] {
			t.Errorf("%v -> (%.3f %.3f %.3f) -> %v", c, h, s, l, [3]byte{r, g, b})
		}
	}
}
