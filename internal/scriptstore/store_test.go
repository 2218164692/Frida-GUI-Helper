package scriptstore

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSaveListRecordRunAndDelete(t *testing.T) {
	store := NewAt(filepath.Join(t.TempDir(), "scripts.json"))
	fixed := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return fixed }

	saved, err := store.Save(SaveRequest{
		Name:        "SSL bypass",
		Description: "test script",
		Tags:        []string{"ssl", "#android", "ssl"},
		Favorite:    true,
		Source:      "console.log('ok');",
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if saved.ID == "" {
		t.Fatal("Save() did not assign an ID")
	}
	if len(saved.Tags) != 2 || saved.Tags[0] != "ssl" || saved.Tags[1] != "android" {
		t.Fatalf("Save() tags = %#v", saved.Tags)
	}

	listed, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed) != 1 || listed[0].Name != "SSL bypass" {
		t.Fatalf("List() = %#v", listed)
	}

	later := fixed.Add(time.Hour)
	store.now = func() time.Time { return later }
	if err := store.RecordRun(saved.ID); err != nil {
		t.Fatalf("RecordRun() error = %v", err)
	}
	listed, err = store.List()
	if err != nil {
		t.Fatalf("List() after RecordRun error = %v", err)
	}
	if listed[0].LastUsedAt != later.Format(time.RFC3339) {
		t.Fatalf("LastUsedAt = %q", listed[0].LastUsedAt)
	}

	if err := store.Delete(saved.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	listed, err = store.List()
	if err != nil {
		t.Fatalf("List() after Delete error = %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("List() after Delete = %#v", listed)
	}
}
