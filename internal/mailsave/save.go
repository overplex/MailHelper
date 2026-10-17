// Package mailsave turns a fetched RFC 5322 message into a folder on disk:
// the attachments as files, plus a content.html that shows the message with
// its sender, time and subject at the top and links to those files at the
// bottom.
package mailsave

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"mailhelper/internal/i18n"
)

// Saved describes what one message produced on disk.
type Saved struct {
	Dir         string
	Attachments int
}

// Save writes one message under root as root/<dd.mm.yyyy>/<message folder>.
// received is the server's internal date, the arrival time the folders are
// named after. fromSent says the message came out of
// a folder of outgoing mail, where the folder is named after the person it
// was sent to instead of the sender - who is the owner of the mailbox and
// would otherwise be the same on every folder.
func Save(root string, raw []byte, received time.Time, tr *i18n.Strings, fromSent bool) (Saved, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		// An unparsable message must not stop the run: keep the original
		// bytes so nothing is lost and carry on.
		return saveUnparsed(root, raw, whenever(received), err)
	}

	header := textproto.MIMEHeader(msg.Header)

	// The server's internal date is the arrival time the folders are named
	// after; a server that did not report one leaves the Date header. Either
	// way the folder is named in the user's own time zone, so browsing by
	// date matches what their mail client shows them.
	if received.IsZero() {
		if sent, err := msg.Header.Date(); err == nil {
			received = sent
		}
	}
	received = whenever(received).Local()

	senderName, senderEmail := sender(header.Get("From"))
	subject := decodeHeader(header.Get("Subject"))

	// Who the folder is named after: the sender, or the recipient of a
	// message taken from the sent folder.
	partyName, partyEmail := senderName, senderEmail
	if fromSent {
		if n, e := recipient(header); n != "" || e != "" {
			partyName, partyEmail = n, e
		}
	}

	c := &collected{used: map[string]bool{}}
	walk(header, msg.Body, c, 0)

	dayDir := filepath.Join(root, received.Format(DateLayout))
	dir, err := MakeUniqueDir(dayDir, MessageDirName(partyName, partyEmail, subject, received))
	if err != nil {
		return Saved{}, err
	}

	for _, a := range c.atts {
		if err := os.WriteFile(filepath.Join(dir, a.fileName), a.data, 0o644); err != nil {
			return Saved{}, fmt.Errorf("%s: %w", a.fileName, err)
		}
	}

	page := buildHTML(c, senderName, senderEmail, subject, received, tr)
	if err := os.WriteFile(filepath.Join(dir, "content.html"), []byte(page), 0o644); err != nil {
		return Saved{}, err
	}

	return Saved{Dir: dir, Attachments: len(c.atts)}, nil
}

// whenever falls back to the current time for a message that carries no
// usable date at all, so it still lands in a dated folder.
func whenever(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now()
	}
	return t
}

// saveUnparsed is the fallback for a message the MIME parser rejects.
func saveUnparsed(root string, raw []byte, received time.Time, cause error) (Saved, error) {
	dayDir := filepath.Join(root, received.Format(DateLayout))
	dir, err := MakeUniqueDir(dayDir, MessageDirName("", "", "message", received))
	if err != nil {
		return Saved{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "message.eml"), raw, 0o644); err != nil {
		return Saved{}, err
	}
	page := "<!DOCTYPE html>\n<html><head><meta charset=\"utf-8\"><title>message</title></head><body>" +
		"<p>" + html.EscapeString(cause.Error()) + "</p>" +
		"<p><a href=\"message.eml\" download=\"message.eml\">message.eml</a></p>" +
		"</body></html>\n"
	if err := os.WriteFile(filepath.Join(dir, "content.html"), []byte(page), 0o644); err != nil {
		return Saved{}, err
	}
	return Saved{Dir: dir, Attachments: 1}, nil
}

// --------------------------------------------------------------- MIME walk

type attachment struct {
	fileName  string
	contentID string
	data      []byte
	inline    bool
}

type collected struct {
	html  string
	plain string
	atts  []attachment
	used  map[string]bool
}

