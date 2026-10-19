// Package apk reads the package name and label of an APK and writes a copy
// with new ones, aligned and signed with the debug key.
package apk

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// Read returns the package name and label of the APK at path.
func Read(path string) (Info, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return Info{}, fmt.Errorf("il file non è un APK valido: %w", err)
	}
	defer zr.Close()
	m, table, err := load(&zr.Reader)
	if err != nil {
		return Info{}, err
	}
	return Info{Package: m.Package(), Label: m.Label(table)}, nil
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

// Rename writes to dst a copy of the APK src with the given package name and
// label, aligned and signed (v1 + v2) with the debug key. The copy is checked
// before being written, and dst is only created if everything succeeded.
func Rename(src, dst, pkg, label string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("il file non è un APK valido: %w", err)
	}
	defer zr.Close()
	m, table, err := load(&zr.Reader)
	if err != nil {
		return err
	}
	m.SetPackage(pkg)
	// An unchanged label is left alone, so a translated one stays translated.
	if label != m.Label(table) {
		if err := m.SetLabel(label); err != nil {
			return err
		}
	}
	signer, err := apksign.DebugSigner()
	if err != nil {
		return err
	}
	out, err := build(&zr.Reader, m.Encode(), signer)
	if err != nil {
		return err
	}
	if err := check(out, pkg, label); err != nil {
		return fmt.Errorf("verifica dell'APK creato non riuscita: %w", err)
	}
	return writeFile(dst, out)
}

// check reads back the new APK before it is saved: the signature must be
// valid and the manifest must hold the requested values.
func check(out []byte, pkg, label string) error {
	if err := apksign.VerifyV2(out); err != nil {
		return err
	}
	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		return err
	}
	m, table, err := load(zr)
	if err != nil {
		return err
	}
	if got := m.Package(); got != pkg {
		return fmt.Errorf("package %q invece di %q", got, pkg)
	}
	if got := m.Label(table); got != label {
		return fmt.Errorf("etichetta %q invece di %q", got, label)
	}
	return nil
}

// build creates the new APK: every entry is copied as it is (still
// compressed), except the manifest, which is replaced, and the old signature
// files, which are dropped. The v1 signature is added at the end and the v2
// signature block before the central directory.
func build(zr *zip.Reader, newManifest []byte, signer *apksign.Signer) ([]byte, error) {
	var buf bytes.Buffer
	w := newAlignedWriter(&buf)
	var digests []apksign.FileDigest
	seen := map[string]bool{}
	for _, f := range zr.File {
		if seen[f.Name] {
			return nil, fmt.Errorf("l'APK contiene due volte il file %s", f.Name)
		}
		seen[f.Name] = true
		if apksign.IsSignatureFile(f.Name) {
			continue
		}
		if f.Name == manifestName {
			if err := w.addCompressed(f.Name, newManifest); err != nil {
				return nil, err
			}
			digests = append(digests, apksign.FileDigest{Name: f.Name, Digest: sha256.Sum256(newManifest)})
			continue
		}
		if err := w.copyRaw(f); err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		if !strings.HasSuffix(f.Name, "/") {
			d, err := digestEntry(f)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", f.Name, err)
			}
			digests = append(digests, apksign.FileDigest{Name: f.Name, Digest: d})
		}
	}
	mf, sf, block, err := signer.SignV1(digests)
	if err != nil {
		return nil, err
	}
	for _, e := range []struct {
		name string
		data []byte
	}{{apksign.ManifestName, mf}, {apksign.SignatureName, sf}, {apksign.BlockName, block}} {
		if err := w.addCompressed(e.name, e.data); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return signer.SignV2(buf.Bytes())
}

// digestEntry returns the SHA-256 of the uncompressed content of f; reading
// it also checks its CRC.
func digestEntry(f *zip.File) ([32]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return [32]byte{}, err
	}
	defer rc.Close()
	h := sha256.New()
	if _, err := io.Copy(h, rc); err != nil {
		return [32]byte{}, err
	}
	return [32]byte(h.Sum(nil)), nil
}

// writeFile writes data through a temporary file, so a failure never leaves
// a half-written APK at path.
func writeFile(name string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(name), ".apktool-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), name)
}

// alignedWriter writes ZIP entries, padding the local header of uncompressed
// entries so their data is aligned as zipalign does: 4 bytes, or 16 KiB for
// native libraries, which Android can then map directly from the APK.
type alignedWriter struct {
	*zip.Writer
	out *countingWriter
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

func newAlignedWriter(w io.Writer) *alignedWriter {
	cw := &countingWriter{w: w}
	return &alignedWriter{Writer: zip.NewWriter(cw), out: cw}
}

// copyRaw copies f without recompressing it.
func (w *alignedWriter) copyRaw(f *zip.File) error {
	raw, err := f.OpenRaw()
	if err != nil {
		return err
	}
	fh := f.FileHeader
	return w.writeRaw(&fh, raw)
}

// addCompressed adds a new deflated entry.
func (w *alignedWriter) addCompressed(name string, data []byte) error {
	var c bytes.Buffer
	fw, err := flate.NewWriter(&c, flate.BestCompression)
	if err != nil {
		return err
	}
	fw.Write(data)
	if err := fw.Close(); err != nil {
		return err
	}
	fh := zip.FileHeader{
		Name:               name,
		Method:             zip.Deflate,
		CRC32:              crc32.ChecksumIEEE(data),
		CompressedSize64:   uint64(c.Len()),
		UncompressedSize64: uint64(len(data)),
	}
	fh.ModifiedDate = 0x21 // 1980-01-01, as aapt2 writes: the output does not depend on the clock
	return w.writeRaw(&fh, &c)
}

func (w *alignedWriter) writeRaw(fh *zip.FileHeader, data io.Reader) error {
	fh.Flags &^= 0x8          // sizes go in the local header, not in a data descriptor
	fh.Modified = time.Time{} // keep the DOS time fields, no extended timestamp
	fh.Extra = nil            // old extra fields (such as old alignment padding) are dropped
	if fh.Method == zip.Store && !strings.HasSuffix(fh.Name, "/") {
		if err := w.Flush(); err != nil { // so that out.n is the real offset
			return err
		}
		dataStart := w.out.n + 30 + int64(len(fh.Name))
		fh.Extra = alignmentExtra(dataStart, alignmentFor(fh.Name))
	}
	dw, err := w.CreateRaw(fh)
	if err != nil {
		return err
	}
	_, err = io.Copy(dw, data)
	return err
}

func alignmentFor(name string) int64 {
	if strings.HasPrefix(name, "lib/") && strings.HasSuffix(name, ".so") {
		return 16384
	}
	return 4
}

// alignmentExtra returns an extra field (ID 0xd935, as used by apksigner)
// that moves data starting at offset to a multiple of align.
func alignmentExtra(offset, align int64) []byte {
	const headerLen = 6 // ID, size, alignment
	pad := (align - (offset+headerLen)%align) % align
	extra := make([]byte, headerLen+pad)
	extra[0], extra[1] = 0x35, 0xd9
	extra[2], extra[3] = byte(2+pad), byte((2+pad)>>8)
	extra[4], extra[5] = byte(align), byte(align>>8)
	return extra
}
