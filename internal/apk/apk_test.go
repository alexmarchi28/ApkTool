package apk

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"apktool/internal/apksign"
)

const testdata = "../../testdata/"

func TestRead(t *testing.T) {
	tests := []struct{ file, pkg, label string }{
		{"demo.apk", "com.example.demo", "Demo App"},
		{"demo-signed.apk", "com.example.demo", "Demo App"},
		{"nolabel.apk", "org.sample.nolabel", ""},
	}
	for _, tt := range tests {
		info, err := Read(testdata + tt.file)
		if err != nil {
			t.Fatalf("%s: %v", tt.file, err)
		}
		if info.Package != tt.pkg || info.Label != tt.label {
			t.Errorf("%s: got %+v, want %q %q", tt.file, info, tt.pkg, tt.label)
		}
	}
}

func TestReadErrors(t *testing.T) {
	dir := t.TempDir()
	notZip := filepath.Join(dir, "a.apk")
	os.WriteFile(notZip, []byte("hello"), 0o644)
	noManifest := filepath.Join(dir, "b.apk")
	writeZip(t, noManifest, map[string]string{"classes.dex": "x"})
	badManifest := filepath.Join(dir, "c.apk")
	writeZip(t, badManifest, map[string]string{"AndroidManifest.xml": "<manifest/>"})
	for _, p := range []string{filepath.Join(dir, "missing.apk"), notZip, noManifest, badManifest} {
		if _, err := Read(p); err == nil {
			t.Errorf("Read(%s) succeeded", filepath.Base(p))
		}
	}
}

func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for name, data := range files {
		e, _ := w.Create(name)
		e.Write([]byte(data))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func entries(t *testing.T, path string) map[string][]byte {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	out := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name], err = io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
	}
	return out
}

func TestRename(t *testing.T) {
	for _, src := range []string{"demo.apk", "demo-signed.apk", "nolabel.apk"} {
		t.Run(src, func(t *testing.T) {
			dst := filepath.Join(t.TempDir(), "out.apk")
			if err := Rename(testdata+src, dst, "it.prova.copia", "Copia è"); err != nil {
				t.Fatal(err)
			}
			info, err := Read(dst)
			if err != nil {
				t.Fatal(err)
			}
			if info.Package != "it.prova.copia" || info.Label != "Copia è" {
				t.Errorf("got %+v", info)
			}
			data, err := os.ReadFile(dst)
			if err != nil {
				t.Fatal(err)
			}
			if err := apksign.VerifyV2(data); err != nil {
				t.Error(err)
			}

			// Every other file is unchanged; old signatures are replaced by ours.
			in, out := entries(t, testdata+src), entries(t, dst)
			for name, b := range in {
				if name == manifestName || apksign.IsSignatureFile(name) {
					continue
				}
				if !bytes.Equal(out[name], b) {
					t.Errorf("%s changed", name)
				}
			}
			for name := range out {
				if apksign.IsSignatureFile(name) && !strings.HasPrefix(name, "META-INF/CERT.") && name != apksign.ManifestName {
					t.Errorf("old signature file %s kept", name)
				}
			}
			for _, name := range []string{apksign.ManifestName, apksign.SignatureName, apksign.BlockName} {
				if _, ok := out[name]; !ok {
					t.Errorf("%s missing", name)
				}
			}
			checkAlignment(t, dst)
		})
	}
}

// checkAlignment verifies what zipalign would: stored data starts at a
// multiple of 4, or of 16 KiB for native libraries.
func checkAlignment(t *testing.T, path string) {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Method != zip.Store {
			continue
		}
		off, err := f.DataOffset()
		if err != nil {
			t.Fatal(err)
		}
		if align := alignmentFor(f.Name); off%align != 0 {
			t.Errorf("%s starts at %d, not aligned to %d", f.Name, off, align)
		}
	}
}

func TestRenameKeepsUnchangedLabel(t *testing.T) {
	// Keeping the label must not turn the translated resource into plain text.
	dst := filepath.Join(t.TempDir(), "out.apk")
	if err := Rename(testdata+"demo.apk", dst, "it.prova", "Demo App"); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	m, _, err := load(&zr.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Label(nil); got != "@0x7f010000" {
		t.Errorf("label = %q, want the original resource reference", got)
	}
}

func TestRenameWithoutLabel(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "out.apk")
	if err := Rename(testdata+"nolabel.apk", dst, "it.prova", ""); err != nil {
		t.Fatal(err)
	}
	if info, err := Read(dst); err != nil || info.Label != "" {
		t.Errorf("got %+v, %v; the app must stay without label", info, err)
	}
}

func TestRenameIsDeterministic(t *testing.T) {
	// RSA PKCS#1 v1.5 signatures are deterministic, and no timestamps are written.
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.apk"), filepath.Join(dir, "b.apk")
	for _, p := range []string{a, b} {
		if err := Rename(testdata+"demo.apk", p, "it.prova", "X"); err != nil {
			t.Fatal(err)
		}
	}
	ba, _ := os.ReadFile(a)
	bb, _ := os.ReadFile(b)
	if !bytes.Equal(ba, bb) {
		t.Error("two runs produced different files")
	}
}

func TestRenameFailureLeavesNoFile(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "missing-dir", "out.apk")
	if err := Rename(testdata+"demo.apk", dst, "it.prova", "X"); err == nil {
		t.Fatal("Rename into a missing folder succeeded")
	}
	bad := filepath.Join(dir, "bad.apk")
	os.WriteFile(bad, []byte("garbage"), 0o644)
	if err := Rename(bad, filepath.Join(dir, "out.apk"), "it.prova", "X"); err == nil {
		t.Fatal("Rename of an invalid APK succeeded")
	}
	files, _ := os.ReadDir(dir)
	for _, f := range files {
		if f.Name() != "bad.apk" {
			t.Errorf("file left behind: %s", f.Name())
		}
	}
}

func TestAlignmentExtra(t *testing.T) {
	for _, align := range []int64{4, 16384} {
		for off := int64(0); off < 40; off++ {
			e := alignmentExtra(off, align)
			if (off+int64(len(e)))%align != 0 {
				t.Errorf("offset %d align %d: data at %d", off, align, off+int64(len(e)))
			}
			if e[0] != 0x35 || e[1] != 0xd9 || int(e[2])|int(e[3])<<8 != len(e)-4 {
				t.Errorf("malformed extra field % x", e[:6])
			}
		}
	}
}

func TestAlignmentFor(t *testing.T) {
	if alignmentFor("lib/arm64-v8a/libx.so") != 16384 || alignmentFor("resources.arsc") != 4 || alignmentFor("assets/lib/x.so") != 4 {
		t.Error("wrong alignment")
	}
}
