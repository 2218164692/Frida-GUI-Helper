package toolchain

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFindFridaToolNeverFallsBackToPATH(t *testing.T) {
	root := t.TempDir()
	pathRoot := t.TempDir()
	name := "frida"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	pathTool := filepath.Join(pathRoot, name)
	if err := os.WriteFile(pathTool, []byte("system"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", pathRoot)

	resolver := &Resolver{baseDirs: []string{root}}
	if tool := resolver.FindFridaTool("frida"); tool.Found {
		t.Fatalf("FindFridaTool unexpectedly used %s", tool.Path)
	}
	if tool := resolver.FindExecutable("frida"); !tool.Found {
		t.Fatalf("generic resolver should still be allowed to use PATH: %s", tool.Error)
	}
}

func TestFindFridaToolUsesOnlyManagedFridaDirectory(t *testing.T) {
	root := t.TempDir()
	name := "frida"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	managed := filepath.Join(root, "frida", name)
	if err := os.MkdirAll(filepath.Dir(managed), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managed, []byte("managed"), 0o755); err != nil {
		t.Fatal(err)
	}
	resolver := &Resolver{baseDirs: []string{root}}
	tool := resolver.FindFridaTool("frida")
	if !tool.Found || tool.Path != managed || tool.Source != "bundled" {
		t.Fatalf("unexpected tool: %#v", tool)
	}
}

func TestFindFridaServerIgnoresLegacyToolsDirectory(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "tools", "frida-server", "android-arm64", "frida-server")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("legacy"), 0o755); err != nil {
		t.Fatal(err)
	}
	resolver := &Resolver{baseDirs: []string{root}}
	if server := resolver.FindFridaServer("17.9.11", "arm64-v8a"); server.Found {
		t.Fatalf("legacy server should be ignored: %#v", server)
	}

	managed := filepath.Join(root, "frida", "server", "android-arm64", "frida-server")
	if err := os.MkdirAll(filepath.Dir(managed), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managed, []byte("managed"), 0o755); err != nil {
		t.Fatal(err)
	}
	server := resolver.FindFridaServer("17.9.11", "arm64-v8a")
	if !server.Found || server.Path != managed {
		t.Fatalf("managed server not resolved: %#v", server)
	}
}

func TestFridaResolutionIgnoresParentAndWorkingDirectories(t *testing.T) {
	managedRoot := t.TempDir()
	parentRoot := t.TempDir()
	workingRoot := t.TempDir()
	name := "frida"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	for _, root := range []string{parentRoot, workingRoot} {
		path := filepath.Join(root, "frida", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("external"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	resolver := &Resolver{
		baseDirs:      []string{managedRoot, parentRoot, workingRoot},
		fridaBaseDirs: []string{managedRoot},
	}
	if tool := resolver.FindFridaTool("frida"); tool.Found {
		t.Fatalf("strict Frida resolver used an external directory: %#v", tool)
	}
}

func TestGenericBaseDirectoryDoesNotOverrideFridaRoot(t *testing.T) {
	managedRoot := t.TempDir()
	genericRoot := t.TempDir()
	name := "frida"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(genericRoot, "frida", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("external"), 0o755); err != nil {
		t.Fatal(err)
	}

	resolver := &Resolver{
		baseDirs:      []string{managedRoot},
		fridaBaseDirs: []string{managedRoot},
	}
	resolver.AddBaseDir(genericRoot)
	if tool := resolver.FindFridaTool("frida"); tool.Found {
		t.Fatalf("generic base directory overrode strict Frida root: %#v", tool)
	}
}
