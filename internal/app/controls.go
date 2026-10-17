//go:build windows

package app

import (
	"math"

	"mailhelper/internal/i18n"
	"mailhelper/internal/winapi"
)

// Control identifiers. Only the controls that report something need one.
const (
	idLang = 100 + iota
	idTheme
	idEmail
	idPassword
	idServer
	idPort
	idUser
	idTest
	idRefresh

	idDeleteSrc
	idDeleteFrom
	idDeleteTo
	idDeleteRun

	idCopySrc
	idMoveAfter
	idCopyDst
	idSaveDir
	idCopyFrom
	idCopyTo
	idCopyRun

	idMoveSrc
	idMoveDst
	idMoveFrom
	idMoveTo
	idMoveRun
)

// The layout is written entirely in logical pixels; applyDPI multiplies
// every one of them by the scale of the monitor the window is on.
const (
	winW = 940

	margin   = 12
	gapH     = 12
	gapV     = 12
	topY     = 10
	topRowH  = 34
	rowH     = 24
	rowPitch = 30
	labelW   = 168
	labelGap = 8

	groupInset  = 14
	groupTop    = 34 // room for the panel caption above the first row
	groupBottom = 14
	captionTop  = 8
	panelRadius = 10

	btnH      = 28
	btnPad    = 18 // space between a button's text and its edges
	btnMinW   = 90
	runGap    = 10 // between the last field and the progress row
	chkH      = 34
	chkPitch  = 38
	savePitch = 34
	dtW       = 170
	langW     = 150

	comboDrop = 220 // how far a combo box list may unfold

	groupW = (winW - 2*margin - gapH) / 2
	ctlOff = labelW + labelGap // where the input column starts inside a panel

	mailboxH = groupTop + 5*rowPitch + 4 + btnH + groupBottom
)

const (
	classStatic = "STATIC"
	classEdit   = "EDIT"
	classButton = "BUTTON"
	classCombo  = "COMBOBOX"
	classDate   = "SysDateTimePick32"
)

// panelID names the four panels the window is divided into.
type panelID int

const (
	pnlMailbox panelID = iota
	pnlDelete
	pnlCopy
	pnlMove
	panelCount
)

// panelRect is where a panel sits, in logical pixels.
type panelRect struct {
	x, y, w, h int32
}

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

func (a *App) checkbox(id int, extra uint32) uintptr {
	return a.child(classButton, winapi.WS_TABSTOP|winapi.BS_AUTOCHECKBOX|extra, id)
}

// dateFilter creates a checkbox that switches the filter on and the picker
// it enables. The picker has no checkbox of its own: the one DTS_SHOWNONE
// draws is stretched to the height of the control on Windows 11, while a
// regular checkbox looks the same as everywhere else.
func (a *App) dateFilter(id int) dateFilter {
	d := dateFilter{
		check:  a.checkbox(id, 0),
		picker: a.child(classDate, winapi.WS_TABSTOP|winapi.DTS_SHORTDATEFORMAT, 0),
	}
	subclassPicker(d.picker)
	return d
}

func (a *App) progressBar(danger bool) uintptr {
	h := a.child(progressClass, 0, 0)
	a.progress[h] = &progressState{danger: danger}
	return h
}

