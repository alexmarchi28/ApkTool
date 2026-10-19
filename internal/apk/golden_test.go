package apk

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// TestRenameOutputIsStable pins the exact bytes written for the test APKs:
// a refactor must not change the APKs that ApkTool produces.
func TestRenameOutputIsStable(t *testing.T) {
	tests := []struct{ src, pkg, label, sha256 string }{
		{"demo.apk", "it.prova.copia", "Copia è", "93bdbfc0a82c167a9dbcac20b2f768bf3805c178a2f67b1e9b9dc8f815cd5dd7"},
		{"demo.apk", "com.example.demo", "Demo App", "1d0e53e6f6c3dbc0d14a2a183d1f71fc1743e36e4270cb4c0b417723f7799072"},
		{"demo-signed.apk", "it.prova.copia", "Copia è", "282167a72b880f4b8e01865ab077df8dc606a3999f4b0cd06861c6b4d64327ab"},
		{"nolabel.apk", "it.prova", "", "778d126f482c207b6fbcea40d600045bdc467e6e9f3063eb8410a14800ae3a4f"},
		{"nolabel.apk", "it.prova", "Nuova", "7c48ec2156a8a136d9d39a382a04af3809c66cd2b6db2c15a83f2ae7a2771873"},
	}
	for _, tt := range tests {
		dst := filepath.Join(t.TempDir(), "out.apk")
		if err := Rename(testdata+tt.src, dst, tt.pkg, tt.label); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(dst)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); got != tt.sha256 {
			t.Errorf("%s → %s %q: sha256 %s, want %s", tt.src, tt.pkg, tt.label, got, tt.sha256)
		}
	}
}
