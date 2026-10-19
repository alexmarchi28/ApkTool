//go:build !windows

package platform

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
)

// OS is the platform of Linux and macOS: there is no file dialog, so the APK
// path is typed or dragged into the terminal.
type OS struct {
	in *bufio.Reader
}

// New returns the platform, reading the answers of the user from in.
func New(in io.Reader) *OS { return &OS{in: bufio.NewReader(in)} }

// SetupConsole prepares the terminal; nothing is needed here.
func (o *OS) SetupConsole() {}

// ReadLine reads one line typed by the user.
func (o *OS) ReadLine() (string, error) { return readLine(o.in) }

// PickHint is "" since there is no file dialog.
func (o *OS) PickHint() string { return "" }

// PickAPK returns "": the path is asked in the terminal.
func (o *OS) PickAPK() string { return "" }

// DownloadsDir returns ~/Downloads.
func (o *OS) DownloadsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Downloads")
}

// Reveal does nothing: the path is printed in the terminal.
func (o *OS) Reveal(path string) {}
