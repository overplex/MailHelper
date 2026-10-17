//go:build windows

// Package winapi is a thin, hand-rolled binding over the Win32 controls this
// application needs: the standard library plus syscall, no cgo and no UI
// toolkit, which is what makes correct per-monitor-v2 DPI behaviour possible -
// every size is a logical value the application scales itself.
package winapi

import (
	"runtime"
	"syscall"
	"unsafe"
)

// ------------------------------------------------------------------- DLLs

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	comctl32 = syscall.NewLazyDLL("comctl32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")
	uxtheme  = syscall.NewLazyDLL("uxtheme.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")

	procGetModuleHandleW         = kernel32.NewProc("GetModuleHandleW")
	procGetUserDefaultUILanguage = kernel32.NewProc("GetUserDefaultUILanguage")
	procGetUserDefaultLangID     = kernel32.NewProc("GetUserDefaultLangID")

	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procIsDialogMessageW    = user32.NewProc("IsDialogMessageW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procUpdateWindow        = user32.NewProc("UpdateWindow")
	procLoadCursorW         = user32.NewProc("LoadCursorW")
	procAdjustWindowRectEx  = user32.NewProc("AdjustWindowRectEx")
	procSendMessageW        = user32.NewProc("SendMessageW")
	procPostMessageW        = user32.NewProc("PostMessageW")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procMoveWindow          = user32.NewProc("MoveWindow")
	procEnableWindow        = user32.NewProc("EnableWindow")
	procSetWindowTextW      = user32.NewProc("SetWindowTextW")
	procGetWindowTextW      = user32.NewProc("GetWindowTextW")
	procGetWindowTextLength = user32.NewProc("GetWindowTextLengthW")
	procInvalidateRect      = user32.NewProc("InvalidateRect")
	procMessageBoxW         = user32.NewProc("MessageBoxW")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	procSetProcessDPIAware  = user32.NewProc("SetProcessDPIAware")
	procGetDC               = user32.NewProc("GetDC")
	procReleaseDC           = user32.NewProc("ReleaseDC")
	procDestroyIcon         = user32.NewProc("DestroyIcon")
	procGetClientRect       = user32.NewProc("GetClientRect")
	procBeginPaint          = user32.NewProc("BeginPaint")
	procEndPaint            = user32.NewProc("EndPaint")
	procFillRect            = user32.NewProc("FillRect")
	procDrawTextW           = user32.NewProc("DrawTextW")
	procIsWindowEnabled     = user32.NewProc("IsWindowEnabled")
	procMonitorFromWindow   = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW     = user32.NewProc("GetMonitorInfoW")
	procCreateIconIndirect  = user32.NewProc("CreateIconIndirect")

	// Per-monitor DPI entry points; every call site checks Find first and
	// falls back to the system-DPI equivalent on older Windows.
	procGetDpiForWindow           = user32.NewProc("GetDpiForWindow")
	procAdjustWindowRectExForDpi  = user32.NewProc("AdjustWindowRectExForDpi")
	procGetSystemMetricsForDpi    = user32.NewProc("GetSystemMetricsForDpi")
	procSetProcessDpiAwarenessCtx = user32.NewProc("SetProcessDpiAwarenessContext")

	procCreateSolidBrush   = gdi32.NewProc("CreateSolidBrush")
	procCreatePen          = gdi32.NewProc("CreatePen")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procRoundRect          = gdi32.NewProc("RoundRect")
	procSetBkMode          = gdi32.NewProc("SetBkMode")
	procSetBkColor         = gdi32.NewProc("SetBkColor")
	procSetTextColor       = gdi32.NewProc("SetTextColor")
	procCreateFontW        = gdi32.NewProc("CreateFontW")
	procGetDeviceCaps      = gdi32.NewProc("GetDeviceCaps")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSaveDC             = gdi32.NewProc("SaveDC")
	procRestoreDC          = gdi32.NewProc("RestoreDC")
	procIntersectClipRect  = gdi32.NewProc("IntersectClipRect")
	procCreateBitmap       = gdi32.NewProc("CreateBitmap")
	procDeleteDC           = gdi32.NewProc("DeleteDC")

	procInitCommonControlsEx = comctl32.NewProc("InitCommonControlsEx")

	procSHBrowseForFolderW   = shell32.NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDListW = shell32.NewProc("SHGetPathFromIDListW")

	procCoInitializeEx = ole32.NewProc("CoInitializeEx")
	procCoTaskMemFree  = ole32.NewProc("CoTaskMemFree")

	procGetTextExtentPoint32W = gdi32.NewProc("GetTextExtentPoint32W")
	procRedrawWindow          = user32.NewProc("RedrawWindow")
	procSetWindowTheme        = uxtheme.NewProc("SetWindowTheme")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
	procRegGetValueW          = advapi32.NewProc("RegGetValueW")
	procSetWindowLongPtrW     = user32.NewProc("SetWindowLongPtrW")
	procCallWindowProcW       = user32.NewProc("CallWindowProcW")
	procBitBlt                = gdi32.NewProc("BitBlt")
)

