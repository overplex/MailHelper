//go:build windows

package app

import (
	"mailhelper/internal/i18n"
	"mailhelper/internal/winapi"
)

// Control identifiers. Only the interactive controls need one.
const (
	idLang = 100 + iota
	idEmail
	idPassword
	idServer
	idPort
	idUser
	idTest
	idRefresh

	idCopySrc
	idMoveAfter
	idCopyDst
	idSaveDir
	idCopyFrom
	idCopyFromX
	idCopyTo
	idCopyToX
	idCopyRun

	idMoveSrc
	idMoveDst
	idMoveFrom
	idMoveFromX
	idMoveTo
	idMoveToX
	idMoveRun
)

// The layout is written entirely in logical pixels; applyDPI multiplies
// every one of them by the scale of the monitor the window is on.
const (
	winW = 940

	margin   = 12
	gapH     = 12
	gapV     = 12
	rowH     = 24
	rowPitch = 30
	labelW   = 168
	labelGap = 8

	groupInset  = 14
	groupTop    = 24
	groupBottom = 14

	btnH      = 28
	runPitch  = 38
	chkH      = 34
	chkPitch  = 38
	savePitch = 34
	pbH       = 26
	dtW       = 170
	clearW    = 28
	clearGap  = 6

	comboDrop = 220 // how far a combo box list may unfold

	groupW = (winW - 2*margin - gapH) / 2
)

const (
	classStatic = "STATIC"
	classEdit   = "EDIT"
	classButton = "BUTTON"
	classCombo  = "COMBOBOX"
	classDate   = "SysDateTimePick32"
)

// child creates one control. Position and size are set later by layoutAll.
func (a *App) child(class string, style uint32, id int) uintptr {
	h := winapi.CreateWindowEx(0, class, "",
		winapi.WS_CHILD|winapi.WS_VISIBLE|style,
		0, 0, 0, 0, a.hwnd, uintptr(id), a.hInstance)
	if h == 0 {
		panic("CreateWindowEx(" + class + ") failed")
	}
	return h
}

func (a *App) label() uintptr {
	return a.child(classStatic, winapi.SS_LEFT|winapi.SS_CENTERIMAGE, 0)
}

func (a *App) group() uintptr {
	return a.child(classButton, winapi.BS_GROUPBOX, 0)
}

func (a *App) edit(id int, extra uint32) uintptr {
	return a.child(classEdit,
		winapi.WS_TABSTOP|winapi.WS_BORDER|winapi.ES_LEFT|winapi.ES_AUTOHSCROLL|extra, id)
}

func (a *App) combo(id int) uintptr {
	return a.child(classCombo,
		winapi.WS_TABSTOP|winapi.WS_VSCROLL|winapi.CBS_DROPDOWNLIST|winapi.CBS_HASSTRINGS, id)
}

func (a *App) button(id int) uintptr {
	return a.child(classButton, winapi.WS_TABSTOP|winapi.BS_PUSHBUTTON, id)
}

func (a *App) datePicker(id int) uintptr {
	// DTS_SHOWNONE gives the control its own "date set / not set" checkbox,
	// so an empty date is a real state rather than a blank the user has to
	// guess at. The X button next to it clears the date in one click.
	h := a.child(classDate,
		winapi.WS_TABSTOP|winapi.DTS_SHORTDATEFORMAT|winapi.DTS_SHOWNONE, id)
	winapi.DateSetNone(h)
	return h
}

func (a *App) progressBar() uintptr {
	h := a.child(progressClass, 0, 0)
	a.progress[h] = &progressState{}
	return h
}

