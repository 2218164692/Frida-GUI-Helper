package profiles

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCRUDAndDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Frida-GUI-Helper", "profiles.json")
	store := NewAt(path)
	fixed := time.Date(2026, 8, 4, 8, 30, 0, 0, time.UTC)
	store.now = func() time.Time { return fixed }

	saved, err := store.Save(SaveRequest{
		DeviceSerial: " device-01 ",
		PackageName:  " com.example.app ",
		ScriptName:   " trace.js ",
		ScriptSource: "const packageName = {{packageName}};\n",
		Variables:    map[string]string{"packageName": "com.example.app"},
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if saved.ID == "" {
		t.Fatal("Save() did not assign an ID")
	}
	if saved.Name != "com.example.app" {
		t.Fatalf("Name = %q, want derived package name", saved.Name)
	}
	if saved.Mode != "attach" || saved.TargetKind != "name" {
		t.Fatalf("defaults = mode %q, target kind %q", saved.Mode, saved.TargetKind)
	}
	if saved.Target != "com.example.app" {
		t.Fatalf("Target = %q, want package name", saved.Target)
	}
	if saved.ServerPath != defaultServerPath {
		t.Fatalf("ServerPath = %q, want %q", saved.ServerPath, defaultServerPath)
	}
	if saved.CreatedAt != fixed.Format(time.RFC3339) || saved.UpdatedAt != fixed.Format(time.RFC3339) {
		t.Fatalf("timestamps = %q / %q", saved.CreatedAt, saved.UpdatedAt)
	}

	// Returned maps are detached from the store.
	saved.Variables["packageName"] = "mutated"
	got, err := store.Get(saved.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Variables["packageName"] != "com.example.app" {
		t.Fatalf("stored variables were mutated: %#v", got.Variables)
	}

	later := fixed.Add(time.Hour)
	store.now = func() time.Time { return later }
	updated, err := store.Save(SaveRequest{
		ID:           saved.ID,
		Name:         "Example profile",
		DeviceSerial: "device-02",
		PackageName:  "com.example.app",
		Target:       "com.example.app",
		Mode:         "spawn",
		TargetKind:   "name",
		ScriptName:   "trace.js",
		ScriptSource: "console.log('updated');",
	})
	if err != nil {
		t.Fatalf("Save(update) error = %v", err)
	}
	if updated.CreatedAt != fixed.Format(time.RFC3339) {
		t.Fatalf("CreatedAt changed to %q", updated.CreatedAt)
	}
	if updated.UpdatedAt != later.Format(time.RFC3339) {
		t.Fatalf("UpdatedAt = %q", updated.UpdatedAt)
	}
	if updated.TargetKind != "package" {
		t.Fatalf("spawn TargetKind = %q, want package", updated.TargetKind)
	}

	listed, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed) != 1 || listed[0].Name != "Example profile" {
		t.Fatalf("List() = %#v", listed)
	}

	if err := store.Delete(saved.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.Get(saved.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(deleted) error = %v, want ErrNotFound", err)
	}
	if err := store.Delete(saved.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(missing) error = %v, want ErrNotFound", err)
	}

	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".profiles-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files left behind: %v", matches)
	}
}

func TestLastAndMarkRun(t *testing.T) {
	store := NewAt(filepath.Join(t.TempDir(), "profiles.json"))
	start := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return start }

	first, err := store.Save(SaveRequest{Name: "First"})
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return start.Add(time.Minute) }
	second, err := store.Save(SaveRequest{Name: "Second"})
	if err != nil {
		t.Fatal(err)
	}
	last, err := store.Last()
	if err != nil || last != nil {
		t.Fatalf("Last() before MarkRun = %#v, %v; want nil", last, err)
	}

	runAt := start.Add(2 * time.Minute)
	store.now = func() time.Time { return runAt }
	marked, err := store.MarkRun(first.ID)
	if err != nil {
		t.Fatalf("MarkRun() error = %v", err)
	}
	if marked.LastRunAt != runAt.Format(time.RFC3339) {
		t.Fatalf("LastRunAt = %q", marked.LastRunAt)
	}
	last, err = store.Last()
	if err != nil {
		t.Fatalf("Last() error = %v", err)
	}
	if last == nil || last.ID != first.ID {
		t.Fatalf("Last() = %#v, want %s", last, first.ID)
	}

	listed, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].ID != first.ID || listed[1].ID != second.ID {
		t.Fatalf("List() order = %#v", listed)
	}

	if err := store.Delete(first.ID); err != nil {
		t.Fatal(err)
	}
	last, err = store.Last()
	if err != nil || last != nil {
		t.Fatalf("Last() after deleting last profile = %#v, %v", last, err)
	}
	if _, err := store.MarkRun("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("MarkRun(missing) error = %v, want ErrNotFound", err)
	}
}

