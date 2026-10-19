//go:build !windows

package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"apktool/internal/apk"
)

// TestRun drives the whole interactive flow as a user would.
func TestRun(t *testing.T) {
	dest := t.TempDir()
	src, _ := filepath.Abs("testdata/demo.apk")
	stdinReader = bufio.NewReader(strings.NewReader(strings.Join([]string{
		"nonexistent.apk", // rejected, asked again
		`"` + src + `"`,   // quoted, as when dragged onto the console
		"not a package",   // rejected, asked again
		"it.prova.copia",
		"Copia di prova",
		dest,
	}, "\n") + "\n"))
	stdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	err := run()
	os.Stdout = stdout
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dest, "copia_di_prova_*.apk"))
	if len(files) != 1 {
		t.Fatalf("output files: %v", files)
	}
	info, err := apk.Read(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if info.Package != "it.prova.copia" || info.Label != "Copia di prova" {
		t.Errorf("got %+v", info)
	}
}
