package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadInitializesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Frida-GUI-Helper", "settings.json")
	store := NewAt(path)

	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got != Default() {
		t.Fatalf("Load() = %#v, want %#v", got, Default())
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("default settings were not persisted: %v", err)
	}
}

func TestSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store := NewAt(path)
	want := Settings{
		AutoCheckAppUpdates:   false,
		AutoCheckFridaUpdates: true,
		AutoUpdateFrida:       true,
		NotifyWhenAvailable:   false,
	}

	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
	want.AutoCheckFridaUpdates = false
	if err := store.Save(want); err != nil {
		t.Fatalf("second Save() failed: %v", err)
	}
	got, err = store.Load()
	if err != nil || got != want {
		t.Fatalf("Load() after replacement = %#v, %v; want %#v", got, err, want)
	}

	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".settings-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files left behind: %v", matches)
	}
}

func TestLoadMergesMissingFieldsWithDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"autoCheckAppUpdates":false}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := NewAt(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.AutoCheckAppUpdates {
		t.Fatal("explicit false value was not preserved")
	}
	if !got.AutoCheckFridaUpdates {
		t.Fatal("missing Frida setting did not receive its default")
	}
	if got.AutoUpdateFrida {
		t.Fatal("missing auto-update setting did not receive its default")
	}
	if !got.NotifyWhenAvailable {
		t.Fatal("missing notification setting did not receive its default")
	}
}

func TestLoadRecoversCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	bad := []byte(`{"autoCheckAppUpdates":`)
	if err := os.WriteFile(path, bad, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := NewAt(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if got != Default() {
		t.Fatalf("Load() = %#v, want defaults", got)
	}

	restored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(restored), `"autoCheckAppUpdates":`) == false {
		t.Fatalf("restored settings do not contain defaults: %s", restored)
	}
	backups, err := filepath.Glob(path + ".corrupt-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("got %d corrupt backups, want 1", len(backups))
	}
	preserved, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(preserved) != string(bad) {
		t.Fatalf("corrupt backup changed: %q", preserved)
	}
}

func TestNilStoreReturnsError(t *testing.T) {
	var store *Store
	if _, err := store.Load(); err == nil {
		t.Fatal("Load() succeeded for a nil store")
	}
	if err := store.Save(Default()); err == nil {
		t.Fatal("Save() succeeded for a nil store")
	}
	if _, err := NewAt("").Load(); err == nil {
		t.Fatal("Load() succeeded for an empty path")
	}
}
