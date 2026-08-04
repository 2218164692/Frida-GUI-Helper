package scriptparams

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

const completeMetadata = `/*
 * @frida-gui
 * {
 *   "name": "Inspector",
 *   "description": "Runtime options",
 *   "parameters": [
 *     {"name":"packageName","label":"Package","placeholder":"com.example.app","type":"string","required":true},
 *     {"name":"retryCount","type":"number","default":3},
 *     {"name":"verbose","type":"boolean","default":false},
 *     {"name":"moduleName","type":"select","default":"liba.so","options":["liba.so","libb.so"]}
 *   ]
 * }
 */
const packageName = {{packageName}};
const retryCount = {{retryCount}};
const verbose = {{verbose}};
const moduleName = {{moduleName}};
`

func TestParseMetadata(t *testing.T) {
	metadata, err := Parse(completeMetadata)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if metadata == nil || metadata.Name != "Inspector" || len(metadata.Parameters) != 4 {
		t.Fatalf("Parse() = %#v", metadata)
	}
	if metadata.Parameters[0].Label != "Package" || !metadata.Parameters[0].Required {
		t.Fatalf("first parameter = %#v", metadata.Parameters[0])
	}
	if metadata.Parameters[0].Placeholder != "com.example.app" {
		t.Fatalf("first parameter placeholder = %q", metadata.Parameters[0].Placeholder)
	}
	if metadata.Parameters[1].Default != json.Number("3") || !metadata.Parameters[1].HasDefault {
		t.Fatalf("number default = %#v", metadata.Parameters[1])
	}
	if metadata.Parameters[2].Default != false || !metadata.Parameters[2].HasDefault {
		t.Fatalf("boolean default = %#v", metadata.Parameters[2])
	}
	if metadata.Parameters[3].Label != "moduleName" || len(metadata.Parameters[3].Options) != 2 {
		t.Fatalf("select parameter = %#v", metadata.Parameters[3])
	}
}

func TestRenderUsesDefaultsAndEscapesStrings(t *testing.T) {
	value := "com.example.'quoted'\n</script>\\path"
	rendered, err := Render(completeMetadata, map[string]any{"packageName": value})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	checks := []string{
		`const packageName = "com.example.'quoted'\n\u003c/script\u003e\\path";`,
		`const retryCount = 3;`,
		`const verbose = false;`,
		`const moduleName = "liba.so";`,
	}
	for _, expected := range checks {
		if !strings.Contains(rendered, expected) {
			t.Errorf("rendered source does not contain %q:\n%s", expected, rendered)
		}
	}
}

func TestFalseZeroAndEmptyStringAreValidDefaults(t *testing.T) {
	source := `/* @frida-gui {
  "parameters": [
    {"name":"text","type":"string","default":""},
    {"name":"count","type":"number","default":0},
    {"name":"enabled","type":"boolean","default":false}
  ]
} */
const text = {{text}};
const count = {{count}};
const enabled = {{enabled}};`
	prepared, err := Prepare(source, nil)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if prepared.Metadata == nil || len(prepared.Metadata.Parameters) != 3 {
		t.Fatalf("Prepare() metadata = %#v", prepared.Metadata)
	}
	for _, parameter := range prepared.Metadata.Parameters {
		if !parameter.HasDefault {
			t.Errorf("parameter %s lost its default", parameter.Name)
		}
	}
	for _, expected := range []string{`const text = "";`, `const count = 0;`, `const enabled = false;`} {
		if !strings.Contains(prepared.Source, expected) {
			t.Errorf("prepared source does not contain %q", expected)
		}
	}
}

func TestRenderNumberBooleanAndSelect(t *testing.T) {
	rendered, err := Render(completeMetadata, map[string]any{
		"packageName": "com.example.app",
		"retryCount":  7.5,
		"verbose":     true,
		"moduleName":  "libb.so",
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	for _, expected := range []string{"const retryCount = 7.5;", "const verbose = true;", `const moduleName = "libb.so";`} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("rendered source does not contain %q", expected)
		}
	}
}