// -------------------------------------------------------------- Constants

const (
	WS_OVERLAPPED  = 0x00000000
	WS_CAPTION     = 0x00C00000
	WS_SYSMENU     = 0x00080000
	WS_MINIMIZEBOX = 0x00020000
	WS_CHILD       = 0x40000000
	WS_VISIBLE     = 0x10000000
	WS_TABSTOP     = 0x00010000
	WS_BORDER      = 0x00800000
	WS_VSCROLL     = 0x00200000

	WS_EX_CONTROLPARENT = 0x00010000

	CW_USEDEFAULT = ^uintptr(0x7FFFFFFF) // 0x80000000 read as a signed int32

	// Button styles.
	BS_PUSHBUTTON   = 0x00000000
	BS_AUTOCHECKBOX = 0x00000003
	BS_MULTILINE    = 0x00002000

	// Edit styles.
	ES_LEFT        = 0x0000
	ES_AUTOHSCROLL = 0x0080
	ES_PASSWORD    = 0x0020
	ES_NUMBER      = 0x2000

	// Static styles.
	SS_LEFT        = 0x00000000
	SS_CENTERIMAGE = 0x00000200

	// Combo box styles.
	CBS_DROPDOWNLIST = 0x0003
	CBS_HASSTRINGS   = 0x0200

	// Date/time picker styles.
	DTS_SHORTDATEFORMAT = 0x0000

	// Messages.
	WM_DESTROY         = 0x0002
	WM_PAINT           = 0x000F
	WM_CLOSE           = 0x0010
	WM_ERASEBKGND      = 0x0014
	WM_SETFONT         = 0x0030
	WM_SETICON         = 0x0080
	WM_COMMAND         = 0x0111
	WM_CTLCOLOREDIT    = 0x0133
	WM_CTLCOLORLISTBOX = 0x0134
	WM_CTLCOLORBTN     = 0x0135
	WM_CTLCOLORSTATIC  = 0x0138
	WM_DPICHANGED      = 0x02E0
	WM_APP             = 0x8000

	// Control messages.
	BM_GETCHECK = 0x00F0
	BM_SETCHECK = 0x00F1

	CB_ADDSTRING    = 0x0143
	CB_GETCURSEL    = 0x0147
	CB_RESETCONTENT = 0x014B
	CB_SETCURSEL    = 0x014E

	DTM_FIRST         = 0x1000
	DTM_GETSYSTEMTIME = DTM_FIRST + 1

	GDT_VALID = 0

	// Notification codes.
	BN_CLICKED    = 0
	EN_CHANGE     = 0x0300
	CBN_SELCHANGE = 1

	BST_CHECKED = 1

	// SetWindowPos flags.
	SWP_NOSIZE     = 0x0001
	SWP_NOMOVE     = 0x0002
	SWP_NOZORDER   = 0x0004
	SWP_NOACTIVATE = 0x0010

	SW_SHOW = 5
	SW_HIDE = 0

	IDC_ARROW = 32512

	TRANSPARENT = 1

	DT_LEFT         = 0x00000000
	DT_CENTER       = 0x00000001
	DT_VCENTER      = 0x00000004
	DT_SINGLELINE   = 0x00000020
	DT_END_ELLIPSIS = 0x00008000

	LOGPIXELSX = 88

	SM_CXICON   = 11
	SM_CXSMICON = 49

	ICON_SMALL = 0
	ICON_BIG   = 1

	// MessageBox flags.
	MB_OK              = 0x00000000
	MB_YESNO           = 0x00000004
	MB_ICONERROR       = 0x00000010
	MB_ICONWARNING     = 0x00000030
	MB_ICONINFORMATION = 0x00000040
	MB_DEFBUTTON2      = 0x00000100

	IDYES = 6

	// InitCommonControlsEx classes.
	ICC_WIN95_CLASSES    = 0x000000FF
	ICC_DATE_CLASSES     = 0x00000100
	ICC_STANDARD_CLASSES = 0x00004000

	// SHBrowseForFolder flags.
	BIF_RETURNONLYFSDIRS = 0x00000001
	BIF_NEWDIALOGSTYLE   = 0x00000040

	// SHBrowseForFolder callback messages.
	BFFM_INITIALIZED   = 1
	BFFM_SETSELECTIONW = 0x0400 + 103 // WM_USER + 103

	// GetWindowLongPtr indices.

	MONITOR_DEFAULTTONEAREST = 2
)

