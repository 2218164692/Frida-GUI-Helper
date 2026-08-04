package scripts

import (
	"strings"
	"testing"

	"frida-gui-helper/internal/scriptparams"
)

func TestNativeTemplatesWaitForModuleLoad(t *testing.T) {
	nativeIDs := map[string]bool{
		"memory-scan-patch":          true,
		"stalker-native-trace":       true,
		"nativefunction-rpc":         true,
		"interceptor-replace-writer": true,
	}

	seen := make(map[string]bool)
	for _, template := range List() {
		if !nativeIDs[template.ID] {
			continue
		}
		seen[template.ID] = true
		for _, expected := range []string{
			"Process.findModuleByName",
			"android_dlopen_ext",
			"hookNowOrOnLoad",
		} {
			if !strings.Contains(template.Source, expected) {
				t.Errorf("%s does not contain %q", template.ID, expected)
			}
		}
	}

	for id := range nativeIDs {
		if !seen[id] {
			t.Errorf("native template %s not found", id)
		}
	}
}

func TestTemplateParameterMetadataRendersWithDefaults(t *testing.T) {
	for _, template := range List() {
		rendered, err := scriptparams.Render(template.Source, nil)
		if err != nil {
			t.Errorf("%s metadata or defaults are invalid: %v", template.ID, err)
			continue
		}
		if strings.Contains(rendered, "{{") {
			t.Errorf("%s leaves an unresolved parameter placeholder", template.ID)
		}
	}
}

func TestTemplatesDoNotUseLegacyNoPauseFlag(t *testing.T) {
	for _, template := range List() {
		if strings.Contains(template.Source, "--no-pause") {
			t.Errorf("%s contains the removed --no-pause flag", template.ID)
		}
	}
}
