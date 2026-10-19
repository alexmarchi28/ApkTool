//go:build !windows

package main

// Linux and macOS: no file dialog, the APK path is typed or dragged into the
// terminal. Also used by the tests.

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

var stdinReader = bufio.NewReader(os.Stdin)

func setupConsole() {}

func readLine() string {
	s, _ := stdinReader.ReadString('\n')
	return strings.TrimRight(s, "\r\n")
}

// pickHint is shown before pickApkFile; there is no file dialog here.
const pickHint = ""

func pickApkFile() string { return "" }

func downloadsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Downloads")
}

func revealInExplorer(path string) {}