// ------------------------------------------------------------------ Types

type Point struct{ X, Y int32 }

type Rect struct{ Left, Top, Right, Bottom int32 }

func (r Rect) Width() int32  { return r.Right - r.Left }
func (r Rect) Height() int32 { return r.Bottom - r.Top }

type Msg struct {
	Hwnd    uintptr
	Message uint32
	_       uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      Point
	_       uint32
}

type WndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type PaintStruct struct {
	Hdc         uintptr
	FErase      int32
	RcPaint     Rect
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}

type SystemTime struct {
	Year         uint16
	Month        uint16
	DayOfWeek    uint16
	Day          uint16
	Hour         uint16
	Minute       uint16
	Second       uint16
	Milliseconds uint16
}

type InitCommonControlsExStruct struct {
	Size uint32
	ICC  uint32
}

type BrowseInfoW struct {
	HwndOwner      uintptr
	PidlRoot       uintptr
	PszDisplayName *uint16
	LpszTitle      *uint16
	UlFlags        uint32
	_              uint32
	Lpfn           uintptr
	LParam         uintptr
	IImage         int32
	_              int32
}

type MonitorInfo struct {
	CbSize    uint32
	RcMonitor Rect
	RcWork    Rect
	DwFlags   uint32
}

type BitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type IconInfo struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	_        uint32
	HbmMask  uintptr
	HbmColor uintptr
}

// ---------------------------------------------------------------- Helpers

// UTF16 converts a Go string to a NUL-terminated UTF-16 pointer. Strings
// containing a NUL - which cannot come from the UI - degrade to empty.
func UTF16(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return &[]uint16{0}[0]
	}
	return p
}

// StructFromLParam reinterprets an LPARAM as a pointer to a structure
// Windows owns. A window procedure has no other way to read the messages it
// is handed, and the memory stays alive for the duration of the call. The
// memory belongs to Windows rather than the Go heap, so the garbage
// collector has nothing to track; the value is reinterpreted through its
// address, the form go vet accepts for such a conversion.
func StructFromLParam[T any](lParam uintptr) *T {
	return *(**T)(unsafe.Pointer(&lParam))
}

func RGB(r, g, b byte) uintptr {
	return uintptr(r) | uintptr(g)<<8 | uintptr(b)<<16
}

func boolToUintptr(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}

// ------------------------------------------------------------ Win32 calls

func GetModuleHandle() uintptr {
	h, _, _ := procGetModuleHandleW.Call(0)
	return h
}

// UserUILanguage reports the LANGID of the language Windows displays its own
// interface in. It falls back to the user's default locale, and to zero when
// neither can be read - which the caller takes as "not known".
func UserUILanguage() uint16 {
	if procGetUserDefaultUILanguage.Find() == nil {
		if id, _, _ := procGetUserDefaultUILanguage.Call(); id != 0 {
			return uint16(id)
		}
	}
	if procGetUserDefaultLangID.Find() == nil {
		if id, _, _ := procGetUserDefaultLangID.Call(); id != 0 {
			return uint16(id)
		}
	}
	return 0
}

func RegisterClassEx(wc *WndClassExW) (uintptr, error) {
	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(wc)))
	if atom == 0 {
		return 0, err
	}
	return atom, nil
}

