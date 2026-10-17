// Package imapc is a small IMAP4rev1 client covering exactly what
// MailHelper needs: listing folders, searching by internal date, fetching
// whole messages, and the COPY + \Deleted + EXPUNGE sequence that makes a
// move. It speaks the protocol directly so that mailbox names can be
// converted between modified UTF-7 and UTF-8 at both boundaries.
package imapc

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Defaults for the connection settings the UI does not expose.
const (
	DefaultConnectTimeout = 30 * time.Second
	DefaultReadTimeout    = 90 * time.Second
	DefaultRetries        = 3
	retryDelay            = 2 * time.Second

	// A sanity bound on a single literal, so a confused server cannot make
	// the process allocate without limit.
	maxLiteral = 256 << 20
)

// Config describes how to reach the mailbox.
type Config struct {
	Server         string
	Port           int
	Username       string
	Password       string
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration
	Retries        int
}

func (cfg *Config) applyDefaults() {
	if cfg.Port == 0 {
		cfg.Port = 993
	}
	if cfg.ConnectTimeout <= 0 {
		cfg.ConnectTimeout = DefaultConnectTimeout
	}
	if cfg.ReadTimeout <= 0 {
		cfg.ReadTimeout = DefaultReadTimeout
	}
	if cfg.Retries < 0 {
		cfg.Retries = 0
	}
}

// ServerError is a tagged NO or BAD reply: the server understood us and
// refused. It is never worth retrying, so it stops a running job at once.
type ServerError struct {
	Command  string
	Response string
}

func (e *ServerError) Error() string {
	return fmt.Sprintf("%s: %s", e.Command, e.Response)
}

// IsServerError reports whether err came back from the server rather than
// from the connection. Transport failures are the ones worth retrying.
func IsServerError(err error) bool {
	var se *ServerError
	return errors.As(err, &se)
}

// Folder is one mailbox as the server reported it.
type Folder struct {
	Name  string   // decoded to UTF-8, for the UI
	Raw   string   // modified UTF-7, for the wire
	Delim string   // hierarchy separator, "" when the server said NIL
	Flags []string // \HasChildren, \Trash, \Noselect ...
}

// Selectable reports whether the folder can hold messages. Servers use
// \Noselect for pure hierarchy nodes such as Gmail's "[Gmail]".
func (f Folder) Selectable() bool {
	for _, fl := range f.Flags {
		if strings.EqualFold(fl, `\Noselect`) || strings.EqualFold(fl, `\NonExistent`) {
			return false
		}
	}
	return true
}

// DisplayName is the folder name as it should be shown to a user. INBOX is
// fixed by the protocol rather than chosen by the owner of the mailbox, so
// the caller passes the word to put in its place - including where INBOX is
// the parent of a hierarchy, as in INBOX/Работа. Every other name belongs to
// the mailbox and is shown exactly as it came.
func (f Folder) DisplayName(inbox string) string {
	const name = "INBOX"
	if inbox == "" || len(f.Name) < len(name) || !strings.EqualFold(f.Name[:len(name)], name) {
		return f.Name
	}

	rest := f.Name[len(name):]
	switch {
	case rest == "":
		return inbox
	case f.Delim != "" && strings.HasPrefix(rest, f.Delim):
		return inbox + rest
	default:
		return f.Name // a different folder that merely starts with those letters
	}
}

// HasFlag reports whether the folder carries the given (case-insensitive)
// attribute, which is how special-use folders such as \Trash are found.
func (f Folder) HasFlag(name string) bool {
	for _, fl := range f.Flags {
		if strings.EqualFold(fl, name) {
			return true
		}
	}
	return false
}

// sentNames are the mailbox names servers give the folder of outgoing mail,
// in the languages this program is likely to meet. They are the fallback for
// a server that reports no \Sent attribute.
var sentNames = map[string]bool{
	"sent": true, "sent items": true, "sent mail": true, "sent messages": true,
	"sentmail": true, "sentbox": true, "outbox": true,
	"отправленные": true, "отправленные письма": true, "исходящие": true,
	"отправленная почта": true,
}

