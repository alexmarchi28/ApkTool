//go:build !windows

package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"apktool/internal/platform"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// TestRunOutputGolden pins what the user sees, byte for byte, so a refactor
// cannot change the interactive flow by accident.
func TestRunOutputGolden(t *testing.T) {
	tests := []struct {
		name  string
		apk   string
		input func(src, dest string) []string
	}{
		{"change", "demo.apk", func(src, dest string) []string {
			return []string{"nonexistent.apk", "'" + src + "'", "not a package", "it.prova.copia", "  Copia di prova  ", dest}
		}},
		{"keep", "nolabel.apk", func(src, dest string) []string {
			return []string{"& '" + src + "'", "", "", ""}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, _ := filepath.Abs(filepath.Join("testdata", tt.apk))
			runGolden(t, src, tt.input, filepath.Join("testdata", "golden", tt.name+".txt"))
		})
	}
}

func runGolden(t *testing.T, src string, input func(src, dest string) []string, golden string) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dest := t.TempDir()
	in := strings.NewReader(strings.Join(input(src, dest), "\n") + "\n")
	var out bytes.Buffer
	app, err := newApp(platform.New(in), &out)
	if err == nil {
		err = app.Run()
	}
	if err != nil {
		t.Fatal(err)
	}

	got := strings.NewReplacer(src, "<SRC>", dest, "<DEST>", home, "<HOME>").Replace(out.String())
	got = regexp.MustCompile(`_\d{8}\.apk`).ReplaceAllString(got, "_AAAAMMGG.apk")
	if *update {
		os.WriteFile(golden, []byte(got), 0o644)
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("output changed:\n%s", got)
	}
}