func TestCorruptFileIsPreservedAndReset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")
	bad := []byte(`{"version":1,"profiles":[`)
	if err := os.WriteFile(path, bad, 0o600); err != nil {
		t.Fatal(err)
	}

	store := NewAt(path)
	store.now = func() time.Time {
		return time.Date(2026, 8, 4, 10, 0, 0, 123, time.UTC)
	}
	listed, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("List() = %#v, want empty", listed)
	}

	backups, err := filepath.Glob(path + ".corrupt-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("corrupt backups = %v, want one", backups)
	}
	preserved, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(preserved) != string(bad) {
		t.Fatalf("preserved corrupt data = %q", preserved)
	}
	restored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(restored), `"version": 1`) || !strings.Contains(string(restored), `"profiles": []`) {
		t.Fatalf("restored file is not an empty valid store: %s", restored)
	}
}

func TestMissingPrimaryRecoversPreviousFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	store := NewAt(path)
	saved, err := store.Save(SaveRequest{Name: "Recover me"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".previous"); err != nil {
		t.Fatal(err)
	}

	reopened := NewAt(path)
	listed, err := reopened.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed) != 1 || listed[0].ID != saved.ID {
		t.Fatalf("recovered profiles = %#v", listed)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("primary file was not restored: %v", err)
	}
	if _, err := os.Stat(path + ".previous"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("previous file still exists or stat failed: %v", err)
	}
}

func TestFutureStoreVersionIsNotQuarantined(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")
	future := []byte(`{"version":999,"profiles":[]}`)
	if err := os.WriteFile(path, future, 0o600); err != nil {
		t.Fatal(err)
	}

	store := NewAt(path)
	if _, err := store.List(); !errors.Is(err, errUnsupportedStoreVersion) {
		t.Fatalf("List() error = %v, want unsupported version", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(future) {
		t.Fatalf("future store was modified: %q", got)
	}
	backups, err := filepath.Glob(path + ".corrupt-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 0 {
		t.Fatalf("future store was quarantined: %v", backups)
	}
}

func TestConcurrentSavesAreSerialized(t *testing.T) {
	store := NewAt(filepath.Join(t.TempDir(), "profiles.json"))
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.Save(SaveRequest{Name: "Concurrent"}); err != nil {
				t.Errorf("Save() error = %v", err)
			}
		}()
	}
	wg.Wait()
	listed, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 20 {
		t.Fatalf("List() count = %d, want 20", len(listed))
	}
}

func TestValidationAndEmptyStore(t *testing.T) {
	var nilStore *Store
	if _, err := nilStore.List(); err == nil {
		t.Fatal("List() succeeded for nil store")
	}
	if _, err := NewAt("").Save(SaveRequest{Name: "test"}); err == nil {
		t.Fatal("Save() succeeded for empty path")
	}
	store := NewAt(filepath.Join(t.TempDir(), "profiles.json"))
	if _, err := store.Save(SaveRequest{Name: "bad", Mode: "invalid"}); err == nil {
		t.Fatal("Save() accepted an invalid mode")
	}
	if _, err := store.Save(SaveRequest{Name: "bad", TargetKind: "invalid"}); err == nil {
		t.Fatal("Save() accepted an invalid target kind")
	}
	if _, err := store.Save(SaveRequest{ID: "does-not-exist", Name: "bad"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Save(update missing) error = %v, want ErrNotFound", err)
	}
}