// createControls builds every control. Group boxes come first so that they
// sit behind the fields they enclose in the sibling z-order.
func (a *App) createControls() {
	a.grpMailbox = a.group()
	a.grpCopy = a.group()
	a.grpMove = a.group()

	a.lblLang = a.label()
	a.cbLang = a.combo(idLang)
	for i := range i18n.Count {
		winapi.ComboAdd(a.cbLang, i18n.Names[i])
	}
	winapi.ComboSetSel(a.cbLang, int(a.lang))

	a.lblEmail, a.edEmail = a.label(), a.edit(idEmail, 0)
	a.lblPassword, a.edPassword = a.label(), a.edit(idPassword, winapi.ES_PASSWORD)
	a.lblServer, a.edServer = a.label(), a.edit(idServer, 0)
	a.lblPort, a.edPort = a.label(), a.edit(idPort, winapi.ES_NUMBER)
	a.lblUser, a.edUser = a.label(), a.edit(idUser, 0)
	a.btnTest, a.btnRefresh = a.button(idTest), a.button(idRefresh)

	a.suppressEdit = true
	winapi.SetWindowText(a.edPort, "993")
	a.suppressEdit = false

	a.lblCopySrc, a.cbCopySrc = a.label(), a.combo(idCopySrc)
	a.chkMoveAfter = a.child(classButton,
		winapi.WS_TABSTOP|winapi.BS_AUTOCHECKBOX|winapi.BS_MULTILINE, idMoveAfter)
	a.lblCopyDst, a.cbCopyDst = a.label(), a.combo(idCopyDst)
	a.btnSaveDir, a.lblSaveDir = a.button(idSaveDir), a.label()
	a.lblCopyFrom, a.dtCopyFrom, a.btnCopyFromX = a.label(), a.datePicker(idCopyFrom), a.button(idCopyFromX)
	a.lblCopyTo, a.dtCopyTo, a.btnCopyToX = a.label(), a.datePicker(idCopyTo), a.button(idCopyToX)
	a.btnCopyRun = a.button(idCopyRun)
	a.pbCopy = a.progressBar()

	a.lblMoveSrc, a.cbMoveSrc = a.label(), a.combo(idMoveSrc)
	a.lblMoveDst, a.cbMoveDst = a.label(), a.combo(idMoveDst)
	a.lblMoveFrom, a.dtMoveFrom, a.btnMoveFromX = a.label(), a.datePicker(idMoveFrom), a.button(idMoveFromX)
	a.lblMoveTo, a.dtMoveTo, a.btnMoveToX = a.label(), a.datePicker(idMoveTo), a.button(idMoveToX)
	a.btnMoveRun = a.button(idMoveRun)
	a.pbMove = a.progressBar()
}

// interactive lists the controls that are switched off while a job runs.
// Labels, group boxes and the progress bars stay as they are: they carry no
// input, and greying them out would only make the window harder to read.
func (a *App) interactive() []uintptr {
	return []uintptr{
		a.cbLang,
		a.edEmail, a.edPassword, a.edServer, a.edPort, a.edUser,
		a.btnTest, a.btnRefresh,
		a.cbCopySrc, a.chkMoveAfter, a.cbCopyDst, a.btnSaveDir,
		a.dtCopyFrom, a.btnCopyFromX, a.dtCopyTo, a.btnCopyToX, a.btnCopyRun,
		a.cbMoveSrc, a.cbMoveDst,
		a.dtMoveFrom, a.btnMoveFromX, a.dtMoveTo, a.btnMoveToX, a.btnMoveRun,
	}
}

// all lists every control, which is what the font has to be applied to.
func (a *App) all() []uintptr {
	return []uintptr{
		a.grpMailbox, a.grpCopy, a.grpMove,
		a.lblLang, a.cbLang,
		a.lblEmail, a.edEmail, a.lblPassword, a.edPassword,
		a.lblServer, a.edServer, a.lblPort, a.edPort,
		a.lblUser, a.edUser, a.btnTest, a.btnRefresh,
		a.lblCopySrc, a.cbCopySrc, a.chkMoveAfter, a.lblCopyDst, a.cbCopyDst,
		a.btnSaveDir, a.lblSaveDir,
		a.lblCopyFrom, a.dtCopyFrom, a.btnCopyFromX,
		a.lblCopyTo, a.dtCopyTo, a.btnCopyToX,
		a.btnCopyRun, a.pbCopy,
		a.lblMoveSrc, a.cbMoveSrc, a.lblMoveDst, a.cbMoveDst,
		a.lblMoveFrom, a.dtMoveFrom, a.btnMoveFromX,
		a.lblMoveTo, a.dtMoveTo, a.btnMoveToX,
		a.btnMoveRun, a.pbMove,
	}
}

