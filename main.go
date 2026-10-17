//go:build windows

// MailHelper copies messages from an IMAP mailbox into dated folders on
// disk, and moves messages between folders on the server. The window is
// plain Win32 with per-monitor DPI awareness, so dragging it between
// monitors with different resolutions and scaling keeps it crisp.
//
// Build:
//
//	go build -ldflags "-H=windowsgui -s -w" -o MailHelper.exe .
package main

import (
	_ "embed"

	"mailhelper/internal/app"
)

// The icon is baked into the executable so the window, the task bar and
// Alt-Tab all show it without a file sitting next to the binary.
//
//go:embed icon.png
var iconPNG []byte

func main() {
	app.Run(iconPNG)
}
