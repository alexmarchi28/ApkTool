package apk

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strings"

	"apktool/internal/apksign"
)

// build creates the new APK: every entry is copied as it is (still
// compressed), except the manifest, which is replaced, and the old signature
// files, which are dropped. The v1 signature, with digest alg, is added at
// the end and the v2 signature block before the central directory.
func build(zr *zip.Reader, newManifest []byte, alg apksign.DigestAlgorithm, signer Signer) ([]byte, error) {
	var buf bytes.Buffer
	w := newAlignedWriter(&buf)
	var digests []apksign.FileDigest
	seen := map[string]bool{}
	for _, f := range zr.File {
		if seen[f.Name] {
			return nil, fmt.Errorf("l'APK contiene due volte il file %s", f.Name)
		}
		seen[f.Name] = true
		switch {
		case apksign.IsSignatureFile(f.Name):
			continue
		case f.Name == manifestName:
			if err := w.addCompressed(f.Name, newManifest); err != nil {
				return nil, err
			}
			digests = append(digests, apksign.FileDigest{Name: f.Name, Digest: alg.Sum(newManifest)})
		default:
			d, err := copyEntry(w, f, alg)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", f.Name, err)
			}
			if d != nil {
				digests = append(digests, *d)
			}
		}
	}
	mf, sf, block, err := signer.SignV1(digests, alg)
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

// copyEntry copies f to w without recompressing it and returns the digest of
// its content, or nil for a directory.
func copyEntry(w *alignedWriter, f *zip.File, alg apksign.DigestAlgorithm) (*apksign.FileDigest, error) {
	if err := w.copyRaw(f); err != nil {
		return nil, err
	}
	if strings.HasSuffix(f.Name, "/") {
		return nil, nil
	}
	// Reading the content also checks its CRC.
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	h := alg.Hash().New()
	if _, err := io.Copy(h, rc); err != nil {
		return nil, err
	}
	return &apksign.FileDigest{Name: f.Name, Digest: h.Sum(nil)}, nil
}
