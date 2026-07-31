package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	configDirectory = "Frida-GUI-Helper"
	configFile      = "settings.json"
)

// Settings contains update preferences which are safe to expose to the UI.
type Settings struct {
	AutoCheckAppUpdates   bool `json:"autoCheckAppUpdates"`
	AutoCheckFridaUpdates bool `json:"autoCheckFridaUpdates"`
	AutoUpdateFrida       bool `json:"autoUpdateFrida"`
	NotifyWhenAvailable   bool `json:"notifyWhenUpdateAvailable"`
}

// Default returns the preferences used when no valid settings file exists.
func Default() Settings {
	return Settings{
		AutoCheckAppUpdates:   true,
		AutoCheckFridaUpdates: true,
		AutoUpdateFrida:       false,
		NotifyWhenAvailable:   true,
	}
}

// Store persists settings in the current user's configuration directory.
// Its writes are serialized and committed with an atomic rename.
type Store struct {
	path string
	mu   sync.Mutex
}

// New creates a store at %AppData%/Frida-GUI-Helper/settings.json on Windows.
func New() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("locate user configuration directory: %w", err)
	}
	return NewAt(filepath.Join(dir, configDirectory, configFile)), nil
}

// NewAt creates a store at path. It is useful for portable environments and
// tests; production callers should normally use New.
func NewAt(path string) *Store {
	if strings.TrimSpace(path) == "" {
		return &Store{}
	}
	return &Store{path: filepath.Clean(path)}
}

// Path returns the settings file location.
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// Load reads the saved preferences. A missing file is initialized with the
// defaults. A malformed file is preserved next to the active file with a
// .corrupt-* suffix before defaults are restored.
func (s *Store) Load() (Settings, error) {
	if s == nil || s.path == "" {
		return Settings{}, errors.New("settings store path is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		defaults := Default()
		if err := s.saveLocked(defaults); err != nil {
			return Settings{}, err
		}
		return defaults, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("read settings: %w", err)
	}

	value, err := decode(data)
	if err == nil {
		return value, nil
	}

	if recoverErr := s.recoverLocked(); recoverErr != nil {
		return Settings{}, fmt.Errorf("recover invalid settings (%v): %w", err, recoverErr)
	}
	return Default(), nil
}

// Save atomically replaces the settings file.
func (s *Store) Save(value Settings) error {
	if s == nil || s.path == "" {
		return errors.New("settings store path is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(value)
}

type diskSettings struct {
	AutoCheckAppUpdates   *bool `json:"autoCheckAppUpdates"`
	AutoCheckFridaUpdates *bool `json:"autoCheckFridaUpdates"`
	AutoUpdateFrida       *bool `json:"autoUpdateFrida"`
	NotifyWhenAvailable   *bool `json:"notifyWhenUpdateAvailable"`
}

func decode(data []byte) (Settings, error) {
	var saved diskSettings
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&saved); err != nil {
		return Settings{}, fmt.Errorf("decode settings: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return Settings{}, fmt.Errorf("decode settings: %w", err)
	}

	value := Default()
	if saved.AutoCheckAppUpdates != nil {
		value.AutoCheckAppUpdates = *saved.AutoCheckAppUpdates
	}
	if saved.AutoCheckFridaUpdates != nil {
		value.AutoCheckFridaUpdates = *saved.AutoCheckFridaUpdates
	}
	if saved.AutoUpdateFrida != nil {
		value.AutoUpdateFrida = *saved.AutoUpdateFrida
	}
	if saved.NotifyWhenAvailable != nil {
		value.NotifyWhenAvailable = *saved.NotifyWhenAvailable
	}
	return value, nil
}

func (s *Store) saveLocked(value Settings) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}

	temp, err := os.CreateTemp(dir, ".settings-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary settings file: %w", err)
	}
	tempPath := temp.Name()
	keepTemp := true
	defer func() {
		if keepTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("secure temporary settings file: %w", err)
	}
	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		_ = temp.Close()
		return fmt.Errorf("encode settings: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("flush settings: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close settings: %w", err)
	}
	backupPath := s.path + ".previous"
	_ = os.Remove(backupPath)
	hadOriginal := false
	if _, err := os.Stat(s.path); err == nil {
		if err := os.Rename(s.path, backupPath); err != nil {
			return fmt.Errorf("stage previous settings: %w", err)
		}
		hadOriginal = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect previous settings: %w", err)
	}
	if err := os.Rename(tempPath, s.path); err != nil {
		if hadOriginal {
			_ = os.Rename(backupPath, s.path)
		}
		return fmt.Errorf("replace settings: %w", err)
	}
	if hadOriginal {
		_ = os.Remove(backupPath)
	}
	keepTemp = false
	return nil
}

func (s *Store) recoverLocked() error {
	backup := fmt.Sprintf("%s.corrupt-%s", s.path, time.Now().UTC().Format("20060102T150405.000000000Z"))
	if err := os.Rename(s.path, backup); err != nil {
		return fmt.Errorf("preserve corrupt settings: %w", err)
	}
	if err := s.saveLocked(Default()); err != nil {
		return err
	}
	return nil
}
