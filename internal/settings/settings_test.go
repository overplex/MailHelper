package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveThenLoadRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "settings.json")
	want := Settings{
		Language:      "ru",
		Theme:         "dark",
		Email:         "user@example.com",
		Server:        "imap.example.com",
		Port:          993,
		Username:      "user",
		SaveDir:       `C:\Mail`,
		MoveAfterCopy: true,
		CopySource:    "INBOX",
		MoveTarget:    "&BBoEPgRABDcEOAQ9BDA-",
	}

	if err := SaveFile(path, want); err != nil {
		t.Fatalf("SaveFile: %v", err)
	}
	got, found, err := LoadFile(path)
	if err != nil || !found {
		t.Fatalf("LoadFile: found=%v err=%v", found, err)
	}
	if got != want {
		t.Errorf("loaded %+v, want %+v", got, want)
	}

	// Nothing but the settings file may be left in the folder.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("folder holds %d entries, want only the settings file", len(entries))
	}
}

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	_, found, err := LoadFile(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil || found {
		t.Errorf("found=%v err=%v, want neither", found, err)
	}
}

func TestLoadCorruptFileReportsIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadFile(path); err == nil {
		t.Error("a corrupt file should be reported")
	}
}

func TestPathEndsInAppFolder(t *testing.T) {
	path, err := Path()
	if err != nil {
		t.Skipf("no configuration directory here: %v", err)
	}
	if filepath.Base(filepath.Dir(path)) != AppDirName {
		t.Errorf("Path() = %q, want it inside a %s folder", path, AppDirName)
	}
}
