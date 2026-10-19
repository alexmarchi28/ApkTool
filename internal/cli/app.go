// Package cli is the interactive flow of ApkTool: it asks for the APK, the
// new package name, the new label and the destination folder, then writes
// the new APK.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"apktool/internal/apk"
	"apktool/internal/manifest"
)

// ErrInputClosed is returned when the input ends before the flow is done.
var ErrInputClosed = errors.New("input terminato")

// Input reads the answers of the user, one line at a time. It returns an
// error, such as io.EOF, when there is nothing more to read.
type Input interface {
	ReadLine() (string, error)
}

// Platform is what the flow needs from the operating system.
type Platform interface {
	// PickHint is shown before PickAPK; "" if there is no file dialog.
	PickHint() string
	// PickAPK lets the user choose the APK in a file dialog. It returns ""
	// if there is no dialog or the user closed it.
	PickAPK() string
	// DownloadsDir is the default destination folder.
	DownloadsDir() string
	// Reveal shows the created file to the user.
	Reveal(path string)
}

// Editor reads and renames APKs.
type Editor interface {
	Read(path string) (apk.Info, error)
	Rename(src, dst, pkg, label string) error
}

// App runs the interactive flow.
type App struct {
	in       Input
	out      io.Writer
	platform Platform
	editor   Editor
	now      func() time.Time
}

// New returns an App that reads from in and writes to out.
func New(in Input, out io.Writer, platform Platform, editor Editor, now func() time.Time) *App {
	return &App{in: in, out: out, platform: platform, editor: editor, now: now}
}

// plan is what the user chose.
type plan struct {
	src   string
	info  apk.Info // package and label of src
	pkg   string
	label string
	dst   string
}

// Run asks the user for the APK and the changes, then writes the new APK.
func (a *App) Run() error {
	a.banner()
	var p plan
	var err error
	if p.src, p.info, err = a.selectAPK(); err != nil {
		return err
	}
	if p.pkg, err = a.askPackage(p.info.Package); err != nil {
		return err
	}
	if p.label, err = a.askLabel(p.info.Label); err != nil {
		return err
	}
	dir, err := a.askDestination()
	if err != nil {
		return err
	}
	p.dst = uniquePath(filepath.Join(dir, fileNameFor(p.label, a.now())))
	a.println()
	a.summary(p)
	return a.create(p)
}

// Exit shows err, if any, and waits for Enter so that the window opened by
// a double click does not close before the user has read the result.
func (a *App) Exit(err error) {
	if err != nil {
		a.println()
		a.println(red + bold + "  ERRORE: " + reset + err.Error())
	}
	a.println()
	a.print(dim + "  Premi Invio per chiudere..." + reset)
	a.in.ReadLine()
}

func (a *App) banner() {
	a.println()
	a.println(cyan + bold + "   ╔══════════════════════════════════════════╗")
	a.println("   ║                 ApkTool                  ║")
	a.println("   ║   Cambia package name ed etichetta APK   ║")
	a.println("   ╚══════════════════════════════════════════╝" + reset)
	a.println()
}

// selectAPK asks for the APK until a readable one is given.
func (a *App) selectAPK() (string, apk.Info, error) {
	a.println(bold + "  1) Seleziona l'APK" + reset)
	if hint := a.platform.PickHint(); hint != "" {
		a.println(dim + "     " + hint + reset)
	}
	path := a.platform.PickAPK()
	for {
		if path == "" {
			a.print("     Percorso dell'APK (puoi anche trascinarlo qui): ")
			line, err := a.readLine()
			if err != nil {
				return "", apk.Info{}, err
			}
			path = cleanPath(line)
		}
		if !isApkFile(path) {
			a.println(red + "     File non valido o inesistente." + reset)
			path = ""
			continue
		}
		info, err := a.editor.Read(path)
		if err == nil {
			a.println("     " + green + "✔ " + reset + path)
			a.println()
			return path, info, nil
		}
		a.println(red + "     Impossibile leggere l'APK: " + err.Error() + reset)
		path = ""
	}
}

// askPackage asks for the new package name; Enter keeps current.
func (a *App) askPackage(current string) (string, error) {
	a.println(bold + "  2) Package name" + reset)
	a.println("     Attuale: " + yellow + current + reset)
	for {
		a.print("     Nuovo package name " + dim + "(Invio = lascia invariato)" + reset + ": ")
		line, err := a.readLine()
		if err != nil {
			return "", err
		}
		s := strings.TrimSpace(line)
		if s == "" {
			s = current
		}
		if manifest.ValidPackage(s) || s == current {
			a.println()
			return s, nil
		}
		a.println(red + "     Formato non valido. Esempio: it.azienda.app.test (lettere, numeri e _, almeno un punto)." + reset)
	}
}

// askLabel asks for the new label; Enter keeps current.
func (a *App) askLabel(current string) (string, error) {
	a.println(bold + "  3) Etichetta (nome visibile dell'app)" + reset)
	a.println("     Attuale: " + yellow + displayLabel(current) + reset)
	a.print("     Nuova etichetta " + dim + "(Invio = lascia invariata)" + reset + ": ")
	line, err := a.readLine()
	if err != nil {
		return "", err
	}
	a.println()
	if s := strings.TrimSpace(line); s != "" {
		return s, nil
	}
	return current, nil
}

// askDestination asks for the destination folder until one can be created.
func (a *App) askDestination() (string, error) {
	def := a.platform.DownloadsDir()
	a.println(bold + "  4) Cartella di destinazione" + reset)
	a.println("     Predefinita: " + yellow + def + reset)
	for {
		a.print("     Percorso " + dim + "(Invio = usa la predefinita)" + reset + ": ")
		line, err := a.readLine()
		if err != nil {
			return "", err
		}
		dir := cleanPath(line)
		if dir == "" {
			dir = def
		}
		if err := os.MkdirAll(dir, 0o755); err == nil {
			return dir, nil
		}
		a.println(red + "     Impossibile usare questa cartella." + reset)
	}
}

func (a *App) summary(p plan) {
	a.println(bold + "  Riepilogo" + reset)
	a.printf("     Package:   %s  →  %s%s%s\n", p.info.Package, green, p.pkg, reset)
	a.printf("     Etichetta: %s  →  %s%s%s\n", displayLabel(p.info.Label), green, displayLabel(p.label), reset)
	a.printf("     File:      %s\n", p.dst)
	a.println()
}

func (a *App) create(p plan) error {
	a.println(dim + "  Creazione e firma dell'APK..." + reset)
	if err := a.editor.Rename(p.src, p.dst, p.pkg, p.label); err != nil {
		return fmt.Errorf("creazione dell'APK non riuscita: %w", err)
	}
	a.println(green + bold + "  ✔ APK creato:" + reset)
	a.println("     " + p.dst)
	a.println()
	a.println(dim + "     Nota: l'APK è firmato con una chiave di test, quindi si installa" + reset)
	a.println(dim + "     come app separata e non come aggiornamento dell'originale." + reset)
	a.platform.Reveal(p.dst)
	return nil
}

// readLine reads an answer; when the input is over it returns
// ErrInputClosed, so the flow stops instead of asking forever.
func (a *App) readLine() (string, error) {
	line, err := a.in.ReadLine()
	if err != nil {
		return "", ErrInputClosed
	}
	return line, nil
}

func displayLabel(label string) string {
	if label == "" {
		return "(nessuna)"
	}
	return label
}