// walk descends one MIME part. Errors inside a part are swallowed on
// purpose: a single malformed section should cost that section, not the
// whole message.
func walk(hdr textproto.MIMEHeader, body io.Reader, c *collected, depth int) {
	if depth > 20 { // a defence against pathological nesting
		return
	}

	ctHeader := hdr.Get("Content-Type")
	if ctHeader == "" {
		ctHeader = "text/plain"
	}
	mediaType, params, err := mime.ParseMediaType(ctHeader)
	if err != nil {
		mediaType, params = "text/plain", map[string]string{}
	}
	mediaType = strings.ToLower(mediaType)

	if strings.HasPrefix(mediaType, "multipart/") && params["boundary"] != "" {
		mr := multipart.NewReader(body, params["boundary"])
		for {
			// NextRawPart leaves the transfer encoding alone, so every part
			// is decoded in one place below.
			p, err := mr.NextRawPart()
			if err != nil {
				return // io.EOF, or a truncated body we take as far as it goes
			}
			walk(p.Header, p, c, depth+1)
		}
	}

	data, err := io.ReadAll(body)
	if err != nil && len(data) == 0 {
		return
	}
	data = decodeTransfer(data, hdr.Get("Content-Transfer-Encoding"))

	disposition, dispParams, _ := mime.ParseMediaType(hdr.Get("Content-Disposition"))
	disposition = strings.ToLower(disposition)

	fileName := decodeHeader(firstNonEmpty(dispParams["filename"], params["name"]))
	isText := mediaType == "text/plain" || mediaType == "text/html"

	if disposition != "attachment" && fileName == "" && isText {
		text := toUTF8(data, params["charset"])
		if mediaType == "text/html" {
			// A later HTML part in an alternative group is the richer one.
			c.html = text
		} else if c.plain == "" || len(text) > len(c.plain) {
			c.plain = text
		}
		return
	}

	if mediaType == "message/rfc822" && fileName == "" {
		fileName = "message.eml"
	}

	name := FileName(fileName, defaultName(len(c.atts)+1, mediaType))
	c.atts = append(c.atts, attachment{
		fileName:  uniqueFileName(c.used, name),
		contentID: strings.Trim(hdr.Get("Content-ID"), "<> \t"),
		data:      data,
		inline:    disposition == "inline",
	})
}

func defaultName(n int, mediaType string) string {
	ext := ""
	if exts, err := mime.ExtensionsByType(mediaType); err == nil && len(exts) > 0 {
		ext = exts[0]
	}
	return fmt.Sprintf("attachment-%02d%s", n, ext)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// decodeTransfer undoes the Content-Transfer-Encoding of a leaf part.
func decodeTransfer(data []byte, encoding string) []byte {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		return decodeBase64Loose(data)
	case "quoted-printable":
		out, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(data)))
		if err != nil && len(out) == 0 {
			return data
		}
		return out
	default:
		return data
	}
}

// decodeBase64Loose ignores the line breaks and stray characters that real
// mail is full of, and keeps whatever decoded before a bad byte.
func decodeBase64Loose(data []byte) []byte {
	clean := make([]byte, 0, len(data))
	for _, b := range data {
		switch {
		case b >= 'A' && b <= 'Z', b >= 'a' && b <= 'z', b >= '0' && b <= '9',
			b == '+', b == '/', b == '=':
			clean = append(clean, b)
		}
	}
	// Trim to a whole number of quanta so a truncated tail still decodes.
	for len(clean)%4 != 0 {
		clean = clean[:len(clean)-1]
	}
	out, err := base64.StdEncoding.DecodeString(string(clean))
	if err != nil {
		if out2, err2 := base64.StdEncoding.WithPadding(base64.NoPadding).
			DecodeString(strings.TrimRight(string(clean), "=")); err2 == nil {
			return out2
		}
		return out
	}
	return out
}

// sender splits a From header into a display name and an address.
func sender(from string) (name, email string) {
	if from == "" {
		return "", ""
	}
	parser := mail.AddressParser{WordDecoder: wordDecoder}
	if a, err := parser.Parse(from); err == nil {
		return strings.TrimSpace(a.Name), strings.TrimSpace(a.Address)
	}

	// Fall back to pulling the angle-bracketed address out by hand.
	decoded := decodeHeader(from)
	if i, j := strings.LastIndex(decoded, "<"), strings.LastIndex(decoded, ">"); i >= 0 && j > i {
		return strings.Trim(strings.TrimSpace(decoded[:i]), `"`), strings.TrimSpace(decoded[i+1 : j])
	}
	if strings.Contains(decoded, "@") {
		return "", decoded
	}
	return decoded, ""
}

// recipient picks the person a sent message went to: the first address of
// To, or of Cc where the message carries no usable To at all. Only the first
// is taken - the folder name has room for one, and it is the one a person
// looks for.
func recipient(hdr textproto.MIMEHeader) (name, email string) {
	for _, key := range []string{"To", "Cc"} {
		if name, email = firstAddress(hdr.Get(key)); name != "" || email != "" {
			return name, email
		}
	}
	return "", ""
}

// firstAddress returns the display name and address of the first entry of an
// address header.
func firstAddress(list string) (name, email string) {
	if strings.TrimSpace(list) == "" {
		return "", ""
	}
	parser := mail.AddressParser{WordDecoder: wordDecoder}
	if as, err := parser.ParseList(list); err == nil && len(as) > 0 {
		return strings.TrimSpace(as[0].Name), strings.TrimSpace(as[0].Address)
	}
	// The list did not parse as a whole - a header real mail is full of.
	// Take it apart by hand and keep the first entry that yields anything.
	for _, part := range splitAddressList(list) {
		if name, email = sender(part); name != "" || email != "" {
			return name, email
		}
	}
	return "", ""
}

