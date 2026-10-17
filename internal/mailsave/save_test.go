package mailsave

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"

	"mailhelper/internal/i18n"
)

func cp1251(t *testing.T, s string) string {
	t.Helper()
	out, err := charmap.Windows1251.NewEncoder().String(s)
	if err != nil {
		t.Fatalf("encode to windows-1251: %v", err)
	}
	return out
}

// buildMessage assembles a message of the shape real Russian mail arrives
// in: RFC 2047 headers in windows-1251, an HTML body in the same charset,
// and a base64 attachment.
func buildMessage(t *testing.T) []byte {
	t.Helper()

	fromName := base64.StdEncoding.EncodeToString([]byte(cp1251(t, "Иван Петров")))
	subject := base64.StdEncoding.EncodeToString([]byte(cp1251(t, "Отчёт за август")))
	fileName := base64.StdEncoding.EncodeToString([]byte(cp1251(t, "отчёт.pdf")))
	body := cp1251(t, "<html><head><meta charset=\"windows-1251\"></head><body>"+
		"<p>Здравствуйте!</p><script>alert(1)</script>"+
		"<img src=\"cid:logo@example\">"+
		"</body></html>")
	attachment := base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 fake"))
	logo := base64.StdEncoding.EncodeToString([]byte("PNG-bytes"))

	return []byte(strings.Join([]string{
		"From: =?windows-1251?B?" + fromName + "?= <ivan@example.ru>",
		"To: me@example.com",
		"Subject: =?windows-1251?B?" + subject + "?=",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="BOUND"`,
		"",
		"--BOUND",
		"Content-Type: text/html; charset=windows-1251",
		"Content-Transfer-Encoding: 8bit",
		"",
		body,
		"--BOUND",
		"Content-Type: image/png",
		"Content-Transfer-Encoding: base64",
		"Content-ID: <logo@example>",
		`Content-Disposition: inline; filename="logo.png"`,
		"",
		logo,
		"--BOUND",
		"Content-Type: application/pdf",
		"Content-Transfer-Encoding: base64",
		`Content-Disposition: attachment; filename="=?windows-1251?B?` + fileName + `?="`,
		"",
		attachment,
		"--BOUND--",
		"",
	}, "\r\n"))
}

func TestSaveWritesFoldersAttachmentsAndContent(t *testing.T) {
	root := t.TempDir()
	tr := i18n.Get(i18n.RU)

	saved, err := Save(root, buildMessage(t), received, tr, false)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	// The day folder is named after the date the message arrived.
	if got := filepath.Base(filepath.Dir(saved.Dir)); got != "08.09.2026" {
		t.Errorf("day folder = %q, want 08.09.2026", got)
	}

	name := filepath.Base(saved.Dir)
	if !strings.HasSuffix(name, "17-56-23") {
		t.Errorf("message folder = %q, want it to end with the received time", name)
	}
	if !strings.Contains(name, "Иван Петров") || !strings.Contains(name, "ivan@example.ru") ||
		!strings.Contains(name, "Отчёт за август") {
		t.Errorf("message folder = %q, want sender, address and subject", name)
	}
	if n := len([]rune(name)); n > MaxDirNameLen {
		t.Errorf("message folder is %d runes", n)
	}

	// Both attachments land next to content.html under their own names.
	if saved.Attachments != 2 {
		t.Errorf("saved %d attachments, want 2", saved.Attachments)
	}
	pdf := filepath.Join(saved.Dir, "отчёт.pdf")
	data, err := os.ReadFile(pdf)
	if err != nil {
		t.Fatalf("attachment: %v", err)
	}
	if string(data) != "%PDF-1.4 fake" {
		t.Errorf("attachment content = %q", data)
	}

	page, err := os.ReadFile(filepath.Join(saved.Dir, "content.html"))
	if err != nil {
		t.Fatalf("content.html: %v", err)
	}
	html := string(page)

	// The four header lines come first, in the language of the UI.
	for _, want := range []string{
		tr.HTMLSender, tr.HTMLSenderEmail, tr.HTMLReceived, tr.HTMLSubject,
		"Иван Петров", "ivan@example.ru", "Отчёт за август", "08.09.2026 17:56:23",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("content.html is missing %q", want)
		}
	}
	headerEnd := strings.Index(html, "Здравствуйте")
	if headerEnd < 0 {
		t.Fatal("the message body was not written")
	}
	if strings.Index(html, tr.HTMLSubject) > headerEnd {
		t.Error("the header block must come before the message body")
	}

	// The body arrived in windows-1251 and has to be UTF-8 on disk.
	if !strings.Contains(html, "Здравствуйте!") {
		t.Error("the body was not converted to UTF-8")
	}
	if strings.Contains(html, "alert(1)") {
		t.Error("a script from the message survived into the saved page")
	}
	if strings.Contains(html, `charset="windows-1251"`) {
		t.Error("the message's own charset declaration must not override the file's")
	}

	// Links to the local files close the page, and cid: references point at
	// the saved copy rather than at nothing.
	if !strings.Contains(html, `download="отчёт.pdf"`) {
		t.Error("content.html has no download link for the attachment")
	}
	if strings.Contains(html, "cid:logo@example") {
		t.Error("the embedded image still points at a cid: URL")
	}
	if !strings.Contains(html, `src="logo.png"`) {
		t.Error("the embedded image was not repointed at the saved file")
	}
	if idx := strings.Index(html, tr.HTMLAttachments); idx < headerEnd {
		t.Error("the attachment list belongs at the end of the page")
	}
}

func TestSavePlainTextMessage(t *testing.T) {
	root := t.TempDir()
	raw := []byte(strings.Join([]string{
		"From: noreply@example.com",
		"Subject: plain",
		"Content-Type: text/plain; charset=koi8-r",
		"Content-Transfer-Encoding: base64",
		"",
		base64.StdEncoding.EncodeToString([]byte{0xf0, 0xd2, 0xc9, 0xd7, 0xc5, 0xd4}), // "Привет"
		"",
	}, "\r\n"))

	saved, err := Save(root, raw, received, i18n.Get(i18n.EN), false)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	page, err := os.ReadFile(filepath.Join(saved.Dir, "content.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "Привет") {
		t.Errorf("koi8-r body was not decoded: %s", page)
	}
	if saved.Attachments != 0 {
		t.Errorf("a plain body must not become an attachment")
	}
}

func TestSaveUnparsableMessageKeepsTheOriginal(t *testing.T) {
	root := t.TempDir()

	saved, err := Save(root, []byte("this is not a message at all"), received, i18n.Get(i18n.EN), false)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(saved.Dir, "message.eml")); err != nil {
		t.Errorf("the original bytes were not kept: %v", err)
	}
}

// A message out of the sent folder is filed under the person it went to:
// the sender is the owner of the mailbox on every one of them.
func TestSaveFromSentNamesTheFolderAfterTheRecipient(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"From: Owner <owner@example.com>",
		`To: "Petrov, Ivan" <ivan@example.ru>, second@example.ru`,
		"Subject: report",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"body",
		"",
	}, "\r\n"))

	saved, err := Save(t.TempDir(), raw, received, i18n.Get(i18n.EN), true)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	name := filepath.Base(saved.Dir)
	if !strings.Contains(name, "ivan@example.ru") || !strings.Contains(name, "Petrov, Ivan") {
		t.Errorf("the folder is not named after the recipient: %q", name)
	}
	if strings.Contains(name, "owner@example.com") {
		t.Errorf("the sender still appears in the folder name: %q", name)
	}

	// The same message taken from an ordinary folder keeps the sender.
	saved, err = Save(t.TempDir(), raw, received, i18n.Get(i18n.EN), false)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if name := filepath.Base(saved.Dir); !strings.Contains(name, "owner@example.com") {
		t.Errorf("a received message must stay named after the sender: %q", name)
	}
}

// A sent message with nothing usable in To falls back to Cc, and then to the
// sender, rather than losing the name altogether.
func TestSaveFromSentFallsBackToCcThenSender(t *testing.T) {
	message := func(headers ...string) []byte {
		return []byte(strings.Join(append(headers,
			"Subject: report", "Content-Type: text/plain; charset=utf-8", "", "body", ""), "\r\n"))
	}

	saved, err := Save(t.TempDir(), message(
		"From: Owner <owner@example.com>", "To: ", "Cc: copy@example.ru"), received, i18n.Get(i18n.EN), true)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if name := filepath.Base(saved.Dir); !strings.Contains(name, "copy@example.ru") {
		t.Errorf("Cc was not used when To was empty: %q", name)
	}

	saved, err = Save(t.TempDir(), message(
		"From: Owner <owner@example.com>"), received, i18n.Get(i18n.EN), true)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if name := filepath.Base(saved.Dir); !strings.Contains(name, "owner@example.com") {
		t.Errorf("a message with no recipient at all must keep the sender: %q", name)
	}
}
