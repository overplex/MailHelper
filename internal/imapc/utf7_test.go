package imapc

import "testing"

func TestModifiedUTF7RoundTrip(t *testing.T) {
	for _, name := range []string{
		"INBOX",
		"Корзина",
		"Отправленные",
		"Входящие/Работа",
		"Sent Items",
		"a&b",
		"&",
		"[Gmail]/Вся почта",
		"日本語",
		"",
	} {
		encoded := EncodeMUTF7(name)
		if got := DecodeMUTF7(encoded); got != name {
			t.Errorf("round trip of %q: encoded %q, decoded %q", name, encoded, got)
		}
	}
}

func TestModifiedUTF7Vectors(t *testing.T) {
	cases := []struct{ raw, decoded string }{
		// From RFC 3501 5.1.3.
		{"~peter/mail/&U,BTFw-/&ZeVnLIqe-", "~peter/mail/台北/日本語"},
		{"&-", "&"},
		{"INBOX", "INBOX"},
		{"&BBoEPgRABDcEOAQ9BDA-", "Корзина"},
		// A name a server sent as plain UTF-8 must survive untouched.
		{"Корзина", "Корзина"},
	}
	for _, c := range cases {
		if got := DecodeMUTF7(c.raw); got != c.decoded {
			t.Errorf("DecodeMUTF7(%q) = %q, want %q", c.raw, got, c.decoded)
		}
	}

	if got := EncodeMUTF7("Корзина"); got != "&BBoEPgRABDcEOAQ9BDA-" {
		t.Errorf("EncodeMUTF7(Корзина) = %q", got)
	}
	if got := EncodeMUTF7("a&b"); got != "a&-b" {
		t.Errorf("EncodeMUTF7(a&b) = %q", got)
	}
}

func TestDecodeMUTF7Malformed(t *testing.T) {
	// A broken shift sequence must not swallow the name.
	for _, in := range []string{"&BBo", "&!!!-", "&"} {
		if got := DecodeMUTF7(in); got == "" {
			t.Errorf("DecodeMUTF7(%q) lost the name", in)
		}
	}
}