// createControls builds every control, in tab order.
func (a *App) createControls() {
	a.lblLang, a.cbLang = a.label(), a.combo(idLang)
	for i := range i18n.Count {
		winapi.ComboAdd(a.cbLang, i18n.Names[i])
	}
	winapi.ComboSetSel(a.cbLang, int(a.lang))
	a.lblTheme, a.cbTheme = a.label(), a.combo(idTheme)

	a.lblEmail, a.edEmail = a.label(), a.edit(idEmail, 0)
	a.lblPassword, a.edPassword = a.label(), a.edit(idPassword, winapi.ES_PASSWORD)
	a.lblUser, a.edUser = a.label(), a.edit(idUser, 0)
	a.lblServer, a.edServer = a.label(), a.edit(idServer, 0)
	a.lblPort, a.edPort = a.label(), a.edit(idPort, winapi.ES_NUMBER)
	a.btnTest, a.btnRefresh = a.button(idTest), a.button(idRefresh)

	a.lblDeleteSrc, a.cbDeleteSrc = a.label(), a.combo(idDeleteSrc)
	a.deleteFrom, a.deleteTo = a.dateFilter(idDeleteFrom), a.dateFilter(idDeleteTo)
	a.pbDelete = a.progressBar(true)
	a.btnDeleteRun = a.button(idDeleteRun)
	for _, h := range []uintptr{
		a.lblDeleteSrc, a.cbDeleteSrc,
		a.deleteFrom.check, a.deleteFrom.picker, a.deleteTo.check, a.deleteTo.picker,
		a.pbDelete, a.btnDeleteRun,
	} {
		a.inDanger[h] = true
	}

	a.lblCopySrc, a.cbCopySrc = a.label(), a.combo(idCopySrc)
	a.chkMoveAfter = a.checkbox(idMoveAfter, winapi.BS_MULTILINE)
	a.lblCopyDst, a.cbCopyDst = a.label(), a.combo(idCopyDst)
	a.btnSaveDir, a.lblSaveDir = a.button(idSaveDir), a.label()
	a.copyFrom, a.copyTo = a.dateFilter(idCopyFrom), a.dateFilter(idCopyTo)
	a.pbCopy = a.progressBar(false)
	a.btnCopyRun = a.button(idCopyRun)

	a.lblMoveSrc, a.cbMoveSrc = a.label(), a.combo(idMoveSrc)
	a.lblMoveDst, a.cbMoveDst = a.label(), a.combo(idMoveDst)
	a.moveFrom, a.moveTo = a.dateFilter(idMoveFrom), a.dateFilter(idMoveTo)
	a.pbMove = a.progressBar(false)
	a.btnMoveRun = a.button(idMoveRun)

	a.updateEnabled()
}

func (a *App) dateFilters() []dateFilter {
	return []dateFilter{a.deleteFrom, a.deleteTo, a.copyFrom, a.copyTo, a.moveFrom, a.moveTo}
}

// interactive lists the controls that are switched off while a job runs.
// Labels and the progress bars stay as they are: they carry no input, and
// greying them out would only make the window harder to read.
func (a *App) interactive() []uintptr {
	out := []uintptr{
		a.cbLang, a.cbTheme,
		a.edEmail, a.edPassword, a.edUser, a.edServer, a.edPort,
		a.btnTest, a.btnRefresh,
		a.cbDeleteSrc, a.btnDeleteRun,
		a.cbCopySrc, a.chkMoveAfter, a.cbCopyDst, a.btnSaveDir, a.btnCopyRun,
		a.cbMoveSrc, a.cbMoveDst, a.btnMoveRun,
	}
	for _, d := range a.dateFilters() {
		out = append(out, d.check, d.picker)
	}
	return out
}

// buttons lists the push buttons and checkboxes, which take the dark visual
// style of buttons.
func (a *App) buttons() []uintptr {
	out := []uintptr{
		a.btnTest, a.btnRefresh, a.btnDeleteRun,
		a.chkMoveAfter, a.btnSaveDir, a.btnCopyRun, a.btnMoveRun,
	}
	for _, d := range a.dateFilters() {
		out = append(out, d.check)
	}
	return out
}

// fields lists the edit and combo boxes, which take the dark visual style
// of input fields. The date pickers have none; see datepicker.go.
func (a *App) fields() []uintptr {
	return []uintptr{
		a.cbLang, a.cbTheme,
		a.edEmail, a.edPassword, a.edUser, a.edServer, a.edPort,
		a.cbDeleteSrc, a.cbCopySrc, a.cbCopyDst, a.cbMoveSrc, a.cbMoveDst,
	}
}

