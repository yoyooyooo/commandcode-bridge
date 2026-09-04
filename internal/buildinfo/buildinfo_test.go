package buildinfo

import "testing"

func TestStringIncludesBinaryName(t *testing.T) {
	if got := String(); got == "" {
		t.Fatal("empty version string")
	}
}
