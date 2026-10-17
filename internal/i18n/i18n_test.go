package i18n

import (
	"reflect"
	"regexp"
	"testing"
)

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

// Every string must be translated, and a translation must take the same
// printf arguments in the same order as the Russian original, or the UI
// would print %!d(MISSING) and the like.
func TestTranslationsAreCompleteAndConsistent(t *testing.T) {
	verbs := regexp.MustCompile(`%[-+# 0-9.]*[a-zA-Z%]`)
	base := reflect.ValueOf(*Get(RU))
	for l := Lang(0); int(l) < Count; l++ {
		v := reflect.ValueOf(*Get(l))
		for i := range v.NumField() {
			name := v.Type().Field(i).Name
			got := v.Field(i).String()
			if got == "" {
				t.Errorf("%s: %s is empty", Names[l], name)
				continue
			}
			want := verbs.FindAllString(base.Field(i).String(), -1)
			if have := verbs.FindAllString(got, -1); !reflect.DeepEqual(have, want) {
				t.Errorf("%s: %s uses %v, the Russian original %v", Names[l], name, have, want)
			}
		}
	}
}

func TestCodesRoundTrip(t *testing.T) {
	for l := Lang(0); int(l) < Count; l++ {
		if got, ok := FromCode(l.Code()); !ok || got != l {
			t.Errorf("FromCode(%q) = %v, %v", l.Code(), got, ok)
		}
	}
	if _, ok := FromCode("xx"); ok {
		t.Error("an unknown code must not be accepted")
	}
}
