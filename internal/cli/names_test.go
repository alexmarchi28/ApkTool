package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileNameFor(t *testing.T) {
	date := time.Date(2026, 10, 19, 23, 59, 0, 0, time.UTC)
	tests := map[string]string{
		"La Mia App - Test": "la_mia_app_test_20261019.apk",
		"  Città!  ":        "città_20261019.apk",
		"a/b\\c:d":          "a_b_c_d_20261019.apk",
		"App 2.0":           "app_2_0_20261019.apk",
		"__x__":             "x_20261019.apk",
		"日本語":               "日本語_20261019.apk",
		"":                  "app_20261019.apk",
		"!!!":               "app_20261019.apk",
		"@0x7f010000":       "0x7f010000_20261019.apk",
	}
	for label, want := range tests {
		if got := fileNameFor(label, date); got != want {
			t.Errorf("fileNameFor(%q) = %q, want %q", label, got, want)
		}
	}
}

func TestUniquePath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.apk")
	if got := uniquePath(p); got != p {
		t.Errorf("free name changed: %s", got)
	}
	os.WriteFile(p, nil, 0o644)
	os.WriteFile(filepath.Join(dir, "x_2.apk"), nil, 0o644)
	if got := uniquePath(p); got != filepath.Join(dir, "x_3.apk") {
		t.Errorf("uniquePath = %s, want x_3.apk", got)
	}
	os.Mkdir(filepath.Join(dir, "d.apk"), 0o755) // a folder takes the name too
	if got := uniquePath(filepath.Join(dir, "d.apk")); got != filepath.Join(dir, "d_2.apk") {
		t.Errorf("uniquePath = %s, want d_2.apk", got)
	}
}

func TestUniquePathReturnsWhenTheNameCannotBeChecked(t *testing.T) {
	// The previous version looped forever when os.Stat failed for a reason
	// other than a missing file.
	p := "bad\x00name.apk"
	if got := uniquePath(p); got != p {
		t.Errorf("uniquePath = %q, want %q", got, p)
	}
}

func TestCleanPath(t *testing.T) {
	tests := map[string]string{
		`  "C:\Users\a b\x.apk" `: `C:\Users\a b\x.apk`,
		`& 'C:\x.apk'`:            `C:\x.apk`,
		`& "C:\a b\x.apk"`:        `C:\a b\x.apk`,
		`'/home/a b/x.apk' `:      `/home/a b/x.apk`,
		`/home/a/x.apk`:           `/home/a/x.apk`,
		"/home/a/x.apk\r":         `/home/a/x.apk`,
		``:                        ``,
		`  `:                      ``,
	}
	for in, want := range tests {
		if got := cleanPath(in); got != want {
			t.Errorf("cleanPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsApkFile(t *testing.T) {
	dir := t.TempDir()
	upper := filepath.Join(dir, "a.APK")
	txt := filepath.Join(dir, "a.txt")
	folder := filepath.Join(dir, "f.apk")
	os.WriteFile(upper, nil, 0o644)
	os.WriteFile(txt, nil, 0o644)
	os.Mkdir(folder, 0o755)
	tests := map[string]bool{upper: true, txt: false, folder: false, dir: false, filepath.Join(dir, "no.apk"): false, "": false}
	for path, want := range tests {
		if got := isApkFile(path); got != want {
			t.Errorf("isApkFile(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestDisplayLabel(t *testing.T) {
	if displayLabel("") != "(nessuna)" || displayLabel("X") != "X" {
		t.Error("displayLabel gave a wrong answer")
	}
}
