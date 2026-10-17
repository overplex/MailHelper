package mailsave

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var received = time.Date(2026, 9, 8, 17, 56, 23, 0, time.Local)

func TestMessageDirNameKeepsTheTime(t *testing.T) {
	long := strings.Repeat("длинная тема ", 30)
	name := MessageDirName("Иван Петров", "ivan@example.ru", long, received)

	if n := len([]rune(name)); n > MaxDirNameLen {
		t.Errorf("name is %d runes, over the %d limit: %q", n, MaxDirNameLen, name)
	}
	if !strings.HasSuffix(name, " 17-56-23") {
		t.Errorf("the time must survive truncation, got %q", name)
	}
	if !strings.HasPrefix(name, "Иван Петров ivan@example.ru") {
		t.Errorf("name = %q", name)
	}
}

func TestMessageDirNameDropsForbiddenCharacters(t *testing.T) {
	name := MessageDirName(`Ivan "The Boss"`, "ivan@example.ru", `Re: a/b\c|d?e*f<g>h`, received)

	if strings.ContainsAny(name, forbidden) {
		t.Errorf("name still holds a character Windows forbids: %q", name)
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		t.Errorf("a trailing dot or space would be dropped by Windows: %q", name)
	}
	if !strings.HasSuffix(name, "17-56-23") {
		t.Errorf("name = %q", name)
	}
}

func TestMessageDirNameWithNothingButTheTime(t *testing.T) {
	if got := MessageDirName("", "", "", received); got != "17-56-23" {
		t.Errorf("got %q, want 17-56-23", got)
	}
}

func TestMakeUniqueDirAddsASuffix(t *testing.T) {
	parent := t.TempDir()
	name := MessageDirName("Иван", "ivan@example.ru", "Отчёт", received)

	first, err := MakeUniqueDir(parent, name)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MakeUniqueDir(parent, name)
	if err != nil {
		t.Fatal(err)
	}

	if first == second {
		t.Fatal("the second message overwrote the first")
	}
	if filepath.Base(second) != name+" (2)" {
		t.Errorf("second folder = %q", filepath.Base(second))
	}
	for _, d := range []string{first, second} {
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			t.Errorf("%s was not created", d)
		}
	}
}

func TestMakeUniqueDirKeepsTheSuffixInsideTheLimit(t *testing.T) {
	parent := t.TempDir()
	name := MessageDirName("Иван Петров", "ivan@example.ru",
		strings.Repeat("тема ", 40), received)

	if _, err := MakeUniqueDir(parent, name); err != nil {
		t.Fatal(err)
	}
	second, err := MakeUniqueDir(parent, name)
	if err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(filepath.Base(second))); n > MaxDirNameLen {
		t.Errorf("de-duplicated name is %d runes: %q", n, filepath.Base(second))
	}
}

func TestFileNameCleansAndShortens(t *testing.T) {
	if got := FileName(`..\..\windows\system32\evil.exe`, "fallback.bin"); got != "evil.exe" {
		t.Errorf("path traversal survived: %q", got)
	}
	if got := FileName("", "fallback.bin"); got != "fallback.bin" {
		t.Errorf("empty name = %q", got)
	}
	if got := FileName("CON", "fallback.bin"); got != "_CON" {
		t.Errorf("reserved device name = %q", got)
	}

	long := FileName(strings.Repeat("отчёт", 40)+".pdf", "fallback.bin")
	if !strings.HasSuffix(long, ".pdf") {
		t.Errorf("the extension must survive shortening: %q", long)
	}
	if n := len([]rune(long)); n > 100 {
		t.Errorf("shortened name is %d runes", n)
	}
}
