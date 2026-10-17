# MailHelper

A Windows 10/11 desktop tool for an IMAP mailbox. It does three jobs:

* **Copy to a folder on disk** - saves the messages of a period into dated
  folders, one folder per message, with the attachments as files and a
  `content.html` that shows the message and links to them. Optionally moves
  each batch on the server once it is safely on disk.
* **Move from folder to folder on the server** - moves the messages of a
  period into another server folder (the Trash by default) with
  COPY + `\Deleted` + EXPUNGE.
* **Delete messages** - removes the messages of a period from a server folder
  for good with `\Deleted` + EXPUNGE, after a confirmation. Its panel is
  tinted red as the danger zone of the window.

The interface is English and Russian, in a light or a dark theme. On the
first run it opens in the language Windows itself is displayed in when that
is Russian, and in English otherwise, and in the theme Windows is set to use
for applications; the two combo boxes at the top switch either at any time,
including while the folder lists are already filled.

## Settings

The language, the theme, the mailbox settings except the password, the save
folder, the "move after copying" box and the folders chosen for copying and
moving are remembered between runs in

```
%AppData%\MailHelper\settings.json
```

The file is written when the window closes. The folder to delete from is
deliberately not remembered, so a destructive job always starts from an
explicit choice.

## Build

```powershell
go build -trimpath -ldflags "-H=windowsgui -s -w" -o MailHelper.exe .
```

`-H=windowsgui` drops the console window, and `-trimpath` keeps the paths of
the build machine out of the executable. The only dependency is
`golang.org/x/text`, for the legacy charsets that Russian mail still arrives
in; everything else is the standard library plus the Win32 API through
`syscall`, no cgo and no UI toolkit.

## Run

```powershell
.\MailHelper.exe
```

Type the address into **E-mail** and the rest of the mailbox settings fill
themselves in: the user name is copied from the address, and the domain
selects the IMAP host and port 993 - `imap.yandex.ru` for `user@yandex.ru`,
`imap.gmail.com` for `user@gmail.com`, and so on, falling back to `imap.` plus
the domain. Editing the server or the user name yourself stops the automatic
filling from touching that field again.

**App password** is what it says: providers that require two-factor
authentication - Yandex, Gmail, Mail.ru - issue a separate password for IMAP
clients. The application never writes it anywhere.

**Test connection** and **Refresh folder list** both need only the mailbox
settings. The folder lists feed all five folder combo boxes.

## The window

The GUI is a plain Win32 window with hand-created controls, because that is
what makes correct per-monitor DPI behaviour possible.

| Concern | Mechanism |
| --- | --- |
| Mixed-DPI monitors | per-monitor v2 awareness in the manifest, plus a `WM_DPICHANGED` handler that rebuilds the fonts, the frame and every control position |
| Layout | every size is a logical pixel multiplied by the scale of the monitor the window is on; nothing is measured in physical pixels. Buttons are as wide as their captions, so the layout is recomputed when the language changes |
| Panels | painted by the window itself: a rounded frame with a caption, filled red for **Delete messages**; the controls on a panel take its colour through `WM_CTLCOLOR*` |
| Themes | `WM_CTLCOLOR*` plus the `DarkMode_Explorer` / `DarkMode_CFD` visual styles for buttons, checkboxes, edits and combo boxes, and `DWMWA_USE_IMMERSIVE_DARK_MODE` for the title bar |
| Progress bars | owner-drawn, so the message count "n / m" can sit inside the bar; the **Start** button sits after the bar on the same line |
| Date fields | a regular checkbox that switches the filter on, next to a `SysDateTimePick32` it enables. The picker's own `DTS_SHOWNONE` checkbox is not used: Windows 11 stretches it to the height of the control |
| Dark date fields | the picker has no dark style and ignores `WM_CTLCOLOR*`, so in the dark theme it paints into an off-screen bitmap whose lightness is inverted, hue kept, before it reaches the screen |
| Icon | `icon.png` embedded with `go:embed` and rendered to the exact pixel size the shell asks for at the current DPI |

### Mixed-DPI monitors

Dragging a window between monitors with different scaling makes Windows
bitmap-stretch a system-DPI-aware window. This one is declared **per-monitor
v2 aware** in `winres/winres.json`, and `WM_DPICHANGED` lays everything out
again at the new scale: the logical size stays put and the window is redrawn
rather than stretched.

Note that the window has no `WS_CLIPCHILDREN`. The panels are painted by the
window underneath its controls, and invalidating the window has to repaint
the controls on top of them too, which is what Windows does for a parent
without that style.

## What the tasks do

All three tasks work in **batches of ten messages**, and all address messages
by UID, so an EXPUNGE in the middle of a run cannot make the remaining work
point at the wrong messages.

While a job runs every input control is disabled except the Stop button of
that job. A pop-up reports the outcome, and the progress bar goes back to
empty once it is dismissed. Closing the window during a job asks the job to
stop and closes once it has unwound.

### Dates

Either date filter can be left switched off, in which case that end is not
filtered.
`From date` becomes IMAP `SINCE`, `To date` becomes `BEFORE` the following day
so that the day the user picked is included. The server compares against its
own internal date, so a message that arrived within a few hours of midnight
can fall on the neighbouring day.

### Copy to a folder on disk

```
<chosen folder>/
  08.09.2026/
    Иван Петров ivan@example.ru Отчёт за август 17-56-23/
      content.html
      отчёт.pdf
      logo.png
```

