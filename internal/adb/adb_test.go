package adb

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseProcessesReturnsEmptySlice(t *testing.T) {
	processes := parseProcesses("")
	if processes == nil {
		t.Fatal("expected empty slice, got nil")
	}

	encoded, err := json.Marshal(processes)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "[]" {
		t.Fatalf("expected JSON [], got %s", encoded)
	}
}

func TestListPackagesParsesPathContainingEquals(t *testing.T) {
	out := "USER PID NAME\n"
	if got := parseProcesses(out); got == nil {
		t.Fatal("parseProcesses should return an empty slice")
	}

	line := "package:/data/app/~~abc==/com.example.demo-abc==/base.apk=com.example.demo"
	trimmed := line[len("package:"):]
	separator := strings.LastIndex(trimmed, "=")
	if separator < 0 {
		t.Fatal("missing separator")
	}

	path := trimmed[:separator]
	pkg := trimmed[separator+1:]
	if path != "/data/app/~~abc==/com.example.demo-abc==/base.apk" {
		t.Fatalf("unexpected path: %s", path)
	}
	if pkg != "com.example.demo" {
		t.Fatalf("unexpected package: %s", pkg)
	}
}
