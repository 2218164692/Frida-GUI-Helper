package logstream

import (
	"testing"

	"frida-gui-helper/internal/diagnostics"
)

func TestAddWithDiagnostic(t *testing.T) {
	stream := New(10, nil)
	finding := diagnostics.Finding{Code: "spawn-timeout", Title: "Spawn timeout"}
	entry := stream.AddWithDiagnostic(LevelError, "frida", "failed", &finding)
	if entry.Diagnostic == nil || entry.Diagnostic.Code != "spawn-timeout" {
		t.Fatalf("diagnostic = %#v", entry.Diagnostic)
	}
	entries := stream.Entries()
	if len(entries) != 1 || entries[0].Diagnostic == nil {
		t.Fatalf("entries = %#v", entries)
	}
}