// ------------------------------------------------------------------ Layout

func (a *App) px(v int32) int32 { return int32(float64(v)*a.scale + 0.5) }

func (a *App) place(hwnd uintptr, x, y, w, h int32) {
	if hwnd == 0 {
		return
	}
	winapi.MoveWindow(hwnd, a.px(x), a.px(y), a.px(w), a.px(h), true)
}

// show hides or shows a control without disturbing the rest of the layout.
func show(hwnd uintptr, visible bool) {
	if visible {
		winapi.ShowWindow(hwnd, winapi.SW_SHOW)
	} else {
		winapi.ShowWindow(hwnd, winapi.SW_HIDE)
	}
}

// applyDPI lays the whole window out for the given DPI: it rebuilds the font
// at the new size, resizes the window and repositions every control. On a
// WM_DPICHANGED the suggested rectangle carries the position Windows wants
// the window moved to; at start-up it is nil and the window stays put.
func (a *App) applyDPI(dpi uint32, suggested *winapi.Rect) {
	if dpi == 0 {
		dpi = 96
	}
	a.dpi = dpi
	a.scale = float64(dpi) / 96

	if a.font != 0 {
		winapi.DeleteObject(a.font)
	}
	// A negative height asks for a character height, which is how a 9pt UI
	// font maps onto whatever DPI this monitor has.
	a.font = winapi.CreateFont(-(9*int32(dpi))/72, 400)
	for _, h := range a.all() {
		winapi.SendMessage(h, winapi.WM_SETFONT, a.font, 1)
	}

	client := winapi.Rect{Right: a.px(winW), Bottom: a.px(a.contentHeight())}
	frame := winapi.AdjustWindowRect(client, mainStyle(), winapi.WS_EX_CONTROLPARENT, dpi)

	flags := uint32(winapi.SWP_NOZORDER | winapi.SWP_NOACTIVATE)
	var x, y int32
	if suggested != nil {
		x, y = suggested.Left, suggested.Top
	} else {
		flags |= winapi.SWP_NOMOVE
	}
	winapi.SetWindowPos(a.hwnd, 0, x, y, frame.Width(), frame.Height(), flags)

	a.layoutAll()
	a.setWindowIcons()

	// Erase and redraw everything at once: the controls have just moved, and
	// the area they came from has to go back to the window background.
	winapi.InvalidateRect(a.hwnd, true)
	winapi.UpdateWindow(a.hwnd)
}

// relayout re-runs the layout after something changed its shape - the only
// such thing being the checkbox that reveals the target folder combo.
func (a *App) relayout() {
	a.applyDPI(a.dpi, nil)
}

// taskGroupHeight is the height both task group boxes get, so they line up
// however many rows each of them currently needs.
func (a *App) taskGroupHeight() int32 {
	copyH := int32(groupTop+rowPitch) + chkPitch + savePitch + 2*rowPitch + runPitch + pbH + groupBottom
	if a.moveAfterCopy() {
		copyH += rowPitch
	}
	moveH := int32(groupTop+4*rowPitch) + runPitch + pbH + groupBottom
	if moveH > copyH {
		return moveH
	}
	return copyH
}

const mailboxH = groupTop + 3*rowPitch + 8 + btnH + groupBottom

func (a *App) contentHeight() int32 {
	return 10 + 34 + mailboxH + gapV + a.taskGroupHeight() + gapV
}

