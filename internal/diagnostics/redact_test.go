package diagnostics

import (
	"strings"
	"testing"
)

func TestSanitize(t *testing.T) {
	input := `device FAKE-SERIAL-0001 script C:\Users\example-user\AppData\Local\Temp\hook.js project X:\example-project\frida`
	got := Sanitize(input, "FAKE-SERIAL-0001", `C:\Users\example-user\AppData\Local\Temp`, `X:\example-project\frida`)
	for _, secret := range []string{"FAKE-SERIAL-0001", "example-user", `X:\example-project\frida`} {
		if strings.Contains(got, secret) {
			t.Fatalf("sanitized output still contains %q: %s", secret, got)
		}
	}
}
