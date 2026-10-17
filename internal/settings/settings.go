// Package settings keeps the user's choices between runs. They live in
// settings.json inside a MailHelper folder of the user's configuration
// directory - %AppData%\MailHelper on Windows. The password is never part of
// them.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// AppDirName is the folder created inside the user's configuration directory.
const AppDirName = "MailHelper"

const fileName = "settings.json"

// Settings is everything that survives a restart.
type Settings struct {
	Language string `json:"language,omitempty"`
	Theme    string `json:"theme,omitempty"`

	Email    string `json:"email,omitempty"`
	Server   string `json:"server,omitempty"`
	Port     int    `json:"port,omitempty"`
	Username string `json:"username,omitempty"`

	SaveDir       string `json:"saveDir,omitempty"`
	MoveAfterCopy bool   `json:"moveAfterCopy,omitempty"`

	// Folder selections, as the raw (modified UTF-7) names the server uses.
	// They are applied once the folder list has been fetched.
	CopySource string `json:"copySource,omitempty"`
	CopyTarget string `json:"copyTarget,omitempty"`
	MoveSource string `json:"moveSource,omitempty"`
	MoveTarget string `json:"moveTarget,omitempty"`
}

// Path returns the location of the settings file.
func Path() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate the configuration directory: %w", err)
	}
	return filepath.Join(base, AppDirName, fileName), nil
}

// Load reads the settings file. found is false when there is none yet, which
// is not an error: the caller starts from its defaults.
func Load() (s Settings, found bool, err error) {
	path, err := Path()
	if err != nil {
		return Settings{}, false, err
	}
	return LoadFile(path)
}

// Save writes the settings file, creating its folder when needed.
func Save(s Settings) error {
	path, err := Path()
	if err != nil {
		return err
	}
	return SaveFile(path, s)
}

// LoadFile reads settings from an explicit path.
func LoadFile(path string) (s Settings, found bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Settings{}, false, nil
	}
	if err != nil {
		return Settings{}, false, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return Settings{}, true, fmt.Errorf("%s: %w", path, err)
	}
	return s, true, nil
}

// SaveFile writes settings to an explicit path. The data goes to a temporary
// file first and is renamed into place, so a crash halfway through never
// leaves a truncated file behind.
func SaveFile(path string, s Settings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, fileName+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename has succeeded

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
