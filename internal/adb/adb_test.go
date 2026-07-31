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

func TestNormalizeFridaServerRemotePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", DefaultFridaServerRemotePath},
		{" /data/local/tmp/frida-server/ ", DefaultFridaServerRemotePath},
		{"/data/local/tmp/frida-server/frida-server", "/data/local/tmp/frida-server/frida-server"},
	}
	for _, test := range tests {
		got, err := normalizeFridaServerRemotePath(test.input)
		if err != nil {
			t.Fatalf("normalize %q: %v", test.input, err)
		}
		if got != test.want {
			t.Fatalf("normalize %q: got %q want %q", test.input, got, test.want)
		}
	}

	if _, err := normalizeFridaServerRemotePath("/data/local/tmp/frida server"); err == nil {
		t.Fatal("expected unsafe path error")
	}
}

func TestParseFridaServerPIDs(t *testing.T) {
	out := "root 10858 1 2249900 61968 0 S frida-server\n" +
		"root 13689 1 2249900 61968 0 S frida-server-16.5.5-android-arm64\n"
	got := parseFridaServerPIDs(out)
	if strings.Join(got, " ") != "10858 13689" {
		t.Fatalf("unexpected PIDs: %#v", got)
	}
}

func TestFridaServerPushPathForDirectory(t *testing.T) {
	got, err := fridaServerPushPathForKind(DefaultFridaServerRemotePath, "directory")
	if err != nil {
		t.Fatal(err)
	}
	want := "/data/local/tmp/frida-server/frida-server"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	got, err = fridaServerPushPathForKind(DefaultFridaServerRemotePath, "file")
	if err != nil || got != DefaultFridaServerRemotePath {
		t.Fatalf("file path: got %q err=%v", got, err)
	}
}

func TestNormalizeFridaServerVersion(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain", input: "17.16.4", want: "17.16.4"},
		{name: "prefixed and padded", input: "  v17.16.4\r\n", want: "17.16.4"},
		{name: "first non-empty line", input: "\n\nV17.16.4\nignored", want: "17.16.4"},
		{name: "empty", input: " \r\n ", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeFridaServerVersion(test.input); got != test.want {
				t.Fatalf("normalizeFridaServerVersion(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestRequireFridaServerVersion(t *testing.T) {
	if err := requireFridaServerVersion("16.5.5", ""); err != nil {
		t.Fatalf("empty expected version must preserve compatibility: %v", err)
	}
	if err := requireFridaServerVersion("17.16.4\n", "v17.16.4"); err != nil {
		t.Fatalf("equivalent versions must match: %v", err)
	}
	if err := requireFridaServerVersion("17.16.3", "17.16.4"); err == nil {
		t.Fatal("expected mismatched versions to fail")
	}
}

func TestFridaServerVersionsEqualRejectsUnknownVersions(t *testing.T) {
	if fridaServerVersionsEqual("", "17.16.4") {
		t.Fatal("empty actual version must not match")
	}
	if fridaServerVersionsEqual("17.16.4", "") {
		t.Fatal("empty expected version must not be treated as a proven match")
	}
	if !fridaServerVersionsEqual("V17.16.4", "17.16.4") {
		t.Fatal("normalized versions should match")
	}
}
