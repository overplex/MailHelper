//go:build windows

// Package app is the MailHelper user interface: a plain Win32 window that
// behaves correctly when it is dragged between monitors of different
// resolution and scaling. Nothing is laid out in physical pixels; every size below is a
// logical value multiplied by the scale of the monitor the window is on, and
// WM_DPICHANGED rebuilds the font, the frame and every control position.
package app

import (
	"context"
	"runtime"
	"syscall"
	"unsafe"

	"mailhelper/internal/appicon"
	"mailhelper/internal/i18n"
	"mailhelper/internal/imapc"
	"mailhelper/internal/winapi"
)

// The application is a single window, so one package-level instance keeps
// the window procedures - which Windows calls with nothing but an HWND -
// able to reach the state.
var instance *App

const (
	mainClass     = "MailHelperMainWindow"
	progressClass = "MailHelperProgress"

	// wmWorker tells the UI thread that the worker goroutine has queued
	// events. The events themselves travel over a channel, because a Go
	// value cannot be smuggled through an LPARAM safely.
	wmWorker = winapi.WM_APP + 1
)

// Colours of the light theme.
var (
	colWindow    = winapi.RGB(0xF3, 0xF5, 0xF8)
	colField     = winapi.RGB(0xFF, 0xFF, 0xFF)
	colText      = winapi.RGB(0x1A, 0x1A, 0x1A)
	colTextDim   = winapi.RGB(0x87, 0x8F, 0x9A)
	colTrack     = winapi.RGB(0xE1, 0xE6, 0xED)
	colTrackEdge = winapi.RGB(0xC2, 0xCB, 0xD6)
	colFill      = winapi.RGB(0x2F, 0x6F, 0xEB)
	colOnFill    = winapi.RGB(0xFF, 0xFF, 0xFF)
)

// task identifies what the worker goroutine is doing; taskNone means idle.
type task int

const (
	taskNone task = iota
	taskTest
	taskFolders
	taskCopy
	taskMove
)

// progressState is what one progress bar shows.
type progressState struct {
	done, total int
}

// App holds every window handle and all the UI state.
type App struct {
	hInstance uintptr
	hwnd      uintptr

	dpi   uint32
	scale float64
	font  uintptr

	brushWindow uintptr
	brushField  uintptr

	lang i18n.Lang
	tr   *i18n.Strings

	// Language row.
	lblLang, cbLang uintptr

	// Mailbox group.
	grpMailbox              uintptr
	lblEmail, edEmail       uintptr
	lblPassword, edPassword uintptr
	lblServer, edServer     uintptr
	lblPort, edPort         uintptr
	lblUser, edUser         uintptr
	btnTest, btnRefresh     uintptr

	// "Copy to a folder on disk" group.
	grpCopy                               uintptr
	lblCopySrc, cbCopySrc                 uintptr
	chkMoveAfter                          uintptr
	lblCopyDst, cbCopyDst                 uintptr
	btnSaveDir, lblSaveDir                uintptr
	lblCopyFrom, dtCopyFrom, btnCopyFromX uintptr
	lblCopyTo, dtCopyTo, btnCopyToX       uintptr
	btnCopyRun, pbCopy                    uintptr

	// "Move from folder to folder" group.
	grpMove                               uintptr
	lblMoveSrc, cbMoveSrc                 uintptr
	lblMoveDst, cbMoveDst                 uintptr
	lblMoveFrom, dtMoveFrom, btnMoveFromX uintptr
	lblMoveTo, dtMoveTo, btnMoveToX       uintptr
	btnMoveRun, pbMove                    uintptr

	folders []imapc.Folder
	saveDir string

	// Set while the code, rather than the user, writes into an edit box, so
	// the automatic e-mail -> server/user filling does not mistake its own
	// writes for the user taking over a field.
	suppressEdit  bool
	userSetUser   bool
	userSetServer bool

	running  task
	cancel   context.CancelFunc
	progress map[uintptr]*progressState

	// Set when the user closed the window while a job was still running;
	// the window is destroyed once that job has unwound.
	closeRequested bool

	events chan event
}