* The day folder is the date the message arrived, in the user's own time zone.
* The message folder is the sender's name, address and subject followed by the
  arrival time. Where the source is a folder of outgoing mail - one the server
  marks `\Sent`, or named `Sent`, `Sent Items`, `Отправленные` and the like -
  the folder is named after the first recipient instead, since the sender
  there is the owner of the mailbox on every message; `Cc` stands in for a
  message with no `To`. Characters Windows forbids (`<>:"/\|?*`, control
  characters) are removed, runs of whitespace collapse to one space, and the
  name is cut to 80 characters - the time is appended after the cut, so it is never
  truncated. A second message that would land on the same name gets ` (2)`.
* `content.html` opens with the sender, the sender's address, the arrival time
  and the subject, one per line and in the language the UI is set to, then the
  message itself, then links to the files next to it. The links carry
  `download`, so clicking one saves the file.
* Bodies and headers in windows-1251, KOI8-R, CP866 and the rest are converted
  to UTF-8. `cid:` references to embedded images are repointed at the saved
  copies. Scripts are stripped out of the message body: the page is opened
  from the file system, and nothing in an e-mail needs to run there.
* A message the MIME parser cannot read is not skipped - its bytes are saved
  as `message.eml` and the run carries on.

### Move from folder to folder on the server

COPY, then `+FLAGS.SILENT (\Deleted)`, then EXPUNGE, ten UIDs at a time. Where
the server supports UIDPLUS the expunge is `UID EXPUNGE`, limited to the
messages just handled, so messages somebody else flagged in the same mailbox
are left alone.

Any refusal from the server stops the run at once and reports the reply.

When **Move on the server after copying to disk** is ticked in the copy
panel, each batch is moved once its messages are on disk. Stopping the job
in the middle of a batch still moves the messages of that batch already
saved, so the next run does not copy them a second time.

### Delete messages

`+FLAGS.SILENT (\Deleted)`, then EXPUNGE (`UID EXPUNGE` with UIDPLUS), ten
UIDs at a time. **Start** first asks "Are you sure? The messages will be
deleted from your mailbox." with **No** as the default answer. The folder has
no default either: it stays empty until it is picked by hand.

Without UIDPLUS a plain EXPUNGE also removes any other message in that folder
that was already flagged `\Deleted` by another client.

## Folder names

IMAP mailbox names travel in "modified UTF-7" (RFC 3501 5.1.3): `Корзина` goes
over the wire as `&BBoEPgRABDcEOAQ9BDA-`. Skipping that conversion is exactly
what turns Russian folder names into mojibake. Every name from the server is
decoded before it reaches the UI, and every name sent back to the server is
the raw form the server gave us, never the decoded one.

`INBOX` is the one folder name the protocol fixes rather than the owner of the
mailbox choosing it, so the lists show it translated - **Inbox** or
**Входящие**, and `INBOX/Работа` as `Входящие/Работа` - and relabel themselves
when the language changes. Every other folder is shown exactly as the server
named it, and the server always hears the name it gave us.

## Connection

TLS on port 993, or plain with a mandatory STARTTLS upgrade on port 143. The
connect timeout is 30 seconds and the read timeout 90, refreshed per 64 KB
chunk so that a large but steadily arriving message is not cut off by the
timeout meant for a stalled link.

A dropped connection is retried three times: the session re-dials, logs in
again and re-selects the mailbox it was working in. A tagged `NO` or `BAD` is
the server making a decision rather than a glitch, so it is never retried and
stops the job immediately.

`BODY.PEEK[]` is used rather than `BODY[]`, so copying a folder to disk does
not silently mark it as read.

## Layout of the source

```
main.go                 embeds icon.png, starts the UI
internal/app/           the window: layout, DPI, themes, events, the three jobs
internal/winapi/        the Win32 bindings this application needs
internal/imapc/         IMAP client, session with retries, modified UTF-7
internal/mailsave/      MIME -> folder on disk, content.html, Windows-safe names
internal/i18n/          the English and Russian strings
internal/settings/      settings.json in %AppData%\MailHelper
internal/appicon/       icon.png -> HICON at the size the shell asks for
tools/genicon/          renders winres/icon*.png from icon.png
```

## Regenerating the icon resource

The window icon comes from the embedded PNG at run time; the icon on the
`.exe` file itself comes from the linked resource, which needs regenerating
when the artwork or the manifest changes:

```powershell
go run ./tools/genicon
go-winres make --arch amd64 --out rsrc
```

The generated `rsrc_windows_amd64.syso` is committed; the intermediate
`winres/icon*.png` are not.

## Tests and linting

```powershell
go test ./...
golangci-lint run ./...
```

The linter configuration is `.golangci.yml` (golangci-lint v2).

The IMAP client is tested against a scripted server over an in-memory pipe:
folder listing with modified UTF-7 and literal names, `SELECT`/`UID SEARCH`,
a `FETCH` whose body arrives as a literal, the exact COPY/STORE/EXPUNGE
sequence a move issues and the STORE/EXPUNGE sequence a delete issues, that a
tagged `NO` is not retried, and the INBOX relabelling against the folders it
must and must not touch. The disk side is tested end to end on a windows-1251
message with an RFC 2047 subject, a base64 attachment and an embedded image.
The remaining tests cover the settings file, the mapping from a Windows
LANGID onto a language, that every string is translated with the same format
arguments as the Russian original, and the recolouring of the dark date
fields.

## Licences

`THIRD_PARTY_NOTICES.md` carries the licence of the Go standard library and of
`golang.org/x/text`, both BSD-3-Clause, whose code is compiled into the
executable; ship it alongside `MailHelper.exe`.