// IsSent reports whether the folder holds mail the owner of the mailbox
// sent, rather than received. The \Sent special-use attribute is the
// reliable answer; where the server does not give one, the last segment of
// the path is matched against the usual names.
func (f Folder) IsSent() bool {
	if f.HasFlag(`\Sent`) {
		return true
	}
	leaf := f.Name
	if f.Delim != "" {
		if i := strings.LastIndex(leaf, f.Delim); i >= 0 {
			leaf = leaf[i+len(f.Delim):]
		}
	}
	return sentNames[strings.ToLower(strings.TrimSpace(leaf))]
}

// Message is one fetched message: the raw RFC 5322 bytes plus the date the
// server recorded when it arrived.
type Message struct {
	UID          uint32
	InternalDate time.Time
	Raw          []byte
}

// ------------------------------------------------------------ Connection

// Client is a single IMAP connection. It is not safe for concurrent use;
// MailHelper drives it from one worker goroutine at a time.
type Client struct {
	cfg  Config
	conn net.Conn
	r    *bufio.Reader
	w    *bufio.Writer
	tag  int
	caps map[string]bool
}

// Dial opens the connection, negotiates TLS and logs in.
func Dial(cfg Config) (*Client, error) {
	cfg.applyDefaults()

	addr := net.JoinHostPort(cfg.Server, strconv.Itoa(cfg.Port))
	dialer := &net.Dialer{Timeout: cfg.ConnectTimeout}

	var conn net.Conn
	var err error
	if cfg.Port == 143 {
		// Plain submission port: upgrade with STARTTLS below.
		conn, err = dialer.Dial("tcp", addr)
	} else {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
			ServerName: cfg.Server,
			MinVersion: tls.VersionTLS12,
		})
	}
	if err != nil {
		return nil, err
	}

	c := &Client{
		cfg:  cfg,
		conn: conn,
		r:    bufio.NewReaderSize(conn, 64*1024),
		w:    bufio.NewWriter(conn),
		caps: map[string]bool{},
	}

	greeting, _, err := c.readResponse()
	if err != nil {
		conn.Close()
		return nil, err
	}
	up := strings.ToUpper(greeting)
	if !strings.HasPrefix(up, "* OK") && !strings.HasPrefix(up, "* PREAUTH") {
		conn.Close()
		return nil, &ServerError{Command: "GREETING", Response: greeting}
	}
	c.noteCapabilities(greeting)

	if cfg.Port == 143 {
		if err := c.startTLS(); err != nil {
			conn.Close()
			return nil, err
		}
	}

	if len(c.caps) == 0 {
		if err := c.Capability(); err != nil {
			c.conn.Close()
			return nil, err
		}
	}
	if err := c.login(); err != nil {
		c.conn.Close()
		return nil, err
	}
	// Authentication usually changes the advertised capability set.
	_ = c.Capability()
	return c, nil
}

// Close drops the connection without waiting for a clean logout.
func (c *Client) Close() {
	if c == nil || c.conn == nil {
		return
	}
	c.conn.Close()
	c.conn = nil
}

// Logout ends the session politely, then closes the socket.
func (c *Client) Logout() {
	if c == nil || c.conn == nil {
		return
	}
	_ = c.exec("LOGOUT", nil)
	c.Close()
}

func (c *Client) startTLS() error {
	if err := c.exec("STARTTLS", nil); err != nil {
		return err
	}
	tlsConn := tls.Client(c.conn, &tls.Config{
		ServerName: c.cfg.Server,
		MinVersion: tls.VersionTLS12,
	})
	_ = c.conn.SetDeadline(time.Now().Add(c.cfg.ConnectTimeout))
	if err := tlsConn.Handshake(); err != nil {
		return err
	}
	c.conn = tlsConn
	c.r = bufio.NewReaderSize(tlsConn, 64*1024)
	c.w = bufio.NewWriter(tlsConn)
	c.caps = map[string]bool{}
	return c.Capability()
}

func (c *Client) login() error {
	if c.caps["LOGINDISABLED"] {
		return &ServerError{Command: "LOGIN", Response: "the server does not accept plain LOGIN on this connection"}
	}
	return c.exec("LOGIN "+quote(c.cfg.Username)+" "+quote(c.cfg.Password), nil)
}

