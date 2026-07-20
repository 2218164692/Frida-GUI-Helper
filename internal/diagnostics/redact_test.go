package diagnostics

import (
	"strings"
	"testing"
)

func TestSanitize(t *testing.T) {
	input := `device 9023f39a script C:\Users\alice\AppData\Local\Temp\hook.js project E:\private\frida`
	got := Sanitize(input, "9023f39a", `C:\Users\alice\AppData\Local\Temp`, `E:\private\frida`)
	for _, secret := range []string{"9023f39a", "alice", `E:\private\frida`} {
		if strings.Contains(got, secret) {
			t.Fatalf("sanitized output still contains %q: %s", secret, got)
		}
	}
}
