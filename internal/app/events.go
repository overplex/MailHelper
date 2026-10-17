//go:build windows

package app

import (
	"strconv"
	"strings"
	"time"

	"mailhelper/internal/i18n"
	"mailhelper/internal/imapc"
	"mailhelper/internal/winapi"
)

// onCommand routes WM_COMMAND from every control.
func (a *App) onCommand(id, notify uint16) {
	switch int(id) {
	case idLang:
		if notify == winapi.CBN_SELCHANGE {
			a.setLanguage(i18n.Lang(winapi.ComboGetSel(a.cbLang)))
		}
	case idTheme:
		if notify == winapi.CBN_SELCHANGE {
			if t := theme(winapi.ComboGetSel(a.cbTheme)); t != a.theme {
				a.setTheme(t)
			}
		}

	case idEmail:
		if notify == winapi.EN_CHANGE && !a.suppressEdit {
			a.onEmailChanged()
		}
	case idUser:
		if notify == winapi.EN_CHANGE && !a.suppressEdit {
			a.userSetUser = true
		}
	case idServer:
		if notify == winapi.EN_CHANGE && !a.suppressEdit {
			a.userSetServer = true
		}

	case idTest:
		if notify == winapi.BN_CLICKED {
			a.startTest()
		}
	case idRefresh:
		if notify == winapi.BN_CLICKED {
			a.startRefresh()
		}

	case idMoveAfter:
		if notify == winapi.BN_CLICKED {
			// The target folder combo only exists while the box is ticked.
			a.relayout()
		}

	case idSaveDir:
		if notify == winapi.BN_CLICKED {
			if dir := winapi.BrowseForFolder(a.hwnd, a.tr.BrowseTitle, a.saveDir); dir != "" {
				a.saveDir = dir
				a.updateSaveDirLabel()
			}
		}

	case idDeleteFrom, idDeleteTo, idCopyFrom, idCopyTo, idMoveFrom, idMoveTo:
		if notify == winapi.BN_CLICKED {
			// A picker is usable only while its date filter is switched on.
			a.updateEnabled()
		}

	case idDeleteRun:
		a.onRun(notify, taskDelete, a.startDelete)
	case idCopyRun:
		a.onRun(notify, taskCopy, a.startCopy)
	case idMoveRun:
		a.onRun(notify, taskMove, a.startMove)
	}
}

// onRun handles a Start/Stop button: it stops the job the button belongs to
// while that job runs, and starts it when nothing runs at all.
func (a *App) onRun(notify uint16, t task, start func()) {
	if notify != winapi.BN_CLICKED {
		return
	}
	switch a.running {
	case t:
		a.stopRunning()
	case taskNone:
		start()
	}
}

func (a *App) setLanguage(l i18n.Lang) {
	if l == a.lang {
		return
	}
	a.lang = l
	a.tr = i18n.Get(l)
	a.retranslate()
}

// ------------------------------------------------------- Mailbox shortcuts

// knownIMAP maps the providers people actually use here to their IMAP hosts;
// anything else falls back to "imap." plus the domain, which is right far
// more often than it is wrong.
var knownIMAP = map[string]string{
	"yandex.ru":      "imap.yandex.ru",
	"yandex.com":     "imap.yandex.com",
	"yandex.by":      "imap.yandex.ru",
	"yandex.kz":      "imap.yandex.ru",
	"ya.ru":          "imap.yandex.ru",
	"gmail.com":      "imap.gmail.com",
	"googlemail.com": "imap.gmail.com",
	"mail.ru":        "imap.mail.ru",
	"inbox.ru":       "imap.mail.ru",
	"bk.ru":          "imap.mail.ru",
	"list.ru":        "imap.mail.ru",
	"internet.ru":    "imap.mail.ru",
	"rambler.ru":     "imap.rambler.ru",
	"lenta.ru":       "imap.rambler.ru",
	"outlook.com":    "outlook.office365.com",
	"hotmail.com":    "outlook.office365.com",
	"live.com":       "outlook.office365.com",
	"msn.com":        "outlook.office365.com",
	"icloud.com":     "imap.mail.me.com",
	"me.com":         "imap.mail.me.com",
	"yahoo.com":      "imap.mail.yahoo.com",
	"zoho.com":       "imap.zoho.com",
	"qq.com":         "imap.qq.com",
}