func CreateWindowEx(exStyle uint32, class, title string, style uint32,
	x, y, w, h int32, parent, menu, instance uintptr) uintptr {
	hwnd, _, _ := procCreateWindowExW.Call(
		uintptr(exStyle),
		uintptr(unsafe.Pointer(UTF16(class))),
		uintptr(unsafe.Pointer(UTF16(title))),
		uintptr(style),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, menu, instance, 0,
	)
	return hwnd
}

// CreateWindowExDefaultPos is CreateWindowEx with CW_USEDEFAULT placement,
// which cannot be expressed through the int32 parameters above.
func CreateWindowExDefaultPos(exStyle uint32, class, title string, style uint32,
	w, h int32, instance uintptr) uintptr {
	hwnd, _, _ := procCreateWindowExW.Call(
		uintptr(exStyle),
		uintptr(unsafe.Pointer(UTF16(class))),
		uintptr(unsafe.Pointer(UTF16(title))),
		uintptr(style),
		CW_USEDEFAULT, CW_USEDEFAULT, uintptr(w), uintptr(h),
		0, 0, instance, 0,
	)
	return hwnd
}

func DefWindowProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

func DestroyWindow(hwnd uintptr)         { procDestroyWindow.Call(hwnd) }
func PostQuitMessage(code int32)         { procPostQuitMessage.Call(uintptr(code)) }
func ShowWindow(hwnd uintptr, cmd int32) { procShowWindow.Call(hwnd, uintptr(cmd)) }
func UpdateWindow(hwnd uintptr)          { procUpdateWindow.Call(hwnd) }

func GetMessage(m *Msg) int32 {
	r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(m)), 0, 0, 0)
	return int32(r)
}

func TranslateMessage(m *Msg) { procTranslateMessage.Call(uintptr(unsafe.Pointer(m))) }
func DispatchMessage(m *Msg)  { procDispatchMessageW.Call(uintptr(unsafe.Pointer(m))) }

func IsDialogMessage(hwnd uintptr, m *Msg) bool {
	r, _, _ := procIsDialogMessageW.Call(hwnd, uintptr(unsafe.Pointer(m)))
	return r != 0
}

func LoadCursor(id uintptr) uintptr {
	h, _, _ := procLoadCursorW.Call(0, id)
	return h
}

func SendMessage(hwnd, msg, wParam, lParam uintptr) uintptr {
	r, _, _ := procSendMessageW.Call(hwnd, msg, wParam, lParam)
	return r
}

func PostMessage(hwnd, msg, wParam, lParam uintptr) {
	procPostMessageW.Call(hwnd, msg, wParam, lParam)
}

func SetWindowPos(hwnd, after uintptr, x, y, w, h int32, flags uint32) {
	procSetWindowPos.Call(hwnd, after, uintptr(x), uintptr(y), uintptr(w), uintptr(h), uintptr(flags))
}

func MoveWindow(hwnd uintptr, x, y, w, h int32, repaint bool) {
	procMoveWindow.Call(hwnd, uintptr(x), uintptr(y), uintptr(w), uintptr(h), boolToUintptr(repaint))
}

func EnableWindow(hwnd uintptr, enable bool) {
	procEnableWindow.Call(hwnd, boolToUintptr(enable))
}

func IsWindowEnabled(hwnd uintptr) bool {
	r, _, _ := procIsWindowEnabled.Call(hwnd)
	return r != 0
}

func SetWindowText(hwnd uintptr, s string) {
	procSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(UTF16(s))))
}

func GetWindowText(hwnd uintptr) string {
	n, _, _ := procGetWindowTextLength.Call(hwnd)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	got, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf[:got])
}

func InvalidateRect(hwnd uintptr, erase bool) {
	procInvalidateRect.Call(hwnd, 0, boolToUintptr(erase))
}

func MessageBox(owner uintptr, text, caption string, flags uint32) int32 {
	r, _, _ := procMessageBoxW.Call(owner,
		uintptr(unsafe.Pointer(UTF16(text))),
		uintptr(unsafe.Pointer(UTF16(caption))),
		uintptr(flags))
	return int32(r)
}

func GetClientRect(hwnd uintptr) Rect {
	var r Rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return r
}

