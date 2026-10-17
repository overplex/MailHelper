//go:build windows

package app

import (
	"fmt"
	"strconv"
	"strings"

	"mailhelper/internal/settings"
	"mailhelper/internal/winapi"
)

// applySettings fills the controls from the saved settings. The language and
// the theme are picked up separately, before the window is laid out.
func (a *App) applySettings(s settings.Settings) {
	a.suppressEdit = true
	winapi.SetWindowText(a.edEmail, s.Email)
	winapi.SetWindowText(a.edServer, s.Server)
	winapi.SetWindowText(a.edUser, s.Username)
	port := defaultPort
	if s.Port > 0 {
		port = strconv.Itoa(s.Port)
	}
	winapi.SetWindowText(a.edPort, port)
	a.suppressEdit = false

	// A saved value that differs from what the e-mail address would fill in
	// was typed by the user, so it must survive further edits of the address.
	a.userSetUser = s.Username != "" && s.Username != s.Email
	a.userSetServer = s.Server != "" && s.Server != guessServer(s.Email)

	a.saveDir = s.SaveDir
	winapi.SetChecked(a.chkMoveAfter, s.MoveAfterCopy)

	a.wanted[a.cbCopySrc] = s.CopySource
	a.wanted[a.cbCopyDst] = s.CopyTarget
	a.wanted[a.cbMoveSrc] = s.MoveSource
	a.wanted[a.cbMoveDst] = s.MoveTarget
}

// currentSettings collects what is to be remembered. The password and the
// folder to delete from are left out on purpose.
func (a *App) currentSettings() settings.Settings {
	port, _ := strconv.Atoi(strings.TrimSpace(winapi.GetWindowText(a.edPort)))
	folder := func(combo uintptr) string {
		if f, ok := a.selectedFolder(combo); ok {
			return f.Raw
		}
		return a.wanted[combo] // the list was never fetched in this run
	}
	return settings.Settings{
		Language:      a.lang.Code(),
		Theme:         a.theme.code(),
		Email:         strings.TrimSpace(winapi.GetWindowText(a.edEmail)),
		Server:        strings.TrimSpace(winapi.GetWindowText(a.edServer)),
		Port:          port,
		Username:      strings.TrimSpace(winapi.GetWindowText(a.edUser)),
		SaveDir:       a.saveDir,
		MoveAfterCopy: a.moveAfterCopy(),
		CopySource:    folder(a.cbCopySrc),
		CopyTarget:    folder(a.cbCopyDst),
		MoveSource:    folder(a.cbMoveSrc),
		MoveTarget:    folder(a.cbMoveDst),
	}
}

// saveSettings writes the settings file, telling the user when it cannot.
func (a *App) saveSettings() {
	if err := settings.Save(a.currentSettings()); err != nil {
		a.warn(fmt.Sprintf(a.tr.ErrSaveSettings, err))
	}
}