// all lists every control, which is what the font has to be applied to.
func (a *App) all() []uintptr {
	out := append(a.buttons(), a.fields()...)
	for _, d := range a.dateFilters() {
		out = append(out, d.picker)
	}
	return append(out,
		a.lblLang, a.lblTheme,
		a.lblEmail, a.lblPassword, a.lblUser, a.lblServer, a.lblPort,
		a.lblDeleteSrc, a.lblCopySrc, a.lblCopyDst, a.lblSaveDir,
		a.lblMoveSrc, a.lblMoveDst,
		a.pbDelete, a.pbCopy, a.pbMove,
	)
}

// ------------------------------------------------------------------ Layout

func (a *App) px(v int32) int32 { return int32(float64(v)*a.scale + 0.5) }

func (a *App) place(hwnd uintptr, x, y, w, h int32) {
	if hwnd == 0 {
		return
	}
	winapi.MoveWindow(hwnd, a.px(x), a.px(y), a.px(w), a.px(h), true)
}

// textWidth is the logical width of s in the UI font.
func (a *App) textWidth(s string) int32 {
	return int32(math.Ceil(float64(winapi.TextWidth(a.hwnd, a.font, s)) / a.scale))
}

// buttonWidth sizes a push button to its caption, so that it is no wider
// than its text needs. Several captions can be given for a button whose text
// changes, such as Start/Stop; the widest one wins, so the button does not
// jump when its caption flips.
func (a *App) buttonWidth(captions ...string) int32 {
	w := int32(0)
	for _, c := range captions {
		w = max(w, a.textWidth(c))
	}
	return max(w+2*btnPad, btnMinW)
}

// show hides or shows a control without disturbing the rest of the layout.
func show(hwnd uintptr, visible bool) {
	if visible {
		winapi.ShowWindow(hwnd, winapi.SW_SHOW)
	} else {
		winapi.ShowWindow(hwnd, winapi.SW_HIDE)
	}
}

// applyDPI lays the whole window out for the given DPI: it rebuilds the fonts
// at the new size, resizes the window and repositions every control. On a
// WM_DPICHANGED the suggested rectangle carries the position Windows wants
// the window moved to; at start-up it is nil and the window stays put.
func (a *App) applyDPI(dpi uint32, suggested *winapi.Rect) {
	if dpi == 0 {
		dpi = 96
	}
	a.dpi = dpi
	a.scale = float64(dpi) / 96

	for _, f := range []uintptr{a.font, a.fontBold} {
		if f != 0 {
			winapi.DeleteObject(f)
		}
	}
	// A negative height asks for a character height, which is how a 9pt UI
	// font maps onto whatever DPI this monitor has.
	height := -(9 * int32(dpi)) / 72
	a.font = winapi.CreateFont(height, 400)
	a.fontBold = winapi.CreateFont(height, 600)
	for _, h := range a.all() {
		winapi.SendMessage(h, winapi.WM_SETFONT, a.font, 1)
	}

	a.resize(suggested)
	a.setWindowIcons()
}

// relayout re-runs the layout after something changed its shape: the
// checkbox that reveals the target folder combo, or a language whose
// captions are wider or narrower.
func (a *App) relayout() {
	a.resize(nil)
}

// resize fits the window around the content and lays the controls out.
func (a *App) resize(suggested *winapi.Rect) {
	client := winapi.Rect{Right: a.px(winW), Bottom: a.px(a.contentHeight())}
	frame := winapi.AdjustWindowRect(client, mainStyle(), winapi.WS_EX_CONTROLPARENT, a.dpi)

	flags := uint32(winapi.SWP_NOZORDER | winapi.SWP_NOACTIVATE)
	var x, y int32
	if suggested != nil {
		x, y = suggested.Left, suggested.Top
	} else {
		flags |= winapi.SWP_NOMOVE
	}
	winapi.SetWindowPos(a.hwnd, 0, x, y, frame.Width(), frame.Height(), flags)

	a.layoutAll()

	// Erase and redraw everything at once: the controls have just moved, and
	// the area they came from has to go back to the window background.
	winapi.InvalidateRect(a.hwnd, true)
	winapi.UpdateWindow(a.hwnd)
}