func BeginPaint(hwnd uintptr, ps *PaintStruct) uintptr {
	hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(ps)))
	return hdc
}

func EndPaint(hwnd uintptr, ps *PaintStruct) {
	procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(ps)))
}

func FillRect(hdc uintptr, r *Rect, brush uintptr) {
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(r)), brush)
}

func DrawText(hdc uintptr, text string, r *Rect, format uint32) {
	procDrawTextW.Call(hdc,
		uintptr(unsafe.Pointer(UTF16(text))), ^uintptr(0),
		uintptr(unsafe.Pointer(r)), uintptr(format))
}

func CreateSolidBrush(color uintptr) uintptr {
	h, _, _ := procCreateSolidBrush.Call(color)
	return h
}

func CreatePen(style int32, width int32, color uintptr) uintptr {
	h, _, _ := procCreatePen.Call(uintptr(style), uintptr(width), color)
	return h
}

func SelectObject(hdc, obj uintptr) uintptr {
	h, _, _ := procSelectObject.Call(hdc, obj)
	return h
}

func DeleteObject(obj uintptr) { procDeleteObject.Call(obj) }

func RoundRect(hdc uintptr, l, t, r, b, rw, rh int32) {
	procRoundRect.Call(hdc, uintptr(l), uintptr(t), uintptr(r), uintptr(b), uintptr(rw), uintptr(rh))
}

func SetBkMode(hdc uintptr, mode int32)   { procSetBkMode.Call(hdc, uintptr(mode)) }
func SetBkColor(hdc uintptr, c uintptr)   { procSetBkColor.Call(hdc, c) }
func SetTextColor(hdc uintptr, c uintptr) { procSetTextColor.Call(hdc, c) }

// CreateFont builds a Segoe UI face. A negative height asks for a character
// height in pixels, which is how point sizes map onto a DPI.
func CreateFont(height int32, weight int32) uintptr {
	h, _, _ := procCreateFontW.Call(
		uintptr(height), 0, 0, 0,
		uintptr(weight),
		0, 0, 0,
		1, // DEFAULT_CHARSET
		0, 0,
		5, // CLEARTYPE_QUALITY
		0,
		uintptr(unsafe.Pointer(UTF16("Segoe UI"))),
	)
	return h
}

func DestroyIcon(h uintptr) { procDestroyIcon.Call(h) }

// SaveDC/RestoreDC bracket a clipping change, which is how the progress bar
// paints its label in two colours across the filled boundary.
func SaveDC(hdc uintptr) int32 {
	r, _, _ := procSaveDC.Call(hdc)
	return int32(r)
}

func RestoreDC(hdc uintptr, state int32) {
	procRestoreDC.Call(hdc, uintptr(state))
}

func IntersectClipRect(hdc uintptr, l, t, r, b int32) {
	procIntersectClipRect.Call(hdc, uintptr(l), uintptr(t), uintptr(r), uintptr(b))
}

func InitCommonControls() {
	icc := InitCommonControlsExStruct{
		Size: uint32(unsafe.Sizeof(InitCommonControlsExStruct{})),
		ICC:  ICC_WIN95_CLASSES | ICC_DATE_CLASSES | ICC_STANDARD_CLASSES,
	}
	procInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&icc)))
}

// CoInitialize puts the calling thread into an STA, which the shell folder
// dialog requires.
func CoInitialize() {
	const coinitApartmentThreaded = 0x2
	procCoInitializeEx.Call(0, coinitApartmentThreaded)
}

// ---------------------------------------------------------------- DPI

// DeclareDpiAwareness asks for per-monitor v2 awareness. The embedded
// manifest already declares it; this call only matters for builds made
// without the resource file and harmlessly fails when the manifest won.
func DeclareDpiAwareness() {
	if procSetProcessDpiAwarenessCtx.Find() == nil {
		const perMonitorAwareV2 = ^uintptr(3) // the context value is -4
		if ok, _, _ := procSetProcessDpiAwarenessCtx.Call(perMonitorAwareV2); ok != 0 {
			return
		}
	}
	procSetProcessDPIAware.Call()
}

