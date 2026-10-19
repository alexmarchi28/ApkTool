package apk

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"apktool/internal/apksign"
)

func TestRenameRejectsInvalidPackage(t *testing.T) {
	dir := t.TempDir()
	for _, pkg := range []string{"", "app", "it..prova", "1t.prova", "it.prova copia"} {
		if err := Rename(testdata+"demo.apk", filepath.Join(dir, "out.apk"), pkg, "X"); err == nil {
			t.Errorf("Rename accepted the package %q", pkg)
		}
	}
	if files, _ := os.ReadDir(dir); len(files) != 0 {
		t.Errorf("files left behind: %v", files)
	}
}

// fakeSigner wraps the debug signer and can make one of its steps fail or
// produce a broken result.
type fakeSigner struct {
	real          *apksign.Signer
	v1Err, v2Err  error
	corruptV2     bool
	gotAlgorithms []apksign.DigestAlgorithm
}

func (f *fakeSigner) SignV1(files []apksign.FileDigest, alg apksign.DigestAlgorithm) ([]byte, []byte, []byte, error) {
	f.gotAlgorithms = append(f.gotAlgorithms, alg)
	if f.v1Err != nil {
		return nil, nil, nil, f.v1Err
	}
	return f.real.SignV1(files, alg)
}

func (f *fakeSigner) SignV2(apk []byte) ([]byte, error) {
	if f.v2Err != nil {
		return nil, f.v2Err
	}
	out, err := f.real.SignV2(apk)
	if f.corruptV2 && err == nil {
		out[100] ^= 0xff // inside the first entry: the v2 digest no longer matches
	}
	return out, err
}

func newFakeSigner(t *testing.T) *fakeSigner {
	t.Helper()
	s, err := apksign.DebugSigner()
	if err != nil {
		t.Fatal(err)
	}
	return &fakeSigner{real: s}
}

func TestRenameSignerFailuresLeaveNoFile(t *testing.T) {
	boom := errors.New("boom")
	tests := map[string]func(*fakeSigner){
		"v1 fails":     func(f *fakeSigner) { f.v1Err = boom },
		"v2 fails":     func(f *fakeSigner) { f.v2Err = boom },
		"v2 is broken": func(f *fakeSigner) { f.corruptV2 = true },
	}
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			s := newFakeSigner(t)
			setup(s)
			dir := t.TempDir()
			if err := NewEditor(s).Rename(testdata+"demo.apk", filepath.Join(dir, "out.apk"), "it.prova", "X"); err == nil {
				t.Fatal("Rename succeeded")
			}
			if files, _ := os.ReadDir(dir); len(files) != 0 {
				t.Errorf("files left behind: %v", files)
			}
		})
	}
}

func TestEditorUsesItsSigner(t *testing.T) {
	s := newFakeSigner(t)
	e := NewEditor(s)
	dst := filepath.Join(t.TempDir(), "out.apk")
	if err := e.Rename(testdata+"demo.apk", dst, "it.prova", "X"); err != nil {
		t.Fatal(err)
	}
	if len(s.gotAlgorithms) != 1 || s.gotAlgorithms[0] != apksign.SHA256 {
		t.Errorf("SignV1 called with %v, want [SHA-256] (minSdk 21)", s.gotAlgorithms)
	}
	info, err := e.Read(dst)
	if err != nil || info.Package != "it.prova" || info.Label != "X" {
		t.Errorf("Read = %+v, %v", info, err)
	}
}

func TestRenameRejectsDuplicateEntries(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "dup.apk")
	files := entries(t, testdata+"demo.apk")
	writeZipEntries(t, src, files, "resources.arsc")
	if err := Rename(src, filepath.Join(dir, "out.apk"), "it.prova", "X"); err == nil {
		t.Error("an APK with a duplicate entry was accepted")
	}
}
