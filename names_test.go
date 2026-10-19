package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidPackage(t *testing.T) {
	valid := []string{"a.b", "it.azienda.app_test", "Com.X1.y"}
	invalid := []string{"", "app", ".a.b", "a.b.", "a..b", "1a.b", "a.1b", "a-b.c", "a.b c", "à.b"}
	for _, s := range valid {
		if !validPackage(s) {
			t.Errorf("%q must be valid", s)
		}
	}
	for _, s := range invalid {
		if validPackage(s) {
			t.Errorf("%q must be invalid", s)
		}
	}
}

func TestFileNameFor(t *testing.T) {
	date := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	tests := map[string]string{
		"La Mia App - Test": "la_mia_app_test_20261019.apk",
		"  Città!  ":        "città_20261019.apk",
		"a/b\\c:d":          "a_b_c_d_20261019.apk",
		"":                  "app_20261019.apk",
		"!!!":               "app_20261019.apk",
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
}

func TestCleanPath(t *testing.T) {
	tests := map[string]string{
		`  "C:\Users\a b\x.apk" `: `C:\Users\a b\x.apk`,
		`& 'C:\x.apk'`:            `C:\x.apk`,
		`/home/a/x.apk`:           `/home/a/x.apk`,
		``:                        ``,
	}
	for in, want := range tests {
		if got := cleanPath(in); got != want {
			t.Errorf("cleanPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsApkFile(t *testing.T) {
	dir := t.TempDir()
	apk := filepath.Join(dir, "a.APK")
	txt := filepath.Join(dir, "a.txt")
	os.WriteFile(apk, nil, 0o644)
	os.WriteFile(txt, nil, 0o644)
	if !isApkFile(apk) || isApkFile(txt) || isApkFile(dir) || isApkFile(filepath.Join(dir, "no.apk")) {
		t.Error("isApkFile gave a wrong answer")
	}
}

func TestDisplayLabel(t *testing.T) {
	if displayLabel("") != "(nessuna)" || displayLabel("X") != "X" {
		t.Error("displayLabel gave a wrong answer")
	}
}