// WindowDPI reports the DPI of the monitor the window sits on, falling back
// to the system-wide value where per-monitor DPI is unavailable.
func WindowDPI(hwnd uintptr) uint32 {
	if procGetDpiForWindow.Find() == nil {
		if dpi, _, _ := procGetDpiForWindow.Call(hwnd); dpi > 0 {
			return uint32(dpi)
		}
	}
	dc, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, dc)
	if dpi, _, _ := procGetDeviceCaps.Call(dc, LOGPIXELSX); dpi > 0 {
		return uint32(dpi)
	}
	return 96
}

// AdjustWindowRect turns a client rectangle into the window rectangle that
// contains it, using the frame metrics of the given DPI.
func AdjustWindowRect(client Rect, style, exStyle uint32, dpi uint32) Rect {
	if procAdjustWindowRectExForDpi.Find() == nil {
		procAdjustWindowRectExForDpi.Call(uintptr(unsafe.Pointer(&client)),
			uintptr(style), 0, uintptr(exStyle), uintptr(dpi))
		return client
	}
	procAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&client)),
		uintptr(style), 0, uintptr(exStyle))
	return client
}

// SystemMetric reads a metric scaled for the given DPI where Windows offers
// that, and the system-wide one otherwise.
func SystemMetric(metric int32, dpi uint32) int32 {
	if procGetSystemMetricsForDpi.Find() == nil {
		if v, _, _ := procGetSystemMetricsForDpi.Call(uintptr(metric), uintptr(dpi)); v > 0 {
			return int32(v)
		}
	}
	v, _, _ := procGetSystemMetrics.Call(uintptr(metric))
	return int32(v)
}

// WorkArea returns the usable rectangle of the monitor holding the window.
func WorkArea(hwnd uintptr) Rect {
	mon, _, _ := procMonitorFromWindow.Call(hwnd, MONITOR_DEFAULTTONEAREST)
	mi := MonitorInfo{CbSize: uint32(unsafe.Sizeof(MonitorInfo{}))}
	if ok, _, _ := procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); ok == 0 {
		return Rect{}
	}
	return mi.RcWork
}

// ------------------------------------------------------- Control wrappers

func ComboReset(hwnd uintptr) { SendMessage(hwnd, CB_RESETCONTENT, 0, 0) }

func ComboAdd(hwnd uintptr, s string) {
	SendMessage(hwnd, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(UTF16(s))))
}

func ComboSetSel(hwnd uintptr, index int) {
	SendMessage(hwnd, CB_SETCURSEL, uintptr(int32(index)), 0)
}

func ComboGetSel(hwnd uintptr) int {
	return int(int32(SendMessage(hwnd, CB_GETCURSEL, 0, 0)))
}

func Checked(hwnd uintptr) bool {
	return SendMessage(hwnd, BM_GETCHECK, 0, 0) == BST_CHECKED
}

func SetChecked(hwnd uintptr, on bool) {
	SendMessage(hwnd, BM_SETCHECK, boolToUintptr(on), 0)
}

// DateGet reads a date/time picker. ok is false when the control holds no
// valid date.
func DateGet(hwnd uintptr) (st SystemTime, ok bool) {
	r := SendMessage(hwnd, DTM_GETSYSTEMTIME, 0, uintptr(unsafe.Pointer(&st)))
	return st, r == GDT_VALID
}

// ------------------------------------------------------------ Folder pick

// browseCallback preselects the folder handed over in LParam as soon as the
// dialog is up, so reopening it lands on the previous choice.
var browseCallback = syscall.NewCallback(func(hwnd uintptr, msg uint32, _, data uintptr) uintptr {
	if msg == BFFM_INITIALIZED && data != 0 {
		procSendMessageW.Call(hwnd, BFFM_SETSELECTIONW, 1, data)
	}
	return 0
})

// BrowseForFolder shows the shell folder picker and returns the chosen path,
// or an empty string when the user cancelled. initial, when it names an
// existing path, is the folder the tree opens on.
func BrowseForFolder(owner uintptr, title, initial string) string {
	display := make([]uint16, 260)
	bi := BrowseInfoW{
		HwndOwner:      owner,
		PszDisplayName: &display[0],
		LpszTitle:      UTF16(title),
		UlFlags:        BIF_RETURNONLYFSDIRS | BIF_NEWDIALOGSTYLE,
	}
	var initialPath *uint16
	if initial != "" {
		initialPath = UTF16(initial)
		bi.Lpfn = browseCallback
		bi.LParam = uintptr(unsafe.Pointer(initialPath))
	}
	pidl, _, _ := procSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
	runtime.KeepAlive(initialPath)
	if pidl == 0 {
		return ""
	}
	defer procCoTaskMemFree.Call(pidl)

	buf := make([]uint16, 32768) // long-path aware manifest allows more than MAX_PATH
	if ok, _, _ := procSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&buf[0]))); ok == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// ------------------------------------------------------------------ Icons