// taskGroupHeight is the height both lower panels get, so they line up
// however many rows each of them currently needs.
func (a *App) taskGroupHeight() int32 {
	copyH := int32(groupTop + rowPitch + chkPitch + savePitch + 2*rowPitch + runGap + btnH + groupBottom)
	if a.moveAfterCopy() {
		copyH += rowPitch
	}
	moveH := int32(groupTop + 4*rowPitch + runGap + btnH + groupBottom)
	return max(copyH, moveH)
}

func (a *App) contentHeight() int32 {
	return topY + topRowH + mailboxH + gapV + a.taskGroupHeight() + gapV
}

func (a *App) layoutAll() {
	y := int32(topY)

	// Language and theme, side by side.
	x := int32(margin + 2)
	w := a.textWidth(a.tr.Language) + labelGap
	a.place(a.lblLang, x, y, w, rowH)
	x += w
	a.place(a.cbLang, x, y, langW, rowH+comboDrop)
	x += langW + 2*gapH
	w = a.textWidth(a.tr.Theme) + labelGap
	a.place(a.lblTheme, x, y, w, rowH)
	x += w
	a.place(a.cbTheme, x, y, langW, rowH+comboDrop)
	y += topRowH

	right := int32(margin + groupW + gapH)
	a.panels[pnlMailbox] = panelRect{margin, y, groupW, mailboxH}
	a.panels[pnlDelete] = panelRect{right, y, groupW, mailboxH}
	a.layoutMailbox(a.panels[pnlMailbox])
	a.layoutDelete(a.panels[pnlDelete])
	y += mailboxH + gapV

	h := a.taskGroupHeight()
	a.panels[pnlCopy] = panelRect{margin, y, groupW, h}
	a.panels[pnlMove] = panelRect{right, y, groupW, h}
	a.layoutCopy(a.panels[pnlCopy])
	a.layoutMove(a.panels[pnlMove])
}

// inner returns the left edge, the input column and the right edge of a
// panel's content area.
func (p panelRect) inner() (left, ctlX, right int32) {
	left = p.x + groupInset
	return left, left + ctlOff, p.x + p.w - groupInset
}

// field places a label and its input on one row.
func (a *App) field(p panelRect, lbl, ctl uintptr, y, w int32) {
	left, ctlX, right := p.inner()
	if w <= 0 {
		w = right - ctlX
	}
	a.place(lbl, left, y, labelW, rowH)
	a.place(ctl, ctlX, y, w, rowH)
}

// comboField is field for a combo box, whose height includes its list.
func (a *App) comboField(p panelRect, lbl, combo uintptr, y int32) {
	left, ctlX, right := p.inner()
	a.place(lbl, left, y, labelW, rowH)
	a.place(combo, ctlX, y, right-ctlX, rowH+comboDrop)
}

func (a *App) dateRow(p panelRect, d dateFilter, y int32) {
	left, ctlX, _ := p.inner()
	a.place(d.check, left, y, labelW, rowH)
	a.place(d.picker, ctlX, y, dtW, rowH)
}

// runRow puts a progress bar and, after it on the same line, the button that
// starts the job, sized to its caption. The row sits at the bottom of the
// panel, so the bars of neighbouring panels line up.
func (a *App) runRow(p panelRect, bar, btn uintptr) {
	left, _, right := p.inner()
	y := p.y + p.h - groupBottom - btnH
	w := a.buttonWidth(a.tr.Start, a.tr.Stop)
	a.place(btn, right-w, y, w, btnH)
	a.place(bar, left, y+1, right-w-gapH-left, btnH-2)
}

