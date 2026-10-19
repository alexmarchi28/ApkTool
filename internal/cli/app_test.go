package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"apktool/internal/apk"
)

var testDate = time.Date(2026, 10, 19, 15, 4, 5, 0, time.UTC)

// lines is an Input that returns its lines, then io.EOF. It fails the test
// if it is read many times after the end, which means the flow loops.
type lines struct {
	t     *testing.T
	lines []string
	eofs  int
}

func (l *lines) ReadLine() (string, error) {
	if len(l.lines) == 0 {
		if l.eofs++; l.eofs > 3 {
			l.t.Fatal("input read again and again after its end")
		}
		return "", io.EOF
	}
	s := l.lines[0]
	l.lines = l.lines[1:]
	return s, nil
}

type fakePlatform struct {
	hint, picked, downloads string
	revealed                []string
}

func (p *fakePlatform) PickHint() string     { return p.hint }
func (p *fakePlatform) PickAPK() string      { return p.picked }
func (p *fakePlatform) DownloadsDir() string { return p.downloads }
func (p *fakePlatform) Reveal(path string)   { p.revealed = append(p.revealed, path) }

type renameCall struct{ src, dst, pkg, label string }

type fakeEditor struct {
	info      apk.Info
	readErr   error // returned by the first Read
	renameErr error
	reads     []string
	renames   []renameCall
}

func (e *fakeEditor) Read(path string) (apk.Info, error) {
	e.reads = append(e.reads, path)
	if e.readErr != nil {
		err := e.readErr
		e.readErr = nil
		return apk.Info{}, err
	}
	return e.info, nil
}

func (e *fakeEditor) Rename(src, dst, pkg, label string) error {
	e.renames = append(e.renames, renameCall{src, dst, pkg, label})
	return e.renameErr
}

// fixture holds an App wired to fakes, with an existing APK file to select.
type fixture struct {
	app      *App
	out      bytes.Buffer
	platform *fakePlatform
	editor   *fakeEditor
	apk      string // path of an existing .apk file
	dir      string // a writable folder
}

