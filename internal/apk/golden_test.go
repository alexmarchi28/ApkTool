package apk

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"testing"
)

// TestRenameOutputIsStable pins what ApkTool writes for the test APKs: the
// order, the compression method and the uncompressed content of every
// entry, signature files included. The compressed bytes are left out, since
// they depend on the Go version.
func TestRenameOutputIsStable(t *testing.T) {
	tests := []struct{ src, pkg, label, sha256 string }{
		{"demo.apk", "it.prova.copia", "Copia è", "485a400044f4ba90e753c5088b04f5738b19a4e511e47f5dbf4b707e635b4868"},
		{"demo.apk", "com.example.demo", "Demo App", "6928b0e6bceedeb80c8707e012874f0ab3b5cbdb108bd1712f064943e70a31b7"},
		{"demo-signed.apk", "it.prova.copia", "Copia è", "15d99b0bbd050a56305673f6c9133c2eb347c5506320ef5e61a4971e4d170e5d"},
		{"nolabel.apk", "it.prova", "", "ae481b3492f9d1a8222fc6f844dd97378d406127278d668dcd6bf44f71c17bc5"},
		{"nolabel.apk", "it.prova", "Nuova", "7f6485929157d9d7a803ce470f20f6313bf28f6539b853ac7cd942b5dde13f31"},
	}
	for _, tt := range tests {
		dst := filepath.Join(t.TempDir(), "out.apk")
		if err := Rename(testdata+tt.src, dst, tt.pkg, tt.label); err != nil {
			t.Fatal(err)
		}
		if got := contentHash(t, dst); got != tt.sha256 {
			t.Errorf("%s → %s %q: content hash %s, want %s", tt.src, tt.pkg, tt.label, got, tt.sha256)
		}
	}
}

func contentHash(t *testing.T, path string) string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	h := sha256.New()
	for _, f := range zr.File {
		fmt.Fprintf(h, "%s\x00%d\x00%d\x00", f.Name, f.Method, f.UncompressedSize64)
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.Copy(h, rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
