package diagnostics

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		name    string
		message string
		code    string
	}{
		{"spawn timeout", "Failed to spawn: unexpectedly timed out while waiting for signal from process with PID 1775", "spawn-timeout"},
		{"early end", "Failed to attach: unexpected early end-of-stream", "early-end-of-stream"},
		{"gadget", "need Gadget to attach on jailed Android", "gadget-required"},
		{"permission", "su -c id: Permission denied", "permission-denied"},
		{"missing process", "Failed to attach: unable to find process with name com.example", "attach-target-missing"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			finding, ok := Classify("frida", test.message)
			if !ok {
				t.Fatal("expected a finding")
			}
			if finding.Code != test.code {
				t.Fatalf("code = %q, want %q", finding.Code, test.code)
			}
		})
	}
}

func TestClassifyIgnoresUnrelatedOutput(t *testing.T) {
	if finding, ok := Classify("frida", "Connected to Android device"); ok {
		t.Fatalf("unexpected finding: %#v", finding)
	}
}
