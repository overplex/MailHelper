package imapc

import (
	"bufio"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

// dialFake wires a Client to a scripted server over an in-memory pipe, which
// exercises the response parsing without a real mailbox.
func dialFake(t *testing.T, script func(t *testing.T, r *bufio.Reader, w *bufio.Writer)) *Client {
	t.Helper()

	ours, theirs := net.Pipe()
	go func() {
		defer theirs.Close()
		theirs.SetDeadline(time.Now().Add(5 * time.Second))
		r := bufio.NewReader(theirs)
		w := bufio.NewWriter(theirs)
		script(t, r, w)
		w.Flush()
	}()

	t.Cleanup(func() { ours.Close() })
	return &Client{
		cfg:  Config{ReadTimeout: 5 * time.Second},
		conn: ours,
		r:    bufio.NewReader(ours),
		w:    bufio.NewWriter(ours),
		caps: map[string]bool{},
	}
}

// respond reads one command line and writes the given reply.
func respond(reply string) func(*testing.T, *bufio.Reader, *bufio.Writer) {
	return func(_ *testing.T, r *bufio.Reader, w *bufio.Writer) {
		if _, err := r.ReadString('\n'); err != nil {
			return
		}
		w.WriteString(reply)
		w.Flush()
	}
}

func TestListDecodesFolderNames(t *testing.T) {
	c := dialFake(t, respond(
		"* LIST (\\HasNoChildren) \"|\" \"INBOX\"\r\n"+
			"* LIST (\\HasNoChildren \\Trash) \"|\" \"&BBoEPgRABDcEOAQ9BDA-\"\r\n"+
			"* LIST (\\Noselect \\HasChildren) \"/\" \"[Gmail]\"\r\n"+
			"A0001 OK LIST completed\r\n"))

	folders, err := c.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(folders) != 3 {
		t.Fatalf("got %d folders, want 3", len(folders))
	}

	if folders[1].Name != "Корзина" {
		t.Errorf("folder name = %q, want Корзина", folders[1].Name)
	}
	if folders[1].Raw != "&BBoEPgRABDcEOAQ9BDA-" {
		t.Errorf("raw name = %q", folders[1].Raw)
	}
	if !folders[1].HasFlag(`\Trash`) {
		t.Errorf("flags %v, want \\Trash", folders[1].Flags)
	}
	if folders[1].Delim != "|" {
		t.Errorf("delimiter = %q, want |", folders[1].Delim)
	}
	if folders[2].Selectable() {
		t.Errorf("[Gmail] is \\Noselect and must not be offered as a folder")
	}
}

func TestListWithLiteralName(t *testing.T) {
	// Some servers send the mailbox name as a literal rather than a quoted
	// string; the response reader has to follow it.
	c := dialFake(t, respond(
		"* LIST (\\HasNoChildren) \"/\" {5}\r\n"+
			"Rabot\r\n"+
			"A0001 OK LIST completed\r\n"))

	folders, err := c.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(folders) != 1 || folders[0].Raw != "Rabot" {
		t.Fatalf("got %#v, want one folder named Rabot", folders)
	}
}

func TestSelectAndSearch(t *testing.T) {
	c := dialFake(t, func(t *testing.T, r *bufio.Reader, w *bufio.Writer) {
		line, _ := r.ReadString('\n')
		if !strings.Contains(line, `SELECT "INBOX"`) {
			t.Errorf("unexpected select command: %q", line)
		}
		w.WriteString("* 17 EXISTS\r\nA0001 OK [READ-WRITE] SELECT completed\r\n")
		w.Flush()

		line, _ = r.ReadString('\n')
		if !strings.Contains(line, "UID SEARCH SINCE 01-Feb-2026 BEFORE 04-Feb-2026") {
			t.Errorf("unexpected search command: %q", line)
		}
		w.WriteString("* SEARCH 3 9 27\r\nA0002 OK SEARCH completed\r\n")
		w.Flush()
	})

	total, err := c.Select("INBOX")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if total != 17 {
		t.Errorf("EXISTS = %d, want 17", total)
	}

	since := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	// The upper bound is inclusive for the user, so BEFORE must be the day
	// after the date they picked.
	until := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)
	uids, err := c.SearchUIDs(&since, &until)
	if err != nil {
		t.Fatalf("SearchUIDs: %v", err)
	}
	if len(uids) != 3 || uids[0] != 3 || uids[2] != 27 {
		t.Errorf("uids = %v", uids)
	}
}

func TestFetchReadsLiteralBody(t *testing.T) {
	body := "Subject: hi\r\n\r\nHello world\r\n"
	c := dialFake(t, respond(
		"* 1 FETCH (UID 42 INTERNALDATE \"08-Sep-2026 17:56:23 +0300\" BODY[] {"+
			strconv.Itoa(len(body))+"}\r\n"+body+")\r\n"+
			"A0001 OK FETCH completed\r\n"))

	msg, err := c.Fetch(42)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(msg.Raw) != body {
		t.Errorf("body = %q", msg.Raw)
	}
	if msg.InternalDate.IsZero() {
		t.Fatal("INTERNALDATE was not parsed")
	}
	if h, m, s := msg.InternalDate.Clock(); h != 17 || m != 56 || s != 23 {
		t.Errorf("INTERNALDATE clock = %02d:%02d:%02d", h, m, s)
	}
}