// Run creates the window and pumps messages until it closes. iconPNG is the
// artwork for the caption, task bar and Alt-Tab.
func Run(iconPNG []byte) {
	// Window messages must be pumped on the thread that created the window.
	runtime.LockOSThread()

	// Declare DPI awareness before measuring anything, otherwise Windows
	// reports 96 DPI and bitmap-stretches the whole window.
	winapi.DeclareDpiAwareness()
	winapi.CoInitialize()
	winapi.InitCommonControls()

	_ = appicon.Load(iconPNG)

	// Open in the language Windows itself is in when that is one we speak;
	// English otherwise, and the combo box still switches at any time.
	lang := i18n.FromWindowsLangID(winapi.UserUILanguage())

	a := &App{
		hInstance: winapi.GetModuleHandle(),
		lang:      lang,
		tr:        i18n.Get(lang),
		progress:  map[uintptr]*progressState{},
		events:    make(chan event, 256),
	}
	instance = a

	a.brushWindow = winapi.CreateSolidBrush(colWindow)
	a.brushField = winapi.CreateSolidBrush(colField)

	a.registerClasses()
	a.createWindow()
	a.createControls()

	a.applyDPI(winapi.WindowDPI(a.hwnd), nil)
	a.retranslate()
	a.centerOnWorkArea()

	winapi.ShowWindow(a.hwnd, winapi.SW_SHOW)
	winapi.UpdateWindow(a.hwnd)

	var m winapi.Msg
	for {
		ret := winapi.GetMessage(&m)
		if ret <= 0 { // 0 is WM_QUIT, -1 is an error
			break
		}
		if winapi.IsDialogMessage(a.hwnd, &m) {
			continue // keyboard navigation between the controls
		}
		winapi.TranslateMessage(&m)
		winapi.DispatchMessage(&m)
	}
}

func (a *App) registerClasses() {
	cursor := winapi.LoadCursor(winapi.IDC_ARROW)

	main := winapi.WndClassExW{
		CbSize:        uint32(unsafe.Sizeof(winapi.WndClassExW{})),
		Style:         0x0002 | 0x0001, // CS_HREDRAW | CS_VREDRAW
		LpfnWndProc:   syscall.NewCallback(mainProc),
		HInstance:     a.hInstance,
		HCursor:       cursor,
		HbrBackground: a.brushWindow,
		LpszClassName: winapi.UTF16(mainClass),
	}
	if _, err := winapi.RegisterClassEx(&main); err != nil {
		panic("RegisterClassEx(main): " + err.Error())
	}

	progress := winapi.WndClassExW{
		CbSize:        uint32(unsafe.Sizeof(winapi.WndClassExW{})),
		Style:         0x0002 | 0x0001,
		LpfnWndProc:   syscall.NewCallback(progressProc),
		HInstance:     a.hInstance,
		HCursor:       cursor,
		HbrBackground: a.brushWindow,
		LpszClassName: winapi.UTF16(progressClass),
	}
	if _, err := winapi.RegisterClassEx(&progress); err != nil {
		panic("RegisterClassEx(progress): " + err.Error())
	}
}

func (a *App) createWindow() {
	// The size here is a placeholder: applyDPI gives the window its real
	// one once the monitor it landed on is known.
	a.hwnd = winapi.CreateWindowExDefaultPos(winapi.WS_EX_CONTROLPARENT,
		mainClass, a.tr.AppTitle, mainStyle(), 800, 600, a.hInstance)
	if a.hwnd == 0 {
		panic("CreateWindowEx(main) failed")
	}
}