// splitAddressList splits an address header on the commas that separate its
// addresses, leaving alone the ones inside a quoted display name or an angle
// bracketed address.
func splitAddressList(s string) []string {
	var out []string
	start, quoted, angle := 0, false, false
	for i, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
		case quoted:
			// a comma in "Doe, John" does not separate anything
		case r == '<':
			angle = true
		case r == '>':
			angle = false
		case r == ',' && !angle:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

// --------------------------------------------------------------- content.html

var (
	reScript      = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>`)
	reScriptOpen  = regexp.MustCompile(`(?is)</?script\b[^>]*>`)
	reCharsetMeta = regexp.MustCompile(`(?is)<meta\b[^>]*charset[^>]*>`)
)

// cleanHTML makes an e-mail body safe to open from the file system. The
// markup is otherwise left alone - including its styles - so the saved page
// still looks like the message did.
func cleanHTML(s string) string {
	s = reScript.ReplaceAllString(s, "")
	s = reScriptOpen.ReplaceAllString(s, "")
	// The wrapper below declares UTF-8; a charset the message carried would
	// describe the encoding it arrived in, which is no longer the one on disk.
	s = reCharsetMeta.ReplaceAllString(s, "")
	return s
}

const (
	headBoxStyle = "font:14px 'Segoe UI',Arial,sans-serif;color:#1a1a1a;background:#f4f6f9;" +
		"border:1px solid #d5dbe3;border-radius:6px;padding:14px 18px;margin:0 0 18px 0"
	headRowStyle   = "margin:3px 0;line-height:1.5"
	headKeyStyle   = "display:inline-block;min-width:200px;color:#5a6472"
	attachBoxStyle = "font:14px 'Segoe UI',Arial,sans-serif;color:#1a1a1a;" +
		"border-top:1px solid #d5dbe3;margin:24px 0 0 0;padding:14px 0 0 0"
)

// buildHTML assembles content.html: the four header lines in a column, the
// message body, then the attachment links.
func buildHTML(c *collected, senderName, senderEmail, subject string,
	received time.Time, tr *i18n.Strings) string {

	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	b.WriteString("<title>" + html.EscapeString(oneLine(subject)) + "</title>\n")
	b.WriteString("</head>\n<body style=\"margin:18px;background:#ffffff\">\n")

	b.WriteString("<div style=\"" + headBoxStyle + "\">\n")
	for _, row := range [][2]string{
		{tr.HTMLSender, senderName},
		{tr.HTMLSenderEmail, senderEmail},
		{tr.HTMLReceived, received.Format("02.01.2006 15:04:05")},
		{tr.HTMLSubject, subject},
	} {
		b.WriteString("<div style=\"" + headRowStyle + "\">")
		b.WriteString("<span style=\"" + headKeyStyle + "\">" + html.EscapeString(row[0]) + "</span>")
		b.WriteString("<span>" + html.EscapeString(row[1]) + "</span>")
		b.WriteString("</div>\n")
	}
	b.WriteString("</div>\n")

	switch {
	case strings.TrimSpace(c.html) != "":
		b.WriteString(inlineCIDs(cleanHTML(c.html), c.atts))
	case strings.TrimSpace(c.plain) != "":
		b.WriteString("<pre style=\"white-space:pre-wrap;word-wrap:break-word;" +
			"font:14px Consolas,'Courier New',monospace;color:#1a1a1a\">")
		b.WriteString(html.EscapeString(c.plain))
		b.WriteString("</pre>\n")
	default:
		b.WriteString("<p style=\"color:#5a6472\">" + html.EscapeString(tr.HTMLNoBody) + "</p>\n")
	}

	if len(c.atts) > 0 {
		b.WriteString("\n<div style=\"" + attachBoxStyle + "\">\n")
		b.WriteString("<div style=\"font-weight:600;margin-bottom:8px\">" +
			html.EscapeString(tr.HTMLAttachments) + "</div>\n<ul style=\"margin:0;padding-left:22px\">\n")
		for _, a := range c.atts {
			href := (&url.URL{Path: a.fileName}).EscapedPath()
			b.WriteString("<li style=\"margin:4px 0\"><a href=\"" + href +
				"\" download=\"" + html.EscapeString(a.fileName) + "\">" +
				html.EscapeString(a.fileName) + "</a> " +
				"<span style=\"color:#5a6472\">(" + humanSize(len(a.data)) + ")</span></li>\n")
		}
		b.WriteString("</ul>\n</div>\n")
	}

	b.WriteString("</body>\n</html>\n")
	return b.String()
}

// inlineCIDs points the message's cid: references at the files that were
// saved next to content.html, so embedded images still show.
func inlineCIDs(body string, atts []attachment) string {
	for _, a := range atts {
		if a.contentID == "" {
			continue
		}
		href := (&url.URL{Path: a.fileName}).EscapedPath()
		for _, form := range []string{
			"cid:" + a.contentID,
			"CID:" + a.contentID,
			"cid:" + url.QueryEscape(a.contentID),
		} {
			body = strings.ReplaceAll(body, form, href)
		}
	}
	return body
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func humanSize(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
