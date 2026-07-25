package frida

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestBuildArgsForSpawnAndAttachFallback(t *testing.T) {
	script := filepath.Join(t.TempDir(), "hook.js")
	spawn, err := buildArgs(RunRequest{
		DeviceSerial: "device-1",
		Mode:         "spawn",
		TargetKind:   "package",
		Target:       "com.example.app",
	}, script)
	if err != nil {
		t.Fatal(err)
	}
	wantSpawn := []string{"-D", "device-1", "-f", "com.example.app", "-l", script}
	if !reflect.DeepEqual(spawn, wantSpawn) {
		t.Fatalf("spawn args = %#v, want %#v", spawn, wantSpawn)
	}

	attach, err := buildArgs(RunRequest{
		DeviceSerial: "device-1",
		Mode:         "attach",
		TargetKind:   "package",
		Target:       "com.example.app",
	}, script)
	if err != nil {
		t.Fatal(err)
	}
	wantAttach := []string{"-D", "device-1", "-N", "com.example.app", "-l", script}
	if !reflect.DeepEqual(attach, wantAttach) {
		t.Fatalf("attach args = %#v, want %#v", attach, wantAttach)
	}
}

func TestEmitSessionUsesLatestEmitter(t *testing.T) {
	runner := NewRunner(nil, nil)
	var received SessionInfo
	runner.SetSessionEmitter(func(info SessionInfo) {
		received = info
	})

	expected := SessionInfo{ID: "session-1", Running: false}
	runner.emitSession(expected)
	if received.ID != expected.ID || received.Running {
		t.Fatalf("unexpected session event: %+v", received)
	}

	runner.SetSessionEmitter(nil)
}