func (a *App) layoutAll() {
	y := int32(10)

	// Language row.
	a.place(a.lblLang, margin+2, y+2, 110, rowH)
	a.place(a.cbLang, margin+120, y, 200, rowH+comboDrop)
	y += 34

	a.layoutMailbox(margin, y, winW-2*margin)
	y += mailboxH + gapV

	h := a.taskGroupHeight()
	a.layoutCopy(margin, y, groupW, h)
	a.layoutMove(margin+groupW+gapH, y, groupW, h)
}

func (a *App) layoutMailbox(gx, gy, gw int32) {
	a.place(a.grpMailbox, gx, gy, gw, mailboxH)

	innerL := gx + groupInset
	innerW := gw - 2*groupInset
	colW := (innerW - 24) / 2
	colB := innerL + colW + 24
	ctlW := colW - labelW - labelGap

	row := func(n int32) int32 { return gy + groupTop + n*rowPitch }
	field := func(lbl, ctl uintptr, x, y, w int32) {
		a.place(lbl, x, y+2, labelW, rowH)
		a.place(ctl, x+labelW+labelGap, y, w, rowH)
	}

	field(a.lblEmail, a.edEmail, innerL, row(0), ctlW)
	field(a.lblPassword, a.edPassword, innerL, row(1), ctlW)
	field(a.lblUser, a.edUser, innerL, row(2), ctlW)

	field(a.lblServer, a.edServer, colB, row(0), ctlW)
	field(a.lblPort, a.edPort, colB, row(1), 90)

	by := row(3) + 8
	a.place(a.btnTest, innerL, by, 210, btnH)
	a.place(a.btnRefresh, innerL+220, by, 230, btnH)
}

func (a *App) layoutCopy(gx, gy, gw, gh int32) {
	a.place(a.grpCopy, gx, gy, gw, gh)

	innerL := gx + groupInset
	innerW := gw - 2*groupInset
	ctlX := innerL + labelW + labelGap
	ctlW := innerL + innerW - ctlX

	y := gy + groupTop

	a.place(a.lblCopySrc, innerL, y+2, labelW, rowH)
	a.place(a.cbCopySrc, ctlX, y, ctlW, rowH+comboDrop)
	y += rowPitch

	a.place(a.chkMoveAfter, innerL, y, innerW, chkH)
	y += chkPitch

	moveAfter := a.moveAfterCopy()
	show(a.lblCopyDst, moveAfter)
	show(a.cbCopyDst, moveAfter)
	if moveAfter {
		a.place(a.lblCopyDst, innerL, y+2, labelW, rowH)
		a.place(a.cbCopyDst, ctlX, y, ctlW, rowH+comboDrop)
		y += rowPitch
	}

	a.place(a.btnSaveDir, innerL, y, 220, btnH)
	a.place(a.lblSaveDir, innerL+230, y+3, innerW-230, rowH)
	y += savePitch

	a.dateRow(a.lblCopyFrom, a.dtCopyFrom, a.btnCopyFromX, innerL, y, ctlX)
	y += rowPitch
	a.dateRow(a.lblCopyTo, a.dtCopyTo, a.btnCopyToX, innerL, y, ctlX)
	y += rowPitch

	a.place(a.btnCopyRun, innerL, y, 220, btnH+2)

	a.place(a.pbCopy, innerL, gy+gh-groupBottom-pbH, innerW, pbH)
}

func (a *App) layoutMove(gx, gy, gw, gh int32) {
	a.place(a.grpMove, gx, gy, gw, gh)

	innerL := gx + groupInset
	innerW := gw - 2*groupInset
	ctlX := innerL + labelW + labelGap
	ctlW := innerL + innerW - ctlX

	y := gy + groupTop

	a.place(a.lblMoveSrc, innerL, y+2, labelW, rowH)
	a.place(a.cbMoveSrc, ctlX, y, ctlW, rowH+comboDrop)
	y += rowPitch

	a.place(a.lblMoveDst, innerL, y+2, labelW, rowH)
	a.place(a.cbMoveDst, ctlX, y, ctlW, rowH+comboDrop)
	y += rowPitch

	a.dateRow(a.lblMoveFrom, a.dtMoveFrom, a.btnMoveFromX, innerL, y, ctlX)
	y += rowPitch
	a.dateRow(a.lblMoveTo, a.dtMoveTo, a.btnMoveToX, innerL, y, ctlX)
	y += rowPitch

	a.place(a.btnMoveRun, innerL, y, 220, btnH+2)

	a.place(a.pbMove, innerL, gy+gh-groupBottom-pbH, innerW, pbH)
}

