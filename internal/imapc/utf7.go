package imapc

import (
	"encoding/base64"
	"strings"
	"unicode/utf16"
)

// IMAP mailbox names travel over the wire in "modified UTF-7" (RFC 3501
// 5.1.3): printable US-ASCII stays as it is, everything else is base64 of
// UTF-16BE wrapped in '&' ... '-', using ',' in place of base64's '/'.
// Skipping this step is exactly what turns Russian folder names into
// mojibake in the UI, so every name is decoded on the way in and encoded
// again on the way out.

const modifiedBase64Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+,"

var modifiedBase64 = base64.NewEncoding(modifiedBase64Alphabet).WithPadding(base64.NoPadding)

// EncodeMUTF7 converts a mailbox name into the form the server expects.
func EncodeMUTF7(s string) string {
	var out strings.Builder
	var shifted []uint16 // the run of non-ASCII runes waiting to be emitted

	flush := func() {
		if len(shifted) == 0 {
			return
		}
		buf := make([]byte, 0, len(shifted)*2)
		for _, u := range shifted {
			buf = append(buf, byte(u>>8), byte(u))
		}
		out.WriteByte('&')
		out.WriteString(modifiedBase64.EncodeToString(buf))
		out.WriteByte('-')
		shifted = shifted[:0]
	}

	for _, r := range s {
		switch {
		case r == '&':
			flush()
			out.WriteString("&-")
		case r >= 0x20 && r <= 0x7E:
			flush()
			out.WriteRune(r)
		default:
			shifted = append(shifted, utf16.Encode([]rune{r})...)
		}
	}
	flush()
	return out.String()
}

// DecodeMUTF7 converts a mailbox name received from the server into UTF-8.
// A malformed shift sequence is passed through verbatim rather than
// discarded, so a name is never lost to a server that got the encoding
// wrong; names already sent as UTF-8 (servers running with UTF8=ACCEPT)
// survive untouched as well.
func DecodeMUTF7(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}

	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '&' {
			out.WriteByte(s[i])
			i++
			continue
		}

		end := strings.IndexByte(s[i+1:], '-')
		if end < 0 { // unterminated shift, keep the rest as it is
			out.WriteString(s[i:])
			break
		}
		segment := s[i+1 : i+1+end]
		i += end + 2

		if segment == "" { // "&-" is a literal ampersand
			out.WriteByte('&')
			continue
		}

		raw, err := modifiedBase64.DecodeString(segment)
		if err != nil || len(raw)%2 != 0 {
			out.WriteString("&" + segment + "-")
			continue
		}
		units := make([]uint16, len(raw)/2)
		for j := range units {
			units[j] = uint16(raw[j*2])<<8 | uint16(raw[j*2+1])
		}
		out.WriteString(string(utf16.Decode(units)))
	}
	return out.String()
}
