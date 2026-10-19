// Package platform holds what ApkTool needs from the operating system: the
// console, the file dialog, the Downloads folder and the file manager. On
// Windows it uses the native APIs; elsewhere the APK path is typed or
// dragged into the terminal.
package platform

import (
	"bufio"
	"io"
	"strings"
)

// readLine reads one line from r without its line ending. A last line
// without newline is returned as is; io.EOF only comes when nothing is left.
func readLine(r *bufio.Reader) (string, error) {
	s, err := r.ReadString('\n')
	if err == io.EOF && s != "" {
		err = nil
	}
	return strings.TrimRight(s, "\r\n"), err
}
