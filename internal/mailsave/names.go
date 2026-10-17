package mailsave

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// MaxDirNameLen is the length a message folder name is trimmed to. Windows
// itself allows more, but the whole path has to stay workable once the user
// buries the save folder a few levels deep, so the name is capped here.
const MaxDirNameLen = 80

// timeLayout is the received time appended to every message folder name. It
// is never truncated: the trimming budget is computed around it.
const timeLayout = "15-04-05"

// DateLayout is the day folder: 31.12.2026.
const DateLayout = "02.01.2006"

// forbidden lists the characters Windows refuses in a file or folder name.
const forbidden = `<>:"/\|?*`

// reserved device names cannot be used for a file or folder, with or
// without an extension.
var reserved = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// sanitize drops the characters Windows forbids, folds every run of
// whitespace into one space and trims the result. Trailing dots and spaces
// go too: Windows silently strips them, which would otherwise make the name
// on disk differ from the name we think we created.
func sanitize(s string) string {
	var b strings.Builder
	lastSpace := false
	for _, r := range s {
		switch {
		case strings.ContainsRune(forbidden, r):
			// removed outright
		case r < 0x20 || r == 0x7F || unicode.IsSpace(r):
			// control characters and every kind of space fold into one
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
		default:
			b.WriteRune(r)
			lastSpace = false
		}
	}
	out := strings.Trim(b.String(), " .")
	if reserved[strings.ToUpper(out)] {
		out = "_" + out
	}
	return out
}

// truncateRunes cuts a string to at most n runes and trims what that leaves
// dangling at the end.
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return strings.Trim(s, " .")
	}
	return strings.Trim(string(r[:n]), " .")
}

// MessageDirName builds the folder name for one message:
// "sender name, e-mail, subject, received time", cleaned of characters
// Windows forbids and trimmed to MaxDirNameLen runes. The time is appended
// after trimming, so it always survives in full.
func MessageDirName(senderName, senderEmail, subject string, received time.Time) string {
	stamp := received.Format(timeLayout)

	head := sanitize(strings.Join(nonEmpty(senderName, senderEmail, subject), " "))
	budget := MaxDirNameLen - len([]rune(stamp)) - 1 // -1 for the separating space
	head = truncateRunes(head, budget)

	if head == "" {
		return stamp
	}
	return head + " " + stamp
}

func nonEmpty(vals ...string) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// MakeUniqueDir creates name inside parent, adding " (2)", " (3)" ... when
// that name is taken. The suffix is made to fit inside MaxDirNameLen by
// shortening the descriptive head, never the time stamp.
func MakeUniqueDir(parent, name string) (string, error) {
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}

	candidate := name
	for i := 2; ; i++ {
		path := filepath.Join(parent, candidate)
		err := os.Mkdir(path, 0o755)
		if err == nil {
			return path, nil
		}
		if !os.IsExist(err) {
			return "", err
		}
		if i > 999 {
			return "", fmt.Errorf("cannot create a unique folder for %q", name)
		}

		suffix := fmt.Sprintf(" (%d)", i)
		candidate = truncateRunes(name, MaxDirNameLen-len([]rune(suffix))) + suffix
	}
}

// FileName cleans an attachment file name and keeps it to a workable length
// while preserving the extension, which is what makes the file openable.
func FileName(name string, fallback string) string {
	name = sanitize(filepath.Base(strings.ReplaceAll(name, `\`, "/")))
	if name == "" || name == "." || name == ".." {
		name = fallback
	}

	const maxFile = 100
	if len([]rune(name)) <= maxFile {
		return name
	}
	ext := filepath.Ext(name)
	if len([]rune(ext)) > 16 { // not really an extension
		ext = ""
	}
	stem := strings.TrimSuffix(name, ext)
	return truncateRunes(stem, maxFile-len([]rune(ext))) + ext
}

// uniqueFileName picks a name not yet used in the set, adding " (2)" and so
// on before the extension.
func uniqueFileName(used map[string]bool, name string) string {
	key := strings.ToLower(name)
	if !used[key] {
		used[key] = true
		return name
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", stem, i, ext)
		key = strings.ToLower(candidate)
		if !used[key] {
			used[key] = true
			return candidate
		}
	}
}