// centerOnWorkArea places the window in the middle of the monitor it opened
// on, clamped to the work area so a tall window is never pushed off-screen.
func (a *App) centerOnWorkArea() {
	work := winapi.WorkArea(a.hwnd)
	if work.Width() == 0 || work.Height() == 0 {
		return
	}
	client := winapi.GetClientRect(a.hwnd)
	frame := winapi.AdjustWindowRect(client, mainStyle(), winapi.WS_EX_CONTROLPARENT, a.dpi)

	w, h := frame.Width(), frame.Height()
	x := work.Left + (work.Width()-w)/2
	y := work.Top + (work.Height()-h)/2
	if y < work.Top {
		y = work.Top
	}
	if x < work.Left {
		x = work.Left
	}
	winapi.SetWindowPos(a.hwnd, 0, x, y, 0, 0, winapi.SWP_NOSIZE|winapi.SWP_NOZORDER|winapi.SWP_NOACTIVATE)
}

// mainStyle is the window style, used both to create the window and to work
// out the frame size for a given client area at a given DPI.
//
// WS_CLIPCHILDREN is deliberately absent. A group box is a child window that
// covers the fields inside it but paints only its frame and caption; clipping
// children out of the parent's background erase would therefore leave the
// pixels inside every group box untouched, and a relayout - toggling the
// checkbox, or moving to a monitor with a different scale - would smear the
// old positions of the controls across the new ones.
func mainStyle() uint32 {
	return winapi.WS_OVERLAPPED | winapi.WS_CAPTION | winapi.WS_SYSMENU |
		winapi.WS_MINIMIZEBOX
}

// setWindowIcons renders the artwork at the sizes the shell asks for at this
// DPI and hands them to the window.
func (a *App) setWindowIcons() {
	for _, spec := range []struct {
		metric int32
		which  uintptr
	}{
		{winapi.SM_CXSMICON, winapi.ICON_SMALL},
		{winapi.SM_CXICON, winapi.ICON_BIG},
	} {
		size := winapi.SystemMetric(spec.metric, a.dpi)
		if size <= 0 {
			continue
		}
		hIcon, err := appicon.New(size)
		if err != nil {
			continue // an icon is cosmetic; carry on without one
		}
		if old := winapi.SendMessage(a.hwnd, winapi.WM_SETICON, spec.which, hIcon); old != 0 {
			winapi.DestroyIcon(old)
		}
	}
}

// ---------------------------------------------------------- window procedure

func mainProc(hwnd, message, wParam, lParam uintptr) uintptr {
	a := instance
	if a == nil {
		return winapi.DefWindowProc(hwnd, message, wParam, lParam)
	}

	switch message {
	case winapi.WM_COMMAND:
		a.onCommand(uint16(wParam), uint16(wParam>>16), lParam)
		return 0

	case winapi.WM_NOTIFY:
		hdr := winapi.StructFromLParam[winapi.NMHDR](lParam)
		if hdr.Code == winapi.DTN_DATETIMECHANGE {
			winapi.InvalidateRect(hdr.HwndFrom, false)
		}
		return 0

	case wmWorker:
		a.drainEvents()
		return 0

	case winapi.WM_CTLCOLOREDIT:
		winapi.SetBkColor(wParam, colField)
		winapi.SetTextColor(wParam, colText)
		return a.brushField

	case winapi.WM_CTLCOLORSTATIC, winapi.WM_CTLCOLORBTN:
		winapi.SetBkColor(wParam, colWindow)
		if winapi.IsWindowEnabled(lParam) {
			winapi.SetTextColor(wParam, colText)
		} else {
			winapi.SetTextColor(wParam, colTextDim)
		}
		return a.brushWindow

	case winapi.WM_DPICHANGED:
		// The window moved to a monitor with a different DPI: rebuild the
		// layout at the new scale instead of letting Windows stretch it.
		a.applyDPI(uint32(uint16(wParam)), winapi.StructFromLParam[winapi.Rect](lParam))
		return 0

	case winapi.WM_CLOSE:
		if a.running != taskNone {
			// A job is in flight; ask it to stop and close once it has
			// unwound, rather than tearing the connection down under it.
			a.closeRequested = true
			a.stopRunning()
			return 0
		}
		winapi.DestroyWindow(hwnd)
		return 0

	case winapi.WM_DESTROY:
		winapi.PostQuitMessage(0)
		return 0
	}

	return winapi.DefWindowProc(hwnd, message, wParam, lParam)
}
