//go:build windows

package app

import (
	"mailhelper/internal/winapi"
)

// theme is the colour scheme the window is drawn in.
type theme int

const (
	themeLight theme = iota
	themeDark
	themeCount
)

// themeCodes are the identifiers a theme is stored under in the settings.
var themeCodes = [themeCount]string{"light", "dark"}

func (t theme) code() string { return themeCodes[t] }

func themeFromCode(code string) (theme, bool) {
	for i, c := range themeCodes {
		if c == code {
			return theme(i), true
		}
	}
	return themeLight, false
}

// palette is every colour the application paints with itself. The common
// controls draw the rest, steered by the visual style set in applyTheme.
type palette struct {
	window     uintptr // window and panel background
	panelEdge  uintptr // panel outline
	field      uintptr // edit boxes and drop-down lists
	text       uintptr
	textDim    uintptr // disabled labels
	track      uintptr // empty part of a progress bar
	trackEdge  uintptr
	fill       uintptr // filled part of a progress bar
	onFill     uintptr // progress label over the fill
	danger     uintptr // background of the "Delete messages" panel
	dangerEdge uintptr
	dangerText uintptr // caption of that panel
	dangerFill uintptr // its progress bar
}

var palettes = [themeCount]palette{
	themeLight: {
		window:     winapi.RGB(0xF3, 0xF5, 0xF8),
		panelEdge:  winapi.RGB(0xD5, 0xDB, 0xE3),
		field:      winapi.RGB(0xFF, 0xFF, 0xFF),
		text:       winapi.RGB(0x1A, 0x1A, 0x1A),
		textDim:    winapi.RGB(0x87, 0x8F, 0x9A),
		track:      winapi.RGB(0xE1, 0xE6, 0xED),
		trackEdge:  winapi.RGB(0xC2, 0xCB, 0xD6),
		fill:       winapi.RGB(0x2F, 0x6F, 0xEB),
		onFill:     winapi.RGB(0xFF, 0xFF, 0xFF),
		danger:     winapi.RGB(0xFD, 0xEC, 0xEC),
		dangerEdge: winapi.RGB(0xF1, 0xB5, 0xB5),
		dangerText: winapi.RGB(0xB4, 0x23, 0x18),
		dangerFill: winapi.RGB(0xD9, 0x2D, 0x20),
	},
	themeDark: {
		window:     winapi.RGB(0x20, 0x20, 0x20),
		panelEdge:  winapi.RGB(0x3D, 0x3D, 0x3D),
		field:      winapi.RGB(0x2B, 0x2B, 0x2B),
		text:       winapi.RGB(0xE8, 0xE8, 0xE8),
		textDim:    winapi.RGB(0x80, 0x80, 0x80),
		track:      winapi.RGB(0x33, 0x33, 0x33),
		trackEdge:  winapi.RGB(0x4A, 0x4A, 0x4A),
		fill:       winapi.RGB(0x3B, 0x82, 0xF6),
		onFill:     winapi.RGB(0xFF, 0xFF, 0xFF),
		danger:     winapi.RGB(0x3B, 0x23, 0x25),
		dangerEdge: winapi.RGB(0x7A, 0x3A, 0x3F),
		dangerText: winapi.RGB(0xFF, 0x8A, 0x80),
		dangerFill: winapi.RGB(0xE5, 0x48, 0x4D),
	},
}

// visual style class names that make the common controls draw dark. They
// are what Explorer itself uses; an older Windows that lacks them simply
// keeps the light look.
const (
	styleDarkButton = "DarkMode_Explorer"
	styleDarkField  = "DarkMode_CFD"
)

// setTheme switches the colour scheme and repaints everything.
func (a *App) setTheme(t theme) {
	if t < 0 || t >= themeCount {
		t = themeLight
	}
	a.theme = t
	a.pal = palettes[t]

	for _, b := range []*uintptr{&a.brushWindow, &a.brushField, &a.brushDanger} {
		if *b != 0 {
			winapi.DeleteObject(*b)
		}
	}
	a.brushWindow = winapi.CreateSolidBrush(a.pal.window)
	a.brushField = winapi.CreateSolidBrush(a.pal.field)
	a.brushDanger = winapi.CreateSolidBrush(a.pal.danger)

	dark := t == themeDark
	winapi.SetDarkTitleBar(a.hwnd, dark)

	buttonStyle, fieldStyle := "", ""
	if dark {
		buttonStyle, fieldStyle = styleDarkButton, styleDarkField
	}
	for _, h := range a.buttons() {
		winapi.SetWindowTheme(h, buttonStyle)
	}
	for _, h := range a.fields() {
		winapi.SetWindowTheme(h, fieldStyle)
	}

	winapi.RedrawAll(a.hwnd)
}

// background returns the colour and brush behind a control: the danger
// panel's for the controls inside it, the window's for everything else.
func (a *App) background(hwnd uintptr) (color, brush uintptr) {
	if a.inDanger[hwnd] {
		return a.pal.danger, a.brushDanger
	}
	return a.pal.window, a.brushWindow
}

// fillRounded paints a rounded rectangle with a one-pixel outline.
func fillRounded(hdc uintptr, r winapi.Rect, radius int32, fill, edge uintptr) {
	brush := winapi.CreateSolidBrush(fill)
	pen := winapi.CreatePen(0 /* PS_SOLID */, 1, edge)
	oldBrush := winapi.SelectObject(hdc, brush)
	oldPen := winapi.SelectObject(hdc, pen)
	winapi.RoundRect(hdc, r.Left, r.Top, r.Right, r.Bottom, radius, radius)
	winapi.SelectObject(hdc, oldBrush)
	winapi.SelectObject(hdc, oldPen)
	winapi.DeleteObject(brush)
	winapi.DeleteObject(pen)
}