const defaultPort = "993"

// onEmailChanged copies the address into the user name field and fills in
// the server and port for the domain - until the user edits either field
// themselves, after which their value is left alone.
func (a *App) onEmailChanged() {
	email := strings.TrimSpace(winapi.GetWindowText(a.edEmail))

	a.suppressEdit = true
	defer func() { a.suppressEdit = false }()

	if !a.userSetUser {
		winapi.SetWindowText(a.edUser, email)
	}
	if !a.userSetServer {
		if host := guessServer(email); host != "" {
			winapi.SetWindowText(a.edServer, host)
			winapi.SetWindowText(a.edPort, defaultPort)
		}
	}
}

func guessServer(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return ""
	}
	domain := strings.ToLower(strings.TrimSpace(email[at+1:]))
	if domain == "" || !strings.Contains(domain, ".") {
		return ""
	}
	if host, ok := knownIMAP[domain]; ok {
		return host
	}
	return "imap." + domain
}

// ------------------------------------------------------------- Field access

// mailboxConfig reads the mailbox settings. problem is a message for the
// user when something is missing or wrong.
func (a *App) mailboxConfig() (cfg imapc.Config, problem string) {
	email := strings.TrimSpace(winapi.GetWindowText(a.edEmail))
	password := winapi.GetWindowText(a.edPassword)
	server := strings.TrimSpace(winapi.GetWindowText(a.edServer))
	portText := strings.TrimSpace(winapi.GetWindowText(a.edPort))
	user := strings.TrimSpace(winapi.GetWindowText(a.edUser))

	switch {
	case email == "":
		return imapc.Config{}, a.tr.ErrNoEmail
	case password == "":
		return imapc.Config{}, a.tr.ErrNoPassword
	case server == "":
		return imapc.Config{}, a.tr.ErrNoServer
	}

	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return imapc.Config{}, a.tr.ErrBadPort
	}
	if user == "" {
		user = email
	}

	return imapc.Config{
		Server:         server,
		Port:           port,
		Username:       user,
		Password:       password,
		ConnectTimeout: imapc.DefaultConnectTimeout,
		ReadTimeout:    imapc.DefaultReadTimeout,
		Retries:        imapc.DefaultRetries,
	}, ""
}

// selectedFolder returns the folder a combo box points at.
func (a *App) selectedFolder(combo uintptr) (imapc.Folder, bool) {
	i := winapi.ComboGetSel(combo)
	if i < 0 || i >= len(a.folders) {
		return imapc.Folder{}, false
	}
	return a.folders[i], true
}

// date reads one end of a period, returning nil when the filter is switched
// off - which the jobs treat as "no limit on this side".
func (d dateFilter) date() *time.Time {
	if !winapi.Checked(d.check) {
		return nil
	}
	st, ok := winapi.DateGet(d.picker)
	if !ok {
		return nil
	}
	t := time.Date(int(st.Year), time.Month(st.Month), int(st.Day), 0, 0, 0, 0, time.Local)
	return &t
}

// period reads a pair of date filters and rejects a range that runs
// backwards.
func (a *App) period(fromFilter, toFilter dateFilter) (from, to *time.Time, problem string) {
	from, to = fromFilter.date(), toFilter.date()
	if from != nil && to != nil && from.After(*to) {
		return nil, nil, a.tr.ErrDateOrder
	}
	return from, to, ""
}

// ------------------------------------------------------------ Folder combos

