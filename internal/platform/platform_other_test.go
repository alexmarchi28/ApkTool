//go:build !windows

package platform

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestOtherPlatform(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	o := New(strings.NewReader(""))
	o.SetupConsole()
	o.Reveal("x")
	if o.PickHint() != "" || o.PickAPK() != "" {
		t.Error("there is no file dialog outside Windows")
	}
	if got := o.DownloadsDir(); got != filepath.Join(home, "Downloads") {
		t.Errorf("DownloadsDir = %s", got)
	}
}