func TestTaggedNoBecomesServerError(t *testing.T) {
	c := dialFake(t, respond("A0001 NO [AUTHENTICATIONFAILED] Invalid credentials\r\n"))

	err := c.exec("LOGIN \"u\" \"p\"", nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !IsServerError(err) {
		t.Fatalf("error %v is not a ServerError, so the job would retry it", err)
	}
	if !strings.Contains(err.Error(), "AUTHENTICATIONFAILED") {
		t.Errorf("error text = %q", err)
	}
}

func TestUIDSetAndQuoting(t *testing.T) {
	if got := uidSet([]uint32{1, 5, 900}); got != "1,5,900" {
		t.Errorf("uidSet = %q", got)
	}
	if got := quote(`a"b\c`); got != `"a\"b\\c"` {
		t.Errorf("quote = %s", got)
	}
}

func TestMoveIssuesCopyThenDeleteThenExpunge(t *testing.T) {
	var commands []string

	c := dialFake(t, func(_ *testing.T, r *bufio.Reader, w *bufio.Writer) {
		for i := 1; i <= 3; i++ {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			commands = append(commands, strings.TrimSpace(line))
			w.WriteString("A000" + strconv.Itoa(i) + " OK done\r\n")
			w.Flush()
		}
	})
	c.caps["UIDPLUS"] = true

	session := &Session{cfg: Config{Retries: 0}, client: c}
	if err := session.Move([]uint32{3, 9, 27}, "&BBoEPgRABDcEOAQ9BDA-"); err != nil {
		t.Fatalf("Move: %v", err)
	}

	want := []string{
		`A0001 UID COPY 3,9,27 "&BBoEPgRABDcEOAQ9BDA-"`,
		`A0002 UID STORE 3,9,27 +FLAGS.SILENT (\Deleted)`,
		`A0003 UID EXPUNGE 3,9,27`,
	}
	if len(commands) != len(want) {
		t.Fatalf("sent %d commands: %q", len(commands), commands)
	}
	for i := range want {
		if commands[i] != want[i] {
			t.Errorf("command %d = %q, want %q", i+1, commands[i], want[i])
		}
	}
}

func TestMoveStopsOnAServerError(t *testing.T) {
	c := dialFake(t, func(_ *testing.T, r *bufio.Reader, w *bufio.Writer) {
		r.ReadString('\n')
		w.WriteString("A0001 NO [TRYCREATE] Mailbox does not exist\r\n")
		w.Flush()
	})

	session := &Session{cfg: Config{Retries: 3}, client: c}
	err := session.Move([]uint32{1}, "Nowhere")
	if err == nil {
		t.Fatal("expected the run to stop")
	}
	if !IsServerError(err) {
		t.Errorf("error %v should not be retried", err)
	}
}

func TestFolderDisplayNameTranslatesInbox(t *testing.T) {
	cases := []struct {
		name, delim, inbox, want string
	}{
		{"INBOX", "/", "Входящие", "Входящие"},
		{"INBOX", "/", "Inbox", "Inbox"},
		{"inbox", "/", "Inbox", "Inbox"}, // servers may spell it in any case
		{"INBOX/Работа", "/", "Входящие", "Входящие/Работа"},
		{"INBOX.Sent", ".", "Inbox", "Inbox.Sent"},
		{"INBOXES", "/", "Inbox", "INBOXES"}, // a different folder, left alone
		{"INBOX-archive", "/", "Inbox", "INBOX-archive"},
		{"Корзина", "/", "Входящие", "Корзина"},
		{"INBOX", "", "Входящие", "Входящие"}, // a server that reported no delimiter
		{"INBOX", "/", "", "INBOX"},           // no translation supplied
	}
	for _, c := range cases {
		f := Folder{Name: c.name, Delim: c.delim}
		if got := f.DisplayName(c.inbox); got != c.want {
			t.Errorf("Folder{%q, delim %q}.DisplayName(%q) = %q, want %q",
				c.name, c.delim, c.inbox, got, c.want)
		}
	}
}

func TestFolderIsSent(t *testing.T) {
	cases := []struct {
		folder Folder
		want   bool
	}{
		{Folder{Name: "INBOX"}, false},
		{Folder{Name: "Sent"}, true},
		{Folder{Name: "SENT ITEMS"}, true},
		{Folder{Name: "Отправленные"}, true},
		{Folder{Name: "[Gmail]/Отправленные", Delim: "/"}, true},
		{Folder{Name: "Archive/2026", Delim: "/", Flags: []string{`\Sent`}}, true},
		{Folder{Name: "Sent notes"}, false},
	}
	for _, c := range cases {
		if got := c.folder.IsSent(); got != c.want {
			t.Errorf("IsSent(%q, %v) = %v, want %v", c.folder.Name, c.folder.Flags, got, c.want)
		}
	}
}
