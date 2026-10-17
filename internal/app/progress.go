//go:build windows

package app

import (
	"fmt"

	"mailhelper/internal/winapi"
)

// The progress bar is drawn by hand rather than taken from the common
// controls, so that the message count - "n / m" - can sit inside the bar
// itself, and so that the theme and the DPI scaling stay under the
// application's control.

func (a *App) setProgress(bar uintptr, done, total int) {
	if bar == 0 {
		return
	}
	state, ok := a.progress[bar]
	if !ok {
		return
	}
	state.done, state.total = done, total
	winapi.InvalidateRect(bar, false)
	winapi.UpdateWindow(bar) // repaint now: the worker is about to be busy again
}

func progressProc(hwnd, message, wParam, lParam uintptr) uintptr {
	a := instance
	if a == nil {
		return winapi.DefWindowProc(hwnd, message, wParam, lParam)
	}

	switch message {
	case winapi.WM_ERASEBKGND:
		return 1 // WM_PAINT covers every pixel, so erasing would only flicker

	case winapi.WM_PAINT:
		var ps winapi.PaintStruct
		hdc := winapi.BeginPaint(hwnd, &ps)
		a.paintProgress(hwnd, hdc)
		winapi.EndPaint(hwnd, &ps)
		return 0
	}

	return winapi.DefWindowProc(hwnd, message, wParam, lParam)
}

func (a *App) paintProgress(hwnd, hdc uintptr) {
	state, ok := a.progress[hwnd]
	if !ok {
		return
	}

	r := winapi.GetClientRect(hwnd)
	if r.Width() <= 0 || r.Height() <= 0 {
		return
	}
	radius := a.px(8)

	// The rounded corners leave pixels the shape never covers, so start from
	// the window background rather than whatever the DC happens to hold.
	winapi.FillRect(hdc, &r, a.brushWindow)

	drawRounded := func(right int32, fill, edge uintptr) {
		brush := winapi.CreateSolidBrush(fill)
		pen := winapi.CreatePen(0 /* PS_SOLID */, 1, edge)
		oldBrush := winapi.SelectObject(hdc, brush)
		oldPen := winapi.SelectObject(hdc, pen)
		winapi.RoundRect(hdc, r.Left, r.Top, right, r.Bottom, radius, radius)
		winapi.SelectObject(hdc, oldBrush)
		winapi.SelectObject(hdc, oldPen)
		winapi.DeleteObject(brush)
		winapi.DeleteObject(pen)
	}

	drawRounded(r.Right, colTrack, colTrackEdge)

	filled := r.Left
	if state.total > 0 && state.done > 0 {
		filled = r.Left + int32(float64(r.Width())*float64(state.done)/float64(state.total))
		if filled > r.Right {
			filled = r.Right
		}
		// Below a couple of radii the rounded rectangle degenerates into a
		// sliver, so give the first sliver of progress a visible width.
		if filled < r.Left+radius*2 {
			filled = r.Left + radius*2
		}
		drawRounded(filled, colFill, colFill)
	}

	if state.total == 0 && state.done == 0 {
		return // idle: an empty track, no label
	}

	label := fmt.Sprintf("%d / %d", state.done, state.total)
	winapi.SetBkMode(hdc, winapi.TRANSPARENT)
	oldFont := winapi.SelectObject(hdc, a.font)

	// The label spans the boundary between the filled and empty parts, so it
	// is drawn twice through complementary clips: white on the fill, dark on
	// the track.
	const format = winapi.DT_CENTER | winapi.DT_VCENTER | winapi.DT_SINGLELINE
	for _, half := range []struct {
		left, right int32
		color       uintptr
	}{
		{r.Left, filled, colOnFill},
		{filled, r.Right, colText},
	} {
		if half.right <= half.left {
			continue
		}
		saved := winapi.SaveDC(hdc)
		winapi.IntersectClipRect(hdc, half.left, r.Top, half.right, r.Bottom)
		winapi.SetTextColor(hdc, half.color)
		box := r
		winapi.DrawText(hdc, label, &box, format)
		winapi.RestoreDC(hdc, saved)
	}

	winapi.SelectObject(hdc, oldFont)
}
