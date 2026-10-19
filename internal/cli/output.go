package cli

import "fmt"

// ANSI styles of the console output.
const (
	reset  = "\x1b[0m"
	bold   = "\x1b[1m"
	dim    = "\x1b[2m"
	red    = "\x1b[31m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	cyan   = "\x1b[36m"
)

// Output errors are ignored: there is nowhere else to report them.

func (a *App) print(s string) { fmt.Fprint(a.out, s) }

func (a *App) println(s ...string) {
	for _, v := range s {
		fmt.Fprint(a.out, v)
	}
	fmt.Fprintln(a.out)
}

func (a *App) printf(format string, args ...any) { fmt.Fprintf(a.out, format, args...) }