func TestRenderRejectsMissingRequired(t *testing.T) {
	_, err := Render(completeMetadata, nil)
	if err == nil || !strings.Contains(err.Error(), "packageName") {
		t.Fatalf("Render() error = %v, want missing packageName", err)
	}
	_, err = Render(completeMetadata, map[string]any{"packageName": "   "})
	if err == nil || !strings.Contains(err.Error(), "不能为空") {
		t.Fatalf("Render(blank required) error = %v, want non-empty validation", err)
	}
}

func TestRenderRejectsUnknownAndWrongTypes(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]any
		want   string
	}{
		{name: "unknown", values: map[string]any{"packageName": "ok", "typo": true}, want: "typo"},
		{name: "string", values: map[string]any{"packageName": 123}, want: "string"},
		{name: "number", values: map[string]any{"packageName": "ok", "retryCount": "7"}, want: "number"},
		{name: "nan", values: map[string]any{"packageName": "ok", "retryCount": math.NaN()}, want: "number"},
		{name: "boolean", values: map[string]any{"packageName": "ok", "verbose": "true"}, want: "boolean"},
		{name: "select", values: map[string]any{"packageName": "ok", "moduleName": "libx.so"}, want: "options"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Render(completeMetadata, test.values)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Render() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestParseRejectsInvalidMetadata(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "malformed json", source: `/* @frida-gui {"parameters": [} */`, want: "解析"},
		{name: "unknown field", source: `/* @frida-gui {"unexpected":true,"parameters":[]} */`, want: "unknown field"},
		{name: "duplicate", source: `/* @frida-gui {"parameters":[{"name":"x","type":"string"},{"name":"x","type":"string"}]} */`, want: "重复"},
		{name: "invalid type", source: `/* @frida-gui {"parameters":[{"name":"x","type":"object"}]} */`, want: "不支持"},
		{name: "select without options", source: `/* @frida-gui {"parameters":[{"name":"x","type":"select"}]} */`, want: "option"},
		{name: "invalid default", source: `/* @frida-gui {"parameters":[{"name":"x","type":"boolean","default":"yes"}]} */`, want: "default"},
		{name: "multiple blocks", source: `/* @frida-gui {"parameters":[]} */\n/* @frida-gui {"parameters":[]} */`, want: "只能"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse(test.source)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Parse() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestParseConsecutiveLineComments(t *testing.T) {
	source := `// @frida-gui
// {
//   "parameters": [{"name":"enabled","type":"boolean","default":true}]
// }
const enabled = {{enabled}};`
	metadata, err := Parse(source)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if metadata == nil || len(metadata.Parameters) != 1 {
		t.Fatalf("Parse() = %#v", metadata)
	}
	rendered, err := Render(source, nil)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(rendered, "const enabled = true;") {
		t.Fatalf("Render() = %s", rendered)
	}
}

func TestNoMetadataIsReturnedUnchanged(t *testing.T) {
	source := `const packageName = "{{packageName}}";`
	metadata, err := Parse(source)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if metadata != nil {
		t.Fatalf("Parse() = %#v, want nil", metadata)
	}
	rendered, err := Render(source, map[string]any{"packageName": "ignored", "unknown": true})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if rendered != source {
		t.Fatalf("Render() = %q, want exact source", rendered)
	}
}

func TestRenderRejectsUndeclaredPlaceholder(t *testing.T) {
	source := `/* @frida-gui {"parameters":[{"name":"known","type":"string","default":"ok"}]} */
const value = {{unknown}};`
	_, err := Render(source, nil)
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("Render() error = %v, want undeclared placeholder", err)
	}
}

func TestOptionalParameterWithoutDefaultRendersUndefined(t *testing.T) {
	source := `/* @frida-gui {"parameters":[{"name":"optional","type":"string"}]} */
const value = {{optional}};`
	rendered, err := Render(source, nil)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(rendered, "const value = undefined;") {
		t.Fatalf("Render() = %s", rendered)
	}
}

func TestMarkerInsideStringIsIgnored(t *testing.T) {
	source := `const example = "/* @frida-gui {invalid} */";`
	metadata, err := Parse(source)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if metadata != nil {
		t.Fatalf("Parse() = %#v, want nil", metadata)
	}
}