func (a *App) dateRow(lbl, picker, clearBtn uintptr, x, y, ctlX int32) {
	a.place(lbl, x, y+2, labelW, rowH)
	a.place(picker, ctlX, y, dtW, rowH)
	a.place(clearBtn, ctlX+dtW+clearGap, y, clearW, rowH)
}

// ------------------------------------------------------------- Translation

// retranslate writes every caption in the current language. The layout is
// fixed, so switching languages never moves a control.
func (a *App) retranslate() {
	tr := a.tr
	winapi.SetWindowText(a.hwnd, tr.AppTitle)

	winapi.SetWindowText(a.lblLang, tr.Language)

	winapi.SetWindowText(a.grpMailbox, tr.GroupMailbox)
	winapi.SetWindowText(a.lblEmail, tr.Email)
	winapi.SetWindowText(a.lblPassword, tr.AppPassword)
	winapi.SetWindowText(a.lblServer, tr.ImapServer)
	winapi.SetWindowText(a.lblPort, tr.ImapPort)
	winapi.SetWindowText(a.lblUser, tr.Username)
	winapi.SetWindowText(a.btnTest, tr.TestConn)
	winapi.SetWindowText(a.btnRefresh, tr.RefreshList)

	winapi.SetWindowText(a.grpCopy, tr.GroupCopy)
	winapi.SetWindowText(a.lblCopySrc, tr.SourceFolder)
	winapi.SetWindowText(a.chkMoveAfter, tr.MoveAfterCopy)
	winapi.SetWindowText(a.lblCopyDst, tr.TargetFolder)
	winapi.SetWindowText(a.btnSaveDir, tr.ChooseSaveDir)
	winapi.SetWindowText(a.lblCopyFrom, tr.DateFrom)
	winapi.SetWindowText(a.lblCopyTo, tr.DateTo)

	winapi.SetWindowText(a.grpMove, tr.GroupMove)
	winapi.SetWindowText(a.lblMoveSrc, tr.SourceFolder)
	winapi.SetWindowText(a.lblMoveDst, tr.TargetFolder)
	winapi.SetWindowText(a.lblMoveFrom, tr.DateFrom)
	winapi.SetWindowText(a.lblMoveTo, tr.DateTo)

	for _, h := range []uintptr{a.btnCopyFromX, a.btnCopyToX, a.btnMoveFromX, a.btnMoveToX} {
		winapi.SetWindowText(h, "✕")
	}

	a.relabelFolderCombos()
	a.updateSaveDirLabel()
	a.updateRunButtons()
}

func (a *App) updateSaveDirLabel() {
	if a.saveDir == "" {
		winapi.SetWindowText(a.lblSaveDir, a.tr.NoSaveDir)
		return
	}
	winapi.SetWindowText(a.lblSaveDir, a.saveDir)
}

// updateRunButtons flips the two Start buttons to Stop while their job runs.
func (a *App) updateRunButtons() {
	copyText, moveText := a.tr.Start, a.tr.Start
	switch a.running {
	case taskCopy:
		copyText = a.tr.Stop
	case taskMove:
		moveText = a.tr.Stop
	}
	winapi.SetWindowText(a.btnCopyRun, copyText)
	winapi.SetWindowText(a.btnMoveRun, moveText)
}

func (a *App) moveAfterCopy() bool {
	return a.chkMoveAfter != 0 && winapi.Checked(a.chkMoveAfter)
}
