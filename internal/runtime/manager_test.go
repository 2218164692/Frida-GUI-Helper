package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeAndroidArch(t *testing.T) {
	tests := map[string]string{
		"arm64-v8a":     "arm64",
		"armeabi-v7a":   "arm",
		"x86_64":        "x86_64",
		"x86":           "x86",
		"aarch64":       "arm64",
		"arm64,armeabi": "arm64",
	}
	for input, expected := range tests {
		actual, err := normalizeAndroidArch(input)
		if err != nil {
			t.Fatalf("normalizeAndroidArch(%q): %v", input, err)
		}
		if actual != expected {
			t.Fatalf("normalizeAndroidArch(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestNormalizeAndroidArchRejectsUnknown(t *testing.T) {
	if _, err := normalizeAndroidArch("mips64"); err == nil {
		t.Fatal("expected unknown ABI to be rejected")
	}
}

func TestFindServerAsset(t *testing.T) {
	assets := []ReleaseAsset{
		{Name: "frida-server-17.16.4-android-arm64.xz", BrowserDownloadURL: "https://github.com/frida/frida/releases/download/17.16.4/frida-server-17.16.4-android-arm64.xz"},
		{Name: "frida-server-17.16.4-android-arm.xz", BrowserDownloadURL: "https://github.com/frida/frida/releases/download/17.16.4/frida-server-17.16.4-android-arm.xz"},
	}
	asset, err := findServerAsset(assets, "17.16.4", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if asset.Name != "frida-server-17.16.4-android-arm64.xz" {
		t.Fatalf("unexpected asset: %s", asset.Name)
	}
}

func TestFindServerAssetRejectsNonGitHubURLWithoutMatchingName(t *testing.T) {
	_, err := findServerAsset([]ReleaseAsset{{
		Name:               "frida-server-17.16.4-android-arm64.xz",
		BrowserDownloadURL: "https://example.com/server.xz",
	}}, "17.16.4", "arm64")
	if err == nil {
		t.Fatal("expected asset without a GitHub URL to be rejected later by download")
	}
}

func TestDigestValue(t *testing.T) {
	if got := digestValue("sha256:ABC123"); got != "ABC123" {
		t.Fatalf("digestValue returned %q", got)
	}
	if got := digestValue("  abc123  "); got != "abc123" {
		t.Fatalf("digestValue returned %q", got)
	}
}

func TestValidSHA256(t *testing.T) {
	valid := strings.Repeat("ab", sha256.Size)
	if !validSHA256("sha256:" + valid) {
		t.Fatal("valid SHA-256 was rejected")
	}
	for _, value := range []string{"", "abc123", strings.Repeat("z", sha256.Size*2)} {
		if validSHA256(value) {
			t.Fatalf("invalid SHA-256 was accepted: %q", value)
		}
	}
}

func TestVerifiedServerCacheRejectsMissingDigests(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "frida-server")
	if err := os.WriteFile(path, []byte("server"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeMetadata(path, runtimeMetadata{Version: "17.16.4", Arch: "arm64"}); err != nil {
		t.Fatal(err)
	}
	ok, err := verifiedServerCache(path, ReleaseAsset{}, "17.16.4", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("cache without file and asset digests was accepted")
	}
}

func TestResolveRuntimeVersionPrefersInstalledCLIForLatest(t *testing.T) {
	tests := []struct {
		requested string
		installed string
		want      string
	}{
		{requested: "latest", installed: "16.5.5", want: "16.5.5"},
		{requested: "", installed: "17.16.4", want: "17.16.4"},
		{requested: "latest", installed: "", want: "latest"},
		{requested: "17.16.4", installed: "16.5.5", want: "17.16.4"},
	}
	for _, test := range tests {
		if got := resolveRuntimeVersion(test.requested, test.installed); got != test.want {
			t.Fatalf("resolveRuntimeVersion(%q, %q) = %q, want %q", test.requested, test.installed, got, test.want)
		}
	}
}

func TestVerifiedServerCacheUsesAssetDigestMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "frida-server")
	content := []byte("decompressed server binary")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	fileSum := sha256.Sum256(content)
	fileDigest := hex.EncodeToString(fileSum[:])
	assetDigest := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := writeMetadata(path, runtimeMetadata{
		Version:     "17.16.4",
		Arch:        "arm64",
		SHA256:      fileDigest,
		FileSHA256:  fileDigest,
		AssetSHA256: assetDigest,
	}); err != nil {
		t.Fatal(err)
	}
	asset := ReleaseAsset{Digest: "sha256:" + assetDigest}
	ok, err := verifiedServerCache(path, asset, "17.16.4", "arm64")
	if err != nil || !ok {
		t.Fatalf("verifiedServerCache() = %v, %v; want true, nil", ok, err)
	}

	asset.Digest = "sha256:" + fileDigest
	ok, err = verifiedServerCache(path, asset, "17.16.4", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("verifiedServerCache accepted a decompressed file digest as the asset digest")
	}
}

func TestEnableEmbeddedSitePackagesUncommentsDefaultDirective(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "python313._pth")
	content := "python313.zip\r\n.\r\n\r\n# Uncomment to run site.main() automatically\r\n#import site\r\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := enableEmbeddedSitePackages(dir); err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(updated)
	if !strings.Contains(text, "\r\nimport site\r\n") {
		t.Fatalf("site import was not enabled: %q", text)
	}
	if strings.Contains(text, "#import site") {
		t.Fatalf("commented site directive remains: %q", text)
	}
}

func TestActivateBundleDirectorySwapsWholeTree(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "frida")
	staging := filepath.Join(parent, ".frida-update-test")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "version"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(staging, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "version"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	legacyCollision := target + ".previous"
	if err := os.MkdirAll(legacyCollision, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyCollision, "user-file"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := activateBundleDirectory(staging, target); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "version"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("activated bundle = %q, want new", data)
	}
	if data, err := os.ReadFile(filepath.Join(legacyCollision, "user-file")); err != nil || string(data) != "keep" {
		t.Fatalf("unrelated sibling directory was changed: %q, %v", data, err)
	}
}

func TestRecoverBundleDirectoryRestoresInterruptedSwap(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "frida")
	backup := filepath.Join(parent, ".frida-previous-test")
	if err := os.MkdirAll(backup, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "manifest.json"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeBundleSwapJournal(target, backup); err != nil {
		t.Fatal(err)
	}

	if err := recoverBundleDirectory(target); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old" {
		t.Fatalf("restored bundle = %q, want old", data)
	}
}

func TestActivateBundleDirectoryRefusesForeignJournal(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "frida")
	staging := filepath.Join(parent, ".frida-update-test")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(staging, 0o755); err != nil {
		t.Fatal(err)
	}
	journal := bundleSwapJournalPath(target)
	if err := os.WriteFile(journal, []byte("user data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := activateBundleDirectory(staging, target); err == nil {
		t.Fatal("foreign journal must block activation")
	}
	data, err := os.ReadFile(journal)
	if err != nil || string(data) != "user data" {
		t.Fatalf("foreign journal was changed: %q, %v", data, err)
	}
}
