package apk

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"apktool/internal/apksign"
	"apktool/internal/binres"
)

// withMinSdk writes a copy of the test APK src whose manifest declares the
// given minimum API level, and returns its path.
func withMinSdk(t *testing.T, src string, minSdk uint32) string {
	t.Helper()
	files := entries(t, testdata+src)
	x, err := binres.ParseXML(files[manifestName])
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range x.Nodes {
		if n.Start != nil && x.ElementName(n.Start) == "uses-sdk" {
			x.FindAttr(n.Start, 0x0101020c).Data = minSdk
		}
	}
	files[manifestName] = x.Encode()
	path := filepath.Join(t.TempDir(), "in.apk")
	writeZipEntries(t, path, files)
	return path
}

// checkV1 verifies that MANIFEST.MF lists, with digest alg, every entry of
// the APK at path except the signature files.
func checkV1(t *testing.T, path string, alg apksign.DigestAlgorithm) {
	t.Helper()
	files := entries(t, path)
	mf := strings.ReplaceAll(string(files[apksign.ManifestName]), "\r\n ", "") // unwrap long lines
	listed := map[string]string{}
	for _, sec := range strings.Split(mf, "\r\n\r\n")[1:] {
		var name, digest string
		for _, line := range strings.Split(sec, "\r\n") {
			if v, ok := strings.CutPrefix(line, "Name: "); ok {
				name = v
			} else if v, ok := strings.CutPrefix(line, alg.String()+"-Digest: "); ok {
				digest = v
			}
		}
		if name != "" {
			listed[name] = digest
		}
	}
	for name, data := range files {
		if apksign.IsSignatureFile(name) || strings.HasSuffix(name, "/") {
			continue
		}
		h := alg.Hash().New()
		h.Write(data)
		if want := base64.StdEncoding.EncodeToString(h.Sum(nil)); listed[name] != want {
			t.Errorf("%s: %s digest %q, want %q", name, alg, listed[name], want)
		}
		delete(listed, name)
	}
	for name := range listed {
		t.Errorf("MANIFEST.MF lists %s, which is not in the APK", name)
	}
	if !bytes.Contains(files[apksign.SignatureName], []byte(alg.String()+"-Digest-Manifest: ")) {
		t.Errorf("CERT.SF is not signed with %s", alg)
	}
}

func TestRenameSignsV1ForTheMinimumAPILevel(t *testing.T) {
	tests := []struct {
		minSdk uint32
		want   apksign.DigestAlgorithm
	}{
		{15, apksign.SHA1}, // Android 4.0-4.2 only accept SHA-1 in JAR signatures
		{17, apksign.SHA1},
		{18, apksign.SHA256},
		{21, apksign.SHA256},
	}
	for _, tt := range tests {
		dst := filepath.Join(t.TempDir(), "out.apk")
		if err := Rename(withMinSdk(t, "demo.apk", tt.minSdk), dst, "it.prova", "X"); err != nil {
			t.Fatal(err)
		}
		checkV1(t, dst, tt.want)
	}
}

// writeZipEntries writes files to a new ZIP at path, plus a second copy of
// each entry named in dup.
func writeZipEntries(t *testing.T, path string, files map[string][]byte, dup ...string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	names := append(sortedNames(files), dup...)
	for _, name := range names {
		e, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		e.Write(files[name])
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func sortedNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