// NewIconFromBGRA builds an HICON from straight-alpha BGRA pixels of the
// given square size. The caller owns the returned handle.
func NewIconFromBGRA(pix []byte, size int32) uintptr {
	hdc, _, _ := procCreateCompatibleDC.Call(0)
	if hdc == 0 {
		return 0
	}
	defer procDeleteDC.Call(hdc)

	bi := BitmapInfoHeader{
		Size:     uint32(unsafe.Sizeof(BitmapInfoHeader{})),
		Width:    size,
		Height:   -size, // negative height means a top-down bitmap
		Planes:   1,
		BitCount: 32,
	}
	var bits unsafe.Pointer
	hbm, _, _ := procCreateDIBSection.Call(hdc, uintptr(unsafe.Pointer(&bi)), 0,
		uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbm == 0 {
		return 0
	}
	defer procDeleteObject.Call(hbm)

	// The colour bitmap is consumed by AlphaBlend, which wants premultiplied
	// alpha, unlike the straight alpha stored in image files.
	dst := unsafe.Slice((*byte)(bits), int(size)*int(size)*4)
	for i := 0; i+3 < len(pix) && i+3 < len(dst); i += 4 {
		a := int(pix[i+3])
		dst[i+0] = byte(int(pix[i+0]) * a / 255)
		dst[i+1] = byte(int(pix[i+1]) * a / 255)
		dst[i+2] = byte(int(pix[i+2]) * a / 255)
		dst[i+3] = pix[i+3]
	}

	// CreateIconIndirect still wants a mask even though the 32bpp colour
	// bitmap carries alpha; an all-zero mask means fully opaque.
	maskStride := ((int(size) + 31) / 32) * 4
	mask := make([]byte, maskStride*int(size))
	hbmMask, _, _ := procCreateBitmap.Call(uintptr(size), uintptr(size), 1, 1,
		uintptr(unsafe.Pointer(&mask[0])))
	if hbmMask == 0 {
		return 0
	}
	defer procDeleteObject.Call(hbmMask)

	info := IconInfo{FIcon: 1, HbmMask: hbmMask, HbmColor: hbm}
	hIcon, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&info)))
	return hIcon
}

// ------------------------------------------------------------------ Theming

// SetWindowTheme makes a control draw itself with the named visual style
// class, such as "DarkMode_Explorer". An empty name restores the default.
func SetWindowTheme(hwnd uintptr, name string) {
	if procSetWindowTheme.Find() != nil {
		return
	}
	var p *uint16
	if name != "" {
		p = UTF16(name)
	}
	procSetWindowTheme.Call(hwnd, uintptr(unsafe.Pointer(p)), 0)
}

// SetDarkTitleBar asks the window manager to draw the caption and frame in
// dark colours. Attribute 20 is DWMWA_USE_IMMERSIVE_DARK_MODE; Windows 10
// builds before 20H1 knew the same switch as attribute 19.
func SetDarkTitleBar(hwnd uintptr, dark bool) {
	if procDwmSetWindowAttribute.Find() != nil {
		return
	}
	value := int32(0)
	if dark {
		value = 1
	}
	for _, attr := range []uintptr{20, 19} {
		hr, _, _ := procDwmSetWindowAttribute.Call(hwnd, attr, uintptr(unsafe.Pointer(&value)), unsafe.Sizeof(value))
		if hr == 0 {
			return
		}
	}
}

// RedrawAll repaints the window, its frame and every child at once.
func RedrawAll(hwnd uintptr) {
	const (
		rdwInvalidate  = 0x0001
		rdwErase       = 0x0004
		rdwAllChildren = 0x0080
		rdwUpdateNow   = 0x0100
		rdwFrame       = 0x0400
	)
	procRedrawWindow.Call(hwnd, 0, 0, rdwInvalidate|rdwErase|rdwAllChildren|rdwUpdateNow|rdwFrame)
}

