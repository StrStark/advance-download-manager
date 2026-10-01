//go:build linux && !android

package autostart

import (
	"os"
	"strings"
	"testing"
)

func TestLinuxEntry(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if Enabled() {
		t.Fatal("should start disabled")
	}
	if err := Set(true); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(entryPath())
	if !Enabled() || !strings.Contains(string(b), BackgroundFlag) || !strings.Contains(string(b), "Exec=\"/") {
		t.Fatalf("bad entry:\n%s", b)
	}
	if err := Set(false); err != nil || Enabled() {
		t.Fatalf("disable failed: %v", err)
	}
}
