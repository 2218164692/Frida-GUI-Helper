package main

import (
	"testing"

	"frida-gui-helper/internal/adb"
)

func TestProcessMatchesTarget(t *testing.T) {
	process := adb.Process{PID: 2718, Name: "com.example.app:worker", Package: "com.example.app"}
	tests := []struct {
		name   string
		kind   string
		target string
		want   bool
	}{
		{"pid", "pid", "2718", true},
		{"bad pid", "pid", "not-a-number", false},
		{"package", "package", "com.example.app", true},
		{"package subprocess", "package", "com.example.app:worker", true},
		{"name", "name", "com.example.app:worker", true},
		{"missing", "name", "com.other.app", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := processMatchesTarget(process, test.kind, test.target); got != test.want {
				t.Fatalf("processMatchesTarget(%q, %q) = %v, want %v", test.kind, test.target, got, test.want)
			}
		})
	}
}

func TestNormalizePreflightRequestValues(t *testing.T) {
	if got := normalizePreflightMode(" SPAWN "); got != "spawn" {
		t.Fatalf("normalizePreflightMode() = %q", got)
	}
	if got := normalizePreflightMode("unexpected"); got != "attach" {
		t.Fatalf("default mode = %q", got)
	}
	if got := normalizePreflightTargetKind(" PID "); got != "pid" {
		t.Fatalf("normalizePreflightTargetKind() = %q", got)
	}
}
