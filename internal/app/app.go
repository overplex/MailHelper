//go:build windows

// Package app is the MailHelper user interface: a plain Win32 window that
// behaves correctly when it is dragged between monitors of different
// resolution and scaling. Nothing is laid out in physical pixels; every size
// below is a logical value multiplied by the scale of the monitor the window
// is on, and WM_DPICHANGED rebuilds the fonts, the frame and every control
// position.
package app

import (
	"context"
	"runtime"
	"syscall"
	"unsafe"

	"mailhelper/internal/appicon"
	"mailhelper/internal/i18n"
	"mailhelper/internal/imapc"
	"mailhelper/internal/settings"
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

// task identifies what the worker goroutine is doing; taskNone means idle.
type task int

const (
	taskNone task = iota
	taskTest
	taskFolders
	taskCopy
	taskMove
	taskDelete
)

// progressState is what one progress bar shows.
type progressState struct {
	done, total int
	danger      bool // drawn in the colours of the danger panel
}

// dateFilter is one end of a period: a checkbox that switches the filter on
// and the picker holding the date.
type dateFilter struct {
	check, picker uintptr
}

// App holds every window handle and all the UI state.
type App struct {
	hInstance uintptr
	hwnd      uintptr

	dpi      uint32
	scale    float64
	font     uintptr
	fontBold uintptr

	theme       theme
	pal         palette
	brushWindow uintptr
	brushField  uintptr
	brushDanger uintptr

	lang i18n.Lang
	tr   *i18n.Strings

	// Top row.
	lblLang, cbLang   uintptr
	lblTheme, cbTheme uintptr

	// Panels, in logical coordinates, refreshed by every layout pass.
	panels [panelCount]panelRect

	// "Mailbox" panel.
	lblEmail, edEmail       uintptr
	lblPassword, edPassword uintptr
	lblUser, edUser         uintptr
	lblServer, edServer     uintptr
	lblPort, edPort         uintptr
	btnTest, btnRefresh     uintptr

	// "Delete messages" panel.
	lblDeleteSrc, cbDeleteSrc uintptr
	deleteFrom, deleteTo      dateFilter
	btnDeleteRun, pbDelete    uintptr

	// "Copy to a folder on disk" panel.
	lblCopySrc, cbCopySrc  uintptr
	chkMoveAfter           uintptr
	lblCopyDst, cbCopyDst  uintptr
	btnSaveDir, lblSaveDir uintptr
	copyFrom, copyTo       dateFilter
	btnCopyRun, pbCopy     uintptr

	// "Move from folder to folder" panel.
	lblMoveSrc, cbMoveSrc uintptr
	lblMoveDst, cbMoveDst uintptr
	moveFrom, moveTo      dateFilter
	btnMoveRun, pbMove    uintptr

	// inDanger marks the controls that sit on the danger panel, so they are
	// painted on its background.
	inDanger map[uintptr]bool

	folders []imapc.Folder
	saveDir string

	// wanted holds folder selections restored from the settings, by combo
	// box, until a folder list arrives that they can be applied to.
	wanted map[uintptr]string

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

	_ = appicon.Load(iconPNG) // without artwork the window keeps the default icon

	// A missing or unreadable settings file just means starting from the
	// defaults; it is written afresh when the window closes.
	saved, found, _ := settings.Load()

	a := &App{
		hInstance: winapi.GetModuleHandle(),
		progress:  map[uintptr]*progressState{},
		inDanger:  map[uintptr]bool{},
		wanted:    map[uintptr]string{},
		events:    make(chan event, 256),
	}
	instance = a

	a.lang = startLanguage(saved, found)
	a.tr = i18n.Get(a.lang)

	a.registerClasses()
	a.createWindow()
	a.createControls()
	a.applySettings(saved)

	a.setTheme(startTheme(saved, found))
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

// startLanguage is the saved language, or - on the first run - the language
// Windows itself is displayed in when that is one we speak, English
// otherwise.
func startLanguage(saved settings.Settings, found bool) i18n.Lang {
	if found {
		if l, ok := i18n.FromCode(saved.Language); ok {
			return l
		}
	}
	return i18n.FromWindowsLangID(winapi.UserUILanguage())
}

// startTheme is the saved theme, or - on the first run - the one Windows is
// set to use for applications.
func startTheme(saved settings.Settings, found bool) theme {
	if found {
		if t, ok := themeFromCode(saved.Theme); ok {
			return t
		}
	}
	if winapi.SystemPrefersDark() {
		return themeDark
	}
	return themeLight
}

func (a *App) registerClasses() {
	cursor := winapi.LoadCursor(winapi.IDC_ARROW)

	// Neither class has a background brush: both windows erase themselves
	// in the colours of the current theme.
	for _, c := range []struct {
		name string
		proc func(hwnd, message, wParam, lParam uintptr) uintptr
	}{
		{mainClass, mainProc},
		{progressClass, progressProc},
	} {
		wc := winapi.WndClassExW{
			CbSize:        uint32(unsafe.Sizeof(winapi.WndClassExW{})),
			Style:         0x0002 | 0x0001, // CS_HREDRAW | CS_VREDRAW
			LpfnWndProc:   syscall.NewCallback(c.proc),
			HInstance:     a.hInstance,
			HCursor:       cursor,
			LpszClassName: winapi.UTF16(c.name),
		}
		if _, err := winapi.RegisterClassEx(&wc); err != nil {
			panic("RegisterClassEx(" + c.name + "): " + err.Error())
		}
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
	x := max(work.Left+(work.Width()-w)/2, work.Left)
	y := max(work.Top+(work.Height()-h)/2, work.Top)
	winapi.SetWindowPos(a.hwnd, 0, x, y, 0, 0, winapi.SWP_NOSIZE|winapi.SWP_NOZORDER|winapi.SWP_NOACTIVATE)
}

// mainStyle is the window style, used both to create the window and to work
// out the frame size for a given client area at a given DPI.
//
// WS_CLIPCHILDREN is deliberately absent: the panels are painted by the
// window itself underneath the controls, and invalidating the window must
// invalidate the controls on top of them too, which is what Windows does
// for a parent without that style.
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
		a.onCommand(uint16(wParam), uint16(wParam>>16))
		return 0

	case wmWorker:
		a.drainEvents()
		return 0

	case winapi.WM_ERASEBKGND:
		r := winapi.GetClientRect(hwnd)
		winapi.FillRect(wParam, &r, a.brushWindow)
		return 1

	case winapi.WM_PAINT:
		var ps winapi.PaintStruct
		hdc := winapi.BeginPaint(hwnd, &ps)
		a.paintPanels(hdc)
		winapi.EndPaint(hwnd, &ps)
		return 0

	case winapi.WM_CTLCOLOREDIT, winapi.WM_CTLCOLORLISTBOX:
		winapi.SetBkColor(wParam, a.pal.field)
		winapi.SetTextColor(wParam, a.pal.text)
		return a.brushField

	case winapi.WM_CTLCOLORSTATIC, winapi.WM_CTLCOLORBTN:
		color, brush := a.background(lParam)
		winapi.SetBkColor(wParam, color)
		if winapi.IsWindowEnabled(lParam) {
			winapi.SetTextColor(wParam, a.pal.text)
		} else {
			winapi.SetTextColor(wParam, a.pal.textDim)
		}
		return brush

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
		a.saveSettings()
		winapi.DestroyWindow(hwnd)
		return 0

	case winapi.WM_DESTROY:
		winapi.PostQuitMessage(0)
		return 0
	}

	return winapi.DefWindowProc(hwnd, message, wParam, lParam)
}
