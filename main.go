// ApkTool changes the package name and the label of an APK.
package main

import (
	"io"
	"os"
	"time"

	"apktool/internal/apk"
	"apktool/internal/cli"
	"apktool/internal/platform"
)

func main() {
	sys := platform.New(os.Stdin)
	sys.SetupConsole()
	app, err := newApp(sys, os.Stdout)
	if err == nil {
		err = app.Run()
	}
	app.Exit(err)
}

// newApp wires the interactive flow to the real platform and APK editor.
func newApp(sys *platform.OS, out io.Writer) (*cli.App, error) {
	editor, err := apk.DebugEditor()
	return cli.New(sys, out, sys, editor, time.Now), err
}
