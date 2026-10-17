package mailsave

import (
	"bytes"
	"io"
	"mime"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/encoding/ianaindex"
)

// Mail written in Russian still arrives in windows-1251 or KOI8-R far more
// often than the specification would suggest, and a header can name any
// charset at all. Everything is converted to UTF-8 here, once, so the rest
// of the package only ever handles UTF-8.

// lookup resolves a charset name to a decoder. It returns nil when the name
// is unknown or already UTF-8.
func lookup(name string) encoding.Encoding {
	name = strings.TrimSpace(strings.Trim(name, `"'`))
	if name == "" {
		return nil
	}
	switch strings.ToLower(name) {
	case "utf-8", "utf8", "us-ascii", "ascii", "ansi_x3.4-1968", "default":
		return nil
	}
	if enc, err := htmlindex.Get(name); err == nil && enc != nil {
		return enc
	}
	if enc, err := ianaindex.MIME.Encoding(name); err == nil && enc != nil {
		return enc
	}
	if enc, err := ianaindex.IANA.Encoding(name); err == nil && enc != nil {
		return enc
	}
	return nil
}

// charsetReader is the hook net/mail and mime.WordDecoder use for any
// charset they do not handle themselves.
func charsetReader(label string, input io.Reader) (io.Reader, error) {
	if enc := lookup(label); enc != nil {
		return enc.NewDecoder().Reader(input), nil
	}
	return input, nil
}

// toUTF8 converts a decoded body part to UTF-8. An unknown charset falls
// back to windows-1251 for byte sequences that are not valid UTF-8, since
// that is by far the most likely origin of such a message here, and to the
// bytes as they stand otherwise.
func toUTF8(data []byte, charset string) string {
	if enc := lookup(charset); enc != nil {
		if out, err := enc.NewDecoder().Bytes(data); err == nil {
			return string(out)
		}
	}
	if utf8.Valid(data) {
		return string(data)
	}
	if enc := lookup("windows-1251"); enc != nil {
		if out, err := enc.NewDecoder().Bytes(data); err == nil {
			return string(out)
		}
	}
	return string(bytes.ToValidUTF8(data, []byte("�")))
}

// wordDecoder decodes RFC 2047 encoded words ("=?koi8-r?B?...?=") in header
// values, using the same charset table.
var wordDecoder = &mime.WordDecoder{CharsetReader: charsetReader}

// decodeHeader turns a raw header value into readable UTF-8 text.
func decodeHeader(v string) string {
	if v == "" {
		return ""
	}
	if out, err := wordDecoder.DecodeHeader(v); err == nil {
		return strings.TrimSpace(out)
	}
	// A malformed encoded word is better shown raw than dropped.
	return strings.TrimSpace(v)
}