func (a *App) layoutMailbox(p panelRect) {
	left, ctlX, right := p.inner()
	row := func(n int32) int32 { return p.y + groupTop + n*rowPitch }

	a.field(p, a.lblEmail, a.edEmail, row(0), 0)
	a.field(p, a.lblPassword, a.edPassword, row(1), 0)
	a.field(p, a.lblUser, a.edUser, row(2), 0)
	a.field(p, a.lblServer, a.edServer, row(3), 0)
	a.field(p, a.lblPort, a.edPort, row(4), 90)

	// The second button lines up with the input fields above it.
	by := row(5) + 4
	a.place(a.btnTest, left, by, labelW, btnH)
	a.place(a.btnRefresh, ctlX, by, right-ctlX, btnH)
}

func (a *App) layoutDelete(p panelRect) {
	y := p.y + groupTop
	a.comboField(p, a.lblDeleteSrc, a.cbDeleteSrc, y)
	y += rowPitch
	a.dateRow(p, a.deleteFrom, y)
	y += rowPitch
	a.dateRow(p, a.deleteTo, y)

	a.runRow(p, a.pbDelete, a.btnDeleteRun)
}

func (a *App) layoutCopy(p panelRect) {
	left, _, right := p.inner()
	y := p.y + groupTop

	a.comboField(p, a.lblCopySrc, a.cbCopySrc, y)
	y += rowPitch

	a.place(a.chkMoveAfter, left, y, right-left, chkH)
	y += chkPitch

	moveAfter := a.moveAfterCopy()
	show(a.lblCopyDst, moveAfter)
	show(a.cbCopyDst, moveAfter)
	if moveAfter {
		a.comboField(p, a.lblCopyDst, a.cbCopyDst, y)
		y += rowPitch
	}

	w := a.buttonWidth(a.tr.ChooseSaveDir)
	a.place(a.btnSaveDir, left, y, w, btnH)
	a.place(a.lblSaveDir, left+w+10, y+2, right-left-w-10, rowH)
	y += savePitch

	a.dateRow(p, a.copyFrom, y)
	y += rowPitch
	a.dateRow(p, a.copyTo, y)

	a.runRow(p, a.pbCopy, a.btnCopyRun)
}

func (a *App) layoutMove(p panelRect) {
	y := p.y + groupTop

	a.comboField(p, a.lblMoveSrc, a.cbMoveSrc, y)
	y += rowPitch
	a.comboField(p, a.lblMoveDst, a.cbMoveDst, y)
	y += rowPitch
	a.dateRow(p, a.moveFrom, y)
	y += rowPitch
	a.dateRow(p, a.moveTo, y)

	a.runRow(p, a.pbMove, a.btnMoveRun)
}

// ------------------------------------------------------------------ Panels

func (a *App) panelCaption(id panelID) string {
	switch id {
	case pnlMailbox:
		return a.tr.GroupMailbox
	case pnlDelete:
		return a.tr.GroupDelete
	case pnlCopy:
		return a.tr.GroupCopy
	case pnlMove:
		return a.tr.GroupMove
	}
	return ""
}

// paintPanels draws the rounded frames the controls are grouped in, with
// their captions. The danger panel is filled in its warning colour; the
// controls on it pick the same colour up through WM_CTLCOLOR*.
func (a *App) paintPanels(hdc uintptr) {
	winapi.SetBkMode(hdc, winapi.TRANSPARENT)
	oldFont := winapi.SelectObject(hdc, a.fontBold)
	defer winapi.SelectObject(hdc, oldFont)

	for id := range panelCount {
		p := a.panels[id]
		fill, edge, text := a.pal.window, a.pal.panelEdge, a.pal.text
		if id == pnlDelete {
			fill, edge, text = a.pal.danger, a.pal.dangerEdge, a.pal.dangerText
		}

		frame := winapi.Rect{Left: a.px(p.x), Top: a.px(p.y), Right: a.px(p.x + p.w), Bottom: a.px(p.y + p.h)}
		fillRounded(hdc, frame, a.px(panelRadius), fill, edge)

		caption := winapi.Rect{
			Left:   a.px(p.x + groupInset),
			Top:    a.px(p.y + captionTop),
			Right:  a.px(p.x + p.w - groupInset),
			Bottom: a.px(p.y + groupTop - 4),
		}
		winapi.SetTextColor(hdc, text)
		winapi.DrawText(hdc, a.panelCaption(id), &caption,
			winapi.DT_LEFT|winapi.DT_VCENTER|winapi.DT_SINGLELINE|winapi.DT_END_ELLIPSIS)
	}
}

