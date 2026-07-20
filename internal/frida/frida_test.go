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