// Capability refreshes the advertised capability set.
func (c *Client) Capability() error {
	return c.exec("CAPABILITY", func(text string, _ [][]byte) error {
		c.noteCapabilities(text)
		return nil
	})
}

func (c *Client) noteCapabilities(text string) {
	up := strings.ToUpper(text)
	i := strings.Index(up, "CAPABILITY")
	if i < 0 {
		return
	}
	rest := up[i+len("CAPABILITY"):]
	if j := strings.IndexByte(rest, ']'); j >= 0 {
		rest = rest[:j]
	}
	for _, f := range strings.Fields(rest) {
		c.caps[f] = true
	}
}

// Has reports whether the server advertised a capability.
func (c *Client) Has(name string) bool { return c.caps[strings.ToUpper(name)] }

// ------------------------------------------------------------- Protocol

var literalRe = regexp.MustCompile(`\{(\d+)\+?\}$`)

func (c *Client) setDeadline() {
	if c.conn != nil {
		_ = c.conn.SetDeadline(time.Now().Add(c.cfg.ReadTimeout))
	}
}

func (c *Client) readLine() (string, error) {
	c.setDeadline()
	line, err := c.r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// readN reads exactly n bytes, refreshing the deadline every chunk so that a
// large but steadily arriving message body is not cut off by the read
// timeout meant for a stalled connection.
func (c *Client) readN(n int) ([]byte, error) {
	buf := make([]byte, n)
	for off := 0; off < n; {
		end := off + 64*1024
		if end > n {
			end = n
		}
		c.setDeadline()
		got, err := io.ReadFull(c.r, buf[off:end])
		off += got
		if err != nil {
			return nil, err
		}
	}
	return buf, nil
}

// readResponse reads one complete server response, following IMAP literals.
// The returned text keeps each "{n}" marker in place, and the literal
// payloads come back separately in the order they appeared.
func (c *Client) readResponse() (string, [][]byte, error) {
	var text strings.Builder
	var lits [][]byte

	for {
		line, err := c.readLine()
		if err != nil {
			return "", nil, err
		}
		text.WriteString(line)

		m := literalRe.FindStringSubmatch(line)
		if m == nil {
			return text.String(), lits, nil
		}
		n, err := strconv.Atoi(m[1])
		if err != nil || n < 0 || n > maxLiteral {
			return "", nil, fmt.Errorf("bad literal size %q", m[1])
		}
		payload, err := c.readN(n)
		if err != nil {
			return "", nil, err
		}
		lits = append(lits, payload)
	}
}

// exec sends one command and pumps responses until its tagged completion.
// Untagged responses go to the handler; a handler error is remembered and
// returned only after the tagged reply has been consumed, so the connection
// is always left at a response boundary.
func (c *Client) exec(cmd string, untagged func(text string, lits [][]byte) error) error {
	if c.conn == nil {
		return errors.New("connection is closed")
	}

	c.tag++
	tag := fmt.Sprintf("A%04d", c.tag)

	c.setDeadline()
	if _, err := c.w.WriteString(tag + " " + cmd + "\r\n"); err != nil {
		return err
	}
	if err := c.w.Flush(); err != nil {
		return err
	}

	name := cmd
	if i := strings.IndexByte(name, ' '); i > 0 {
		name = name[:i]
	}

	var handlerErr error
	for {
		text, lits, err := c.readResponse()
		if err != nil {
			return err
		}

		if strings.HasPrefix(text, tag+" ") {
			result := strings.TrimSpace(text[len(tag)+1:])
			if strings.HasPrefix(strings.ToUpper(result), "OK") {
				return handlerErr
			}
			return &ServerError{Command: name, Response: result}
		}
		if strings.HasPrefix(text, "+") {
			continue // a continuation request we never asked for
		}
		if untagged != nil && handlerErr == nil {
			handlerErr = untagged(text, lits)
		}
	}
}

// quote renders a string as an IMAP quoted string. Mailbox names reach this
// point already in modified UTF-7, so they are pure ASCII.
func quote(s string) string {
	s = strings.NewReplacer("\r", "", "\n", "").Replace(s)
	s = strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
	return `"` + s + `"`
}

// ------------------------------------------------------------- Commands

var listRe = regexp.MustCompile(`(?i)^\* (LIST|LSUB) `)

// List returns every mailbox, with names decoded for display.
func (c *Client) List() ([]Folder, error) {
	var out []Folder
	err := c.exec(`LIST "" "*"`, func(text string, lits [][]byte) error {
		loc := listRe.FindStringIndex(text)
		if loc == nil {
			return nil
		}
		sc := &scanner{s: text[loc[1]:], lits: lits}

		flagGroup, ok := sc.token()
		if !ok {
			return nil
		}
		delim, ok := sc.token()
		if !ok {
			return nil
		}
		if strings.EqualFold(delim, "NIL") {
			delim = ""
		}
		raw, ok := sc.token()
		if !ok || raw == "" {
			return nil
		}

		out = append(out, Folder{
			Name:  DecodeMUTF7(raw),
			Raw:   raw,
			Delim: delim,
			Flags: strings.Fields(strings.Trim(flagGroup, "()")),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

var existsRe = regexp.MustCompile(`^\* (\d+) EXISTS`)

// Select opens a mailbox for reading and writing and reports how many
// messages it holds. raw must already be modified UTF-7.
func (c *Client) Select(raw string) (int, error) {
	total := 0
	err := c.exec("SELECT "+quote(raw), func(text string, _ [][]byte) error {
		if m := existsRe.FindStringSubmatch(text); m != nil {
			total, _ = strconv.Atoi(m[1])
		}
		return nil
	})
	return total, err
}

// SearchUIDs lists the UIDs whose internal date falls in [since, until].
// A nil bound means that end is not filtered. Both bounds are inclusive,
// which is why the upper one is sent as BEFORE the following day: IMAP's
// BEFORE is exclusive.
func (c *Client) SearchUIDs(since, until *time.Time) ([]uint32, error) {
	criteria := []string{}
	if since != nil {
		criteria = append(criteria, "SINCE "+since.Format("02-Jan-2006"))
	}
	if until != nil {
		criteria = append(criteria, "BEFORE "+until.AddDate(0, 0, 1).Format("02-Jan-2006"))
	}
	if len(criteria) == 0 {
		criteria = append(criteria, "ALL")
	}

	var uids []uint32
	err := c.exec("UID SEARCH "+strings.Join(criteria, " "), func(text string, _ [][]byte) error {
		if !strings.HasPrefix(strings.ToUpper(text), "* SEARCH") {
			return nil
		}
		for _, f := range strings.Fields(text[len("* SEARCH"):]) {
			if v, err := strconv.ParseUint(f, 10, 32); err == nil {
				uids = append(uids, uint32(v))
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return uids, nil
}

var internalDateRe = regexp.MustCompile(`INTERNALDATE "([^"]+)"`)

// Fetch downloads one whole message. BODY.PEEK is used rather than BODY so
// that copying to disk does not silently mark the mailbox as read.
func (c *Client) Fetch(uid uint32) (Message, error) {
	msg := Message{UID: uid}
	found := false

	err := c.exec(fmt.Sprintf("UID FETCH %d (INTERNALDATE BODY.PEEK[])", uid),
		func(text string, lits [][]byte) error {
			if !strings.Contains(strings.ToUpper(text), " FETCH ") {
				return nil
			}
			if m := internalDateRe.FindStringSubmatch(text); m != nil {
				if t, err := time.Parse("02-Jan-2006 15:04:05 -0700", m[1]); err == nil {
					msg.InternalDate = t
				}
			}
			body, ok := pickLiteral(text, lits, "BODY[]", "RFC822")
			if !ok {
				return nil
			}
			msg.Raw = body
			found = true
			return nil
		})
	if err != nil {
		return msg, err
	}
	if !found {
		return msg, fmt.Errorf("message %d returned no body", uid)
	}
	return msg, nil
}

// Copy copies the given UIDs into another mailbox.
func (c *Client) Copy(uids []uint32, destRaw string) error {
	if len(uids) == 0 {
		return nil
	}
	return c.exec("UID COPY "+uidSet(uids)+" "+quote(destRaw), nil)
}

// MarkDeleted sets the \Deleted flag on the given UIDs.
func (c *Client) MarkDeleted(uids []uint32) error {
	if len(uids) == 0 {
		return nil
	}
	return c.exec("UID STORE "+uidSet(uids)+` +FLAGS.SILENT (\Deleted)`, nil)
}

// Expunge removes the messages flagged \Deleted. When the server supports
// UIDPLUS the expunge is limited to the UIDs just handled, so messages
// somebody else flagged in the same mailbox are left alone.
func (c *Client) Expunge(uids []uint32) error {
	if !c.Has("UIDPLUS") || len(uids) == 0 {
		return c.exec("EXPUNGE", nil)
	}

	err := c.exec("UID EXPUNGE "+uidSet(uids), nil)
	if err == nil {
		return nil
	}
	if !IsServerError(err) {
		return err // the connection broke; let the caller retry the batch
	}
	// Some servers advertise UIDPLUS and still refuse UID EXPUNGE. Fall back
	// to the plain form rather than failing the job.
	return c.exec("EXPUNGE", nil)
}

func uidSet(uids []uint32) string {
	parts := make([]string, len(uids))
	for i, u := range uids {
		parts[i] = strconv.FormatUint(uint64(u), 10)
	}
	return strings.Join(parts, ",")
}

// pickLiteral returns the literal that belongs to the first of the given
// response items. With a single literal in the response - the normal case
// for a one-message FETCH - the answer is unambiguous.
func pickLiteral(text string, lits [][]byte, keys ...string) ([]byte, bool) {
	if len(lits) == 0 {
		return nil, false
	}
	if len(lits) == 1 {
		return lits[0], true
	}
	for _, key := range keys {
		ki := strings.Index(text, key)
		if ki < 0 {
			continue
		}
		rel := strings.IndexByte(text[ki:], '{')
		if rel < 0 {
			continue
		}
		pos := ki + rel

		index := 0
		for j := range pos {
			if text[j] == '{' && literalRe.MatchString(trailingMarker(text[j:])) {
				index++
			}
		}
		if index < len(lits) {
			return lits[index], true
		}
	}
	return nil, false
}

// trailingMarker isolates a "{n}" run starting at s, so pickLiteral can tell
// a literal marker from a brace inside a header value.
func trailingMarker(s string) string {
	end := strings.IndexByte(s, '}')
	if end < 0 {
		return ""
	}
	return s[:end+1]
}

// ------------------------------------------------------------- scanner

// scanner walks the tokens of one response line: parenthesised groups,
// quoted strings, literal markers (resolved against the payloads that came
// with the response) and bare atoms.
type scanner struct {
	s    string
	i    int
	lits [][]byte
	used int
}

func (sc *scanner) token() (string, bool) {
	for sc.i < len(sc.s) && sc.s[sc.i] == ' ' {
		sc.i++
	}
	if sc.i >= len(sc.s) {
		return "", false
	}

	switch sc.s[sc.i] {
	case '(':
		start, depth := sc.i, 0
		for sc.i < len(sc.s) {
			switch sc.s[sc.i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					sc.i++
					return sc.s[start:sc.i], true
				}
			}
			sc.i++
		}
		return sc.s[start:], true

	case '"':
		sc.i++
		var b strings.Builder
		for sc.i < len(sc.s) {
			switch ch := sc.s[sc.i]; {
			case ch == '\\' && sc.i+1 < len(sc.s):
				b.WriteByte(sc.s[sc.i+1])
				sc.i += 2
			case ch == '"':
				sc.i++
				return b.String(), true
			default:
				b.WriteByte(ch)
				sc.i++
			}
		}
		return b.String(), true

	case '{':
		end := strings.IndexByte(sc.s[sc.i:], '}')
		if end < 0 {
			sc.i = len(sc.s)
			return "", false
		}
		sc.i += end + 1
		if sc.used < len(sc.lits) {
			v := string(sc.lits[sc.used])
			sc.used++
			return v, true
		}
		return "", true

	default:
		start := sc.i
		for sc.i < len(sc.s) && sc.s[sc.i] != ' ' && sc.s[sc.i] != ')' {
			sc.i++
		}
		return sc.s[start:sc.i], true
	}
}