// SystemPrefersDark reports whether Windows is set to show applications in
// dark mode. It answers false when the setting cannot be read, which is the
// case on Windows versions that predate the switch.
func SystemPrefersDark() bool {
	const (
		hkeyCurrentUser = 0x80000001
		rrfRtRegDword   = 0x00000010
	)
	var value, size uint32 = 0, 4
	r, _, _ := procRegGetValueW.Call(hkeyCurrentUser,
		uintptr(unsafe.Pointer(UTF16(`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`))),
		uintptr(unsafe.Pointer(UTF16("AppsUseLightTheme"))),
		rrfRtRegDword, 0,
		uintptr(unsafe.Pointer(&value)), uintptr(unsafe.Pointer(&size)))
	return r == 0 && value == 0
}

// TextWidth measures how many physical pixels s takes in the given font.
func TextWidth(hwnd, font uintptr, s string) int32 {
	if s == "" {
		return 0
	}
	hdc, _, _ := procGetDC.Call(hwnd)
	if hdc == 0 {
		return 0
	}
	defer procReleaseDC.Call(hwnd, hdc)
	old := SelectObject(hdc, font)
	defer SelectObject(hdc, old)

	text, err := syscall.UTF16FromString(s)
	if err != nil || len(text) < 2 {
		return 0
	}
	var size struct{ CX, CY int32 }
	procGetTextExtentPoint32W.Call(hdc, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)-1),
		uintptr(unsafe.Pointer(&size)))
	return size.CX
}

// SubclassWindow replaces a window's procedure and returns the previous one,
// which the new procedure hands every message it does not handle itself.
func SubclassWindow(hwnd, proc uintptr) uintptr {
	const gwlpWndProc = ^uintptr(3) // GWLP_WNDPROC is -4
	old, _, _ := procSetWindowLongPtrW.Call(hwnd, gwlpWndProc, proc)
	return old
}

// CallWindowProc passes a message on to a window procedure saved by
// SubclassWindow.
func CallWindowProc(proc, hwnd, msg, wParam, lParam uintptr) uintptr {
	r, _, _ := procCallWindowProcW.Call(proc, hwnd, msg, wParam, lParam)
	return r
}

// Canvas is an off-screen 32-bit top-down bitmap with a device context, so
// that a control can be painted into memory, recoloured and then copied to
// the screen in one go.
type Canvas struct {
	DC     uintptr
	Pixels []byte // BGRA, Width*Height*4 bytes
	Width  int32
	Height int32
	bitmap uintptr
	old    uintptr
}

// NewCanvas creates a canvas compatible with hdc. It returns nil when GDI
// is out of resources; the caller then paints directly.
func NewCanvas(hdc uintptr, width, height int32) *Canvas {
	if width <= 0 || height <= 0 {
		return nil
	}
	dc, _, _ := procCreateCompatibleDC.Call(hdc)
	if dc == 0 {
		return nil
	}
	bi := BitmapInfoHeader{
		Size:     uint32(unsafe.Sizeof(BitmapInfoHeader{})),
		Width:    width,
		Height:   -height, // negative height means a top-down bitmap
		Planes:   1,
		BitCount: 32,
	}
	var bits unsafe.Pointer
	bmp, _, _ := procCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bi)), 0,
		uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 {
		procDeleteDC.Call(dc)
		return nil
	}
	return &Canvas{
		DC:     dc,
		Pixels: unsafe.Slice((*byte)(bits), int(width)*int(height)*4),
		Width:  width,
		Height: height,
		bitmap: bmp,
		old:    SelectObject(dc, bmp),
	}
}

// CopyTo copies the canvas onto hdc at the origin.
func (c *Canvas) CopyTo(hdc uintptr) {
	const srcCopy = 0x00CC0020
	procBitBlt.Call(hdc, 0, 0, uintptr(c.Width), uintptr(c.Height), c.DC, 0, 0, srcCopy)
}

// Release frees the bitmap and the device context.
func (c *Canvas) Release() {
	SelectObject(c.DC, c.old)
	DeleteObject(c.bitmap)
	procDeleteDC.Call(c.DC)
}