// fillFolders refreshes every folder combo, keeping whatever was selected
// before when the folder is still there. Names arrive already decoded from
// modified UTF-7, so Cyrillic folders read correctly.
func (a *App) fillFolders(folders []imapc.Folder) {
	combos := a.folderCombos()

	// Read what is selected now, while a.folders still describes the lists
	// the indices belong to. A combo with nothing selected yet falls back to
	// the choice restored from the settings.
	previous := make([]string, len(combos))
	for i, c := range combos {
		if f, ok := a.selectedFolder(c); ok {
			previous[i] = f.Raw
		} else {
			previous[i] = a.wanted[c]
		}
	}
	clear(a.wanted)

	selectable := make([]imapc.Folder, 0, len(folders))
	for _, f := range folders {
		if f.Selectable() {
			selectable = append(selectable, f)
		}
	}
	a.folders = selectable

	for i, c := range combos {
		a.fillCombo(c)
		index := -1
		if previous[i] != "" {
			index = a.indexOfRaw(previous[i])
		}
		if index < 0 {
			index = a.defaultFolder(c)
		}
		winapi.ComboSetSel(c, index)
	}
}

// defaultFolder is the folder a combo starts on when there is nothing to
// restore: INBOX for a source, the Trash for a move target, and nothing at
// all for the folder to delete from, which the user has to pick on purpose.
func (a *App) defaultFolder(combo uintptr) int {
	switch combo {
	case a.cbCopySrc, a.cbMoveSrc:
		return a.indexOfInbox()
	case a.cbCopyDst, a.cbMoveDst:
		return a.indexOfTrash()
	}
	return -1
}

func (a *App) folderCombos() []uintptr {
	return []uintptr{a.cbDeleteSrc, a.cbCopySrc, a.cbCopyDst, a.cbMoveSrc, a.cbMoveDst}
}

func (a *App) fillCombo(c uintptr) {
	winapi.ComboReset(c)
	for _, f := range a.folders {
		winapi.ComboAdd(c, f.DisplayName(a.tr.FolderInbox))
	}
}

// relabelFolderCombos rewrites the lists in the new language. Only INBOX
// actually changes, but the whole list is rebuilt so the selection and the
// order stay in step with a.folders.
func (a *App) relabelFolderCombos() {
	for _, c := range a.folderCombos() {
		selected := winapi.ComboGetSel(c)
		a.fillCombo(c)
		winapi.ComboSetSel(c, selected)
	}
}

func (a *App) indexOfRaw(raw string) int {
	for i, f := range a.folders {
		if f.Raw == raw {
			return i
		}
	}
	return -1
}

func (a *App) indexOfInbox() int {
	for i, f := range a.folders {
		if strings.EqualFold(f.Raw, "INBOX") {
			return i
		}
	}
	if len(a.folders) > 0 {
		return 0
	}
	return -1
}

// trashNames are the folder names servers without the \Trash special-use
// attribute tend to use.
var trashNames = []string{"trash", "корзина", "deleted items", "deleted messages", "удалённые", "удаленные"}

func (a *App) indexOfTrash() int {
	for i, f := range a.folders {
		if f.HasFlag(`\Trash`) {
			return i
		}
	}
	for i, f := range a.folders {
		name := strings.ToLower(f.Name)
		if idx := strings.LastIndexAny(name, "/."); idx >= 0 {
			name = name[idx+1:]
		}
		for _, t := range trashNames {
			if name == t {
				return i
			}
		}
	}
	return -1
}

// --------------------------------------------------------------- Dialogues

func (a *App) info(text string) {
	winapi.MessageBox(a.hwnd, text, a.tr.CapInfo, winapi.MB_OK|winapi.MB_ICONINFORMATION)
}

func (a *App) warn(text string) {
	winapi.MessageBox(a.hwnd, text, a.tr.CapWarning, winapi.MB_OK|winapi.MB_ICONWARNING)
}

func (a *App) fail(text string) {
	winapi.MessageBox(a.hwnd, text, a.tr.CapError, winapi.MB_OK|winapi.MB_ICONERROR)
}

// confirm asks a yes/no question with "No" as the default answer, so that
// pressing Enter by reflex does not set off a destructive job.
func (a *App) confirm(text string) bool {
	answer := winapi.MessageBox(a.hwnd, text, a.tr.CapConfirm,
		winapi.MB_YESNO|winapi.MB_ICONWARNING|winapi.MB_DEFBUTTON2)
	return answer == winapi.IDYES
}
