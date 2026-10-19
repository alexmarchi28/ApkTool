// Package apk reads the package name and label of an APK and writes a copy
// with new ones, aligned and signed.
package apk

import (
	"archive/zip"
	"fmt"
	"io"

	"apktool/internal/apksign"
	"apktool/internal/binres"
	"apktool/internal/manifest"
)

const (
	manifestName = "AndroidManifest.xml"
	tableName    = "resources.arsc"
)

// Info describes an APK.
type Info struct {
	Package string
	Label   string // "" if the app has no label
}

// Signer signs the copies written by an Editor.
type Signer interface {
	// SignV1 returns MANIFEST.MF, CERT.SF and CERT.RSA for the given entries.
	SignV1(files []apksign.FileDigest, alg apksign.DigestAlgorithm) (mf, sf, block []byte, err error)
	// SignV2 returns the APK with a v2 signature block.
	SignV2(apk []byte) ([]byte, error)
}

// Editor writes renamed copies of APKs, signed by its Signer.
type Editor struct {
	signer Signer
}

// NewEditor returns an Editor that signs with s.
func NewEditor(s Signer) *Editor { return &Editor{signer: s} }

// DebugEditor returns an Editor that signs with the embedded debug key.
func DebugEditor() (*Editor, error) {
	s, err := apksign.DebugSigner()
	if err != nil {
		return nil, err
	}
	return NewEditor(s), nil
}

// Read returns the package name and label of the APK at path.
func Read(path string) (Info, error) {
	zr, err := open(path)
	if err != nil {
		return Info{}, err
	}
	defer zr.Close()
	m, table, err := load(&zr.Reader)
	if err != nil {
		return Info{}, err
	}
	return Info{Package: m.Package(), Label: m.Label(table)}, nil
}

// Read returns the package name and label of the APK at path.
func (e *Editor) Read(path string) (Info, error) { return Read(path) }

// Rename writes to dst a copy of the APK src signed with the debug key; see
// Editor.Rename.
func Rename(src, dst, pkg, label string) error {
	e, err := DebugEditor()
	if err != nil {
		return err
	}
	return e.Rename(src, dst, pkg, label)
}

// Rename writes to dst a copy of the APK src with the given package name and
// label, aligned and signed (v1 + v2). The copy is checked before being
// written, and dst is only created if everything succeeded.
func (e *Editor) Rename(src, dst, pkg, label string) error {
	if !manifest.ValidPackage(pkg) {
		return fmt.Errorf("package name non valido: %q", pkg)
	}
	zr, err := open(src)
	if err != nil {
		return err
	}
	defer zr.Close()
	m, table, err := load(&zr.Reader)
	if err != nil {
		return err
	}
	if err := edit(m, table, pkg, label); err != nil {
		return err
	}
	out, err := build(&zr.Reader, m.Encode(), apksign.V1DigestFor(m.MinSdk()), e.signer)
	if err != nil {
		return err
	}
	if err := check(out, pkg, label); err != nil {
		return fmt.Errorf("verifica dell'APK creato non riuscita: %w", err)
	}
	return writeFile(dst, out)
}

// edit sets the package name and the label of m. An unchanged label is left
// alone, so a translated one stays translated.
func edit(m *manifest.Manifest, table *binres.Table, pkg, label string) error {
	m.SetPackage(pkg)
	if label == m.Label(table) {
		return nil
	}
	return m.SetLabel(label)
}

func open(path string) (*zip.ReadCloser, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("il file non è un APK valido: %w", err)
	}
	return zr, nil
}

// load parses the manifest and, if present, the resource table.
func load(zr *zip.Reader) (*manifest.Manifest, *binres.Table, error) {
	mb, err := readEntry(zr, manifestName)
	if err != nil {
		return nil, nil, err
	}
	m, err := manifest.Parse(mb)
	if err != nil {
		return nil, nil, err
	}
	var table *binres.Table
	if tb, err := readEntry(zr, tableName); err == nil {
		table, _ = binres.ParseTable(tb) // without resources the label is shown as a reference
	}
	return m, table, nil
}

func readEntry(zr *zip.Reader, name string) ([]byte, error) {
	f, err := zr.Open(name)
	if err != nil {
		return nil, fmt.Errorf("%s non trovato nell'APK", name)
	}
	defer f.Close()
	return io.ReadAll(f)
}
