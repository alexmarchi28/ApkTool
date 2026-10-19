//go:build windows

package platform

import (
	"os"
	"strings"
	"testing"
)

func TestWindowsPlatform(t *testing.T) {
	o := New(strings.NewReader(""))
	if o.isTTY {
		t.Error("a string reader taken for the console")
	}
	if o.PickHint() == "" {
		t.Error("the file dialog needs a hint")
	}
	if st, err := os.Stat(o.DownloadsDir()); err == nil && !st.IsDir() {
		t.Error("Downloads is not a folder")
	}
}