func newFixture(t *testing.T, input ...string) *fixture {
	t.Helper()
	f := &fixture{dir: t.TempDir()}
	f.apk = filepath.Join(f.dir, "in.apk")
	if err := os.WriteFile(f.apk, []byte("apk"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.platform = &fakePlatform{downloads: filepath.Join(f.dir, "Downloads")}
	f.editor = &fakeEditor{info: apk.Info{Package: "com.example.app", Label: "Example"}}
	in := &lines{t: t, lines: input}
	f.app = New(in, &f.out, f.platform, f.editor, func() time.Time { return testDate })
	return f
}

// setInput replaces the answers of the user (used when they depend on f.apk).
func (f *fixture) setInput(t *testing.T, input ...string) {
	f.app.in = &lines{t: t, lines: input}
}

func (f *fixture) assertOutput(t *testing.T, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(f.out.String(), w) {
			t.Errorf("output does not contain %q:\n%s", w, f.out.String())
		}
	}
}

func TestRunRenamesWithTheAnswers(t *testing.T) {
	f := newFixture(t)
	dest := filepath.Join(f.dir, "out", "nested")
	f.setInput(t, `"`+f.apk+`"`, "  it.nuovo.app ", "  Nuovo Nome ", dest)
	if err := f.app.Run(); err != nil {
		t.Fatal(err)
	}
	want := renameCall{f.apk, filepath.Join(dest, "nuovo_nome_20261019.apk"), "it.nuovo.app", "Nuovo Nome"}
	if len(f.editor.renames) != 1 || f.editor.renames[0] != want {
		t.Errorf("Rename calls = %+v, want %+v", f.editor.renames, want)
	}
	if st, err := os.Stat(dest); err != nil || !st.IsDir() {
		t.Error("the destination folder was not created")
	}
	if len(f.platform.revealed) != 1 || f.platform.revealed[0] != want.dst {
		t.Errorf("revealed %v, want %s", f.platform.revealed, want.dst)
	}
	f.assertOutput(t, "com.example.app  →  "+green+"it.nuovo.app", "Example  →  "+green+"Nuovo Nome", "✔ APK creato:")
}

func TestRunEnterKeepsEverything(t *testing.T) {
	f := newFixture(t)
	f.setInput(t, f.apk, "", "", "")
	if err := f.app.Run(); err != nil {
		t.Fatal(err)
	}
	want := renameCall{f.apk, filepath.Join(f.platform.downloads, "example_20261019.apk"), "com.example.app", "Example"}
	if len(f.editor.renames) != 1 || f.editor.renames[0] != want {
		t.Errorf("Rename calls = %+v, want %+v", f.editor.renames, want)
	}
}

func TestRunKeepsAPackageTheAppAlreadyHas(t *testing.T) {
	f := newFixture(t)
	f.editor.info.Package = "Com.Example_1.x"
	f.setInput(t, f.apk, "Com.Example_1.x", "", "")
	if err := f.app.Run(); err != nil {
		t.Fatal(err)
	}
	if got := f.editor.renames[0].pkg; got != "Com.Example_1.x" {
		t.Errorf("package = %q", got)
	}
}

func TestRunUsesTheFileDialog(t *testing.T) {
	f := newFixture(t, "", "", "")
	f.platform.hint = "Scegli il file"
	f.platform.picked = f.apk
	if err := f.app.Run(); err != nil {
		t.Fatal(err)
	}
	if len(f.editor.reads) != 1 || f.editor.reads[0] != f.apk {
		t.Errorf("reads = %v", f.editor.reads)
	}
	f.assertOutput(t, dim+"     Scegli il file"+reset)
	if strings.Contains(f.out.String(), "Percorso dell'APK") {
		t.Error("the path was asked although the dialog returned a file")
	}
}

func TestRunAsksAgainForAnInvalidAPK(t *testing.T) {
	f := newFixture(t)
	notApk := filepath.Join(f.dir, "a.txt")
	os.WriteFile(notApk, nil, 0o644)
	f.editor.readErr = errors.New("manifest rotto")
	f.platform.picked = filepath.Join(f.dir, "missing.apk") // as if the dialog returned a vanished file
	f.setInput(t, notApk, f.dir, f.apk, f.apk, "", "", "")
	if err := f.app.Run(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(f.out.String(), "File non valido o inesistente."); got != 3 {
		t.Errorf("%d invalid file messages, want 3", got)
	}
	f.assertOutput(t, "Impossibile leggere l'APK: manifest rotto")
	if len(f.editor.reads) != 2 {
		t.Errorf("reads = %v, want 2", f.editor.reads)
	}
}

func TestRunAsksAgainForAnInvalidPackage(t *testing.T) {
	f := newFixture(t)
	f.setInput(t, f.apk, "app", "it..x", "1t.x", "it.x-y", "it.ok", "", "")
	if err := f.app.Run(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(f.out.String(), "Formato non valido."); got != 4 {
		t.Errorf("%d format messages, want 4", got)
	}
	if got := f.editor.renames[0].pkg; got != "it.ok" {
		t.Errorf("package = %q, want it.ok", got)
	}
}

func TestRunAsksAgainForAnUnusableFolder(t *testing.T) {
	f := newFixture(t)
	file := filepath.Join(f.dir, "file")
	os.WriteFile(file, nil, 0o644)
	f.setInput(t, f.apk, "", "", filepath.Join(file, "sub"), f.dir)
	if err := f.app.Run(); err != nil {
		t.Fatal(err)
	}
	f.assertOutput(t, "Impossibile usare questa cartella.")
	if got := filepath.Dir(f.editor.renames[0].dst); got != f.dir {
		t.Errorf("destination = %s, want %s", got, f.dir)
	}
}

func TestRunDoesNotOverwrite(t *testing.T) {
	f := newFixture(t)
	os.WriteFile(filepath.Join(f.dir, "example_20261019.apk"), nil, 0o644)
	f.setInput(t, f.apk, "", "", f.dir)
	if err := f.app.Run(); err != nil {
		t.Fatal(err)
	}
	if got := f.editor.renames[0].dst; got != filepath.Join(f.dir, "example_20261019_2.apk") {
		t.Errorf("destination = %s", got)
	}
}

func TestRunReportsRenameFailure(t *testing.T) {
	f := newFixture(t)
	f.editor.renameErr = errors.New("disco pieno")
	f.setInput(t, f.apk, "", "", f.dir)
	err := f.app.Run()
	if err == nil || err.Error() != "creazione dell'APK non riuscita: disco pieno" {
		t.Errorf("err = %v", err)
	}
	if len(f.platform.revealed) != 0 {
		t.Error("a file that was not created was revealed")
	}
}

func TestRunStopsWhenTheInputEnds(t *testing.T) {
	// Each case ends the input at a different question.
	for n := 0; n <= 3; n++ {
		f := newFixture(t)
		f.setInput(t, append([]string{f.apk}, "it.x.y", "Label", f.dir)[:n]...)
		if err := f.app.Run(); !errors.Is(err, ErrInputClosed) {
			t.Errorf("input of %d lines: err = %v, want ErrInputClosed", n, err)
		}
		if len(f.editor.renames) != 0 {
			t.Errorf("input of %d lines: the APK was created", n)
		}
	}
}

func TestExit(t *testing.T) {
	f := newFixture(t, "")
	f.app.Exit(errors.New("qualcosa"))
	want := "\n" + red + bold + "  ERRORE: " + reset + "qualcosa\n\n" + dim + "  Premi Invio per chiudere..." + reset
	if f.out.String() != want {
		t.Errorf("output = %q, want %q", f.out.String(), want)
	}
	if in := f.app.in.(*lines); len(in.lines) != 0 {
		t.Error("Exit did not wait for Enter")
	}

	f = newFixture(t) // no input left: Exit must return anyway
	f.app.Exit(nil)
	if want := "\n" + dim + "  Premi Invio per chiudere..." + reset; f.out.String() != want {
		t.Errorf("output = %q, want %q", f.out.String(), want)
	}
}

// TestRunWithTheRealEditor drives the whole flow on a real APK.
func TestRunWithTheRealEditor(t *testing.T) {
	editor, err := apk.DebugEditor()
	if err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	src, _ := filepath.Abs("../../testdata/demo.apk")
	in := &lines{t: t, lines: []string{"'" + src + "'", "it.prova.copia", "Copia di prova", dest}}
	var out bytes.Buffer
	if err := New(in, &out, &fakePlatform{}, editor, func() time.Time { return testDate }).Run(); err != nil {
		t.Fatal(err)
	}
	info, err := apk.Read(filepath.Join(dest, "copia_di_prova_20261019.apk"))
	if err != nil {
		t.Fatal(err)
	}
	if info != (apk.Info{Package: "it.prova.copia", Label: "Copia di prova"}) {
		t.Errorf("got %+v", info)
	}
}