// ------------------------------------------------------------- Translation

// retranslate writes every caption in the current language and lays the
// window out again, since buttons are as wide as their captions.
func (a *App) retranslate() {
	tr := a.tr
	winapi.SetWindowText(a.hwnd, tr.AppTitle)

	winapi.SetWindowText(a.lblLang, tr.Language)
	winapi.SetWindowText(a.lblTheme, tr.Theme)
	winapi.ComboReset(a.cbTheme)
	for _, name := range [themeCount]string{themeLight: tr.ThemeLight, themeDark: tr.ThemeDark} {
		winapi.ComboAdd(a.cbTheme, name)
	}
	winapi.ComboSetSel(a.cbTheme, int(a.theme))

	winapi.SetWindowText(a.lblEmail, tr.Email)
	winapi.SetWindowText(a.lblPassword, tr.AppPassword)
	winapi.SetWindowText(a.lblUser, tr.Username)
	winapi.SetWindowText(a.lblServer, tr.ImapServer)
	winapi.SetWindowText(a.lblPort, tr.ImapPort)
	winapi.SetWindowText(a.btnTest, tr.TestConn)
	winapi.SetWindowText(a.btnRefresh, tr.RefreshList)

	winapi.SetWindowText(a.lblDeleteSrc, tr.TargetFolder)

	winapi.SetWindowText(a.lblCopySrc, tr.SourceFolder)
	winapi.SetWindowText(a.chkMoveAfter, tr.MoveAfterCopy)
	winapi.SetWindowText(a.lblCopyDst, tr.TargetFolder)
	winapi.SetWindowText(a.btnSaveDir, tr.ChooseSaveDir)

	winapi.SetWindowText(a.lblMoveSrc, tr.SourceFolder)
	winapi.SetWindowText(a.lblMoveDst, tr.TargetFolder)

	for _, pair := range [][2]dateFilter{
		{a.deleteFrom, a.deleteTo}, {a.copyFrom, a.copyTo}, {a.moveFrom, a.moveTo},
	} {
		winapi.SetWindowText(pair[0].check, tr.DateFrom)
		winapi.SetWindowText(pair[1].check, tr.DateTo)
	}

	a.relabelFolderCombos()
	a.updateSaveDirLabel()
	a.updateRunButtons()
	a.relayout()
}

func (a *App) updateSaveDirLabel() {
	if a.saveDir == "" {
		winapi.SetWindowText(a.lblSaveDir, a.tr.NoSaveDir)
		return
	}
	winapi.SetWindowText(a.lblSaveDir, a.saveDir)
}

// updateRunButtons flips each Start button to Stop while its job runs.
func (a *App) updateRunButtons() {
	for _, b := range []struct {
		btn uintptr
		t   task
	}{
		{a.btnDeleteRun, taskDelete},
		{a.btnCopyRun, taskCopy},
		{a.btnMoveRun, taskMove},
	} {
		text := a.tr.Start
		if a.running == b.t {
			text = a.tr.Stop
		}
		winapi.SetWindowText(b.btn, text)
	}
}

func (a *App) moveAfterCopy() bool {
	return a.chkMoveAfter != 0 && winapi.Checked(a.chkMoveAfter)
}
