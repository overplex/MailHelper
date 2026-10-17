package i18n

import "testing"

func TestFromWindowsLangID(t *testing.T) {
	cases := []struct {
		langID uint16
		want   Lang
		what   string
	}{
		{0x0419, RU, "ru-RU"},
		{0x0819, RU, "ru-MD"},
		{0x0409, EN, "en-US"},
		{0x0809, EN, "en-GB"},
		{0x0422, EN, "uk-UA, which we do not translate"},
		{0x0423, EN, "be-BY, which we do not translate"},
		{0x0000, EN, "a system that could not say"},
	}
	for _, c := range cases {
		if got := FromWindowsLangID(c.langID); got != c.want {
			t.Errorf("FromWindowsLangID(%#04x) = %v, want %v (%s)", c.langID, got, c.want, c.what)
		}
	}
}

func TestEveryLanguageHasStrings(t *testing.T) {
	for l := Lang(0); int(l) < Count; l++ {
		if Get(l).AppTitle == "" || Get(l).Start == "" || Get(l).FolderInbox == "" {
			t.Errorf("language %d (%s) has empty strings", l, Names[l])
		}
	}
	// An out-of-range language must degrade rather than panic.
	if Get(Lang(99)) != Get(EN) {
		t.Error("an unknown language should fall back to English")
	}
}
