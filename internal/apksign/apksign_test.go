package apksign

import (
	"archive/zip"
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

func debugSigner(t *testing.T) *Signer {
	t.Helper()
	s, err := DebugSigner()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// The certificate must stay the one used by previous versions (uber-apk-signer),
// so new copies can update the ones already installed.
func TestDebugCertificate(t *testing.T) {
	sum := sha256.Sum256(debugSigner(t).cert.Raw)
	const want = "1e08a903aef9c3a721510b64ec764d01d3d094eb954161b62544ea8f187b5953"
	if got := hex.EncodeToString(sum[:]); got != want {
		t.Errorf("certificate SHA-256 = %s, want %s", got, want)
	}
}

func TestNewSignerErrors(t *testing.T) {
	if _, err := NewSigner([]byte("x"), debugCertPEM); err == nil {
		t.Error("invalid key accepted")
	}
	if _, err := NewSigner(debugKeyPEM, []byte("x")); err == nil {
		t.Error("invalid certificate accepted")
	}
}

func TestIsSignatureFile(t *testing.T) {
	tests := map[string]bool{
		"META-INF/MANIFEST.MF":                true,
		"META-INF/CERT.SF":                    true,
		"META-INF/ANDROIDD.RSA":               true,
		"META-INF/key.dsa":                    true,
		"META-INF/X.EC":                       true,
		"META-INF/services/a.b.C":             false,
		"META-INF/kotlinx_coroutines.version": false,
		"META-INF/sub/CERT.SF":                false,
		"AndroidManifest.xml":                 false,
		"assets/CERT.SF":                      false,
	}
	for name, want := range tests {
		if got := IsSignatureFile(name); got != want {
			t.Errorf("IsSignatureFile(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestWriteAttrWrapsLongLines(t *testing.T) {
	var b bytes.Buffer
	long := "res/" + strings.Repeat("abcdefghij", 20) + ".xml"
	writeAttr(&b, "Name", long)
	lines := strings.Split(strings.TrimSuffix(b.String(), "\r\n"), "\r\n")
	if len(lines) < 3 {
		t.Fatalf("expected wrapped lines, got %q", lines)
	}
	joined := lines[0]
	for i, l := range lines {
		if len(l) > 70 {
			t.Errorf("line %d is %d bytes long", i, len(l))
		}
		if i > 0 {
			if !strings.HasPrefix(l, " ") {
				t.Errorf("continuation line %d does not start with a space", i)
			}
			joined += l[1:]
		}
	}
	if joined != "Name: "+long {
		t.Errorf("unwrapped value = %q", joined)
	}
}

func TestSignV1(t *testing.T) {
	s := debugSigner(t)
	files := []FileDigest{{Name: "AndroidManifest.xml", Digest: sha256.Sum256([]byte("m"))}, {Name: "classes.dex", Digest: sha256.Sum256([]byte("d"))}}
	mf, sf, block, err := s.SignV1(files)
	if err != nil {
		t.Fatal(err)
	}
	d := sha256.Sum256([]byte("m"))
	if !bytes.Contains(mf, []byte("Name: AndroidManifest.xml\r\nSHA-256-Digest: "+base64.StdEncoding.EncodeToString(d[:])+"\r\n")) {
		t.Errorf("MANIFEST.MF without the entry digest:\n%s", mf)
	}
	if !bytes.Contains(sf, []byte("SHA-256-Digest-Manifest: "+b64sum(mf)+"\r\n")) {
		t.Error("CERT.SF does not hold the digest of MANIFEST.MF")
	}
	if !bytes.Contains(sf, []byte("X-Android-APK-Signed: 2\r\n")) {
		t.Error("CERT.SF does not declare the v2 signature")
	}

	// The PKCS#7 block must carry the certificate and a valid signature of CERT.SF.
	var ci struct {
		ContentType asn1.ObjectIdentifier
		Content     asn1.RawValue `asn1:"explicit,tag:0"`
	}
	if _, err := asn1.Unmarshal(block, &ci); err != nil {
		t.Fatal(err)
	}
	if !ci.ContentType.Equal(oidSignedData) {
		t.Fatalf("content type %v", ci.ContentType)
	}
	var sd signedData
	if _, err := asn1.Unmarshal(ci.Content.Bytes, &sd); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sd.Certificates.Bytes, s.cert.Raw) {
		t.Error("certificate missing from the PKCS#7 block")
	}
	if len(sd.SignerInfos) != 1 {
		t.Fatalf("%d signers", len(sd.SignerInfos))
	}
	si := sd.SignerInfos[0]
	if si.IssuerAndSerial.Serial.Cmp(s.cert.SerialNumber) != 0 {
		t.Error("wrong serial number")
	}
	h := sha256.Sum256(sf)
	if err := rsa.VerifyPKCS1v15(&s.key.PublicKey, crypto.SHA256, h[:], si.EncryptedDigest); err != nil {
		t.Errorf("signature of CERT.SF: %v", err)
	}
}

func makeZip(t *testing.T, files map[string]string, comment string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for name, data := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(data))
	}
	w.SetComment(comment)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestSignV2(t *testing.T) {
	s := debugSigner(t)
	for _, comment := range []string{"", "a comment"} {
		apk := makeZip(t, map[string]string{"a.txt": "hello", "b.txt": strings.Repeat("x", 3<<20)}, comment)
		signed, err := s.SignV2(apk)
		if err != nil {
			t.Fatal(err)
		}
		if err := VerifyV2(signed); err != nil {
			t.Fatalf("comment %q: %v", comment, err)
		}
		// Still a valid ZIP with the same content.
		zr, err := zip.NewReader(bytes.NewReader(signed), int64(len(signed)))
		if err != nil {
			t.Fatal(err)
		}
		if len(zr.File) != 2 || zr.Comment != comment {
			t.Errorf("ZIP changed: %d files, comment %q", len(zr.File), zr.Comment)
		}
	}
}

func TestVerifyV2DetectsChanges(t *testing.T) {
	s := debugSigner(t)
	signed, err := s.SignV2(makeZip(t, map[string]string{"a.txt": "hello hello hello"}, ""))
	if err != nil {
		t.Fatal(err)
	}
	for _, pos := range []int{40, len(signed) - 30, len(signed) - 300} {
		bad := append([]byte(nil), signed...)
		bad[pos] ^= 0xff
		if err := VerifyV2(bad); err == nil {
			t.Errorf("change at byte %d not detected", pos)
		}
	}
	unsigned := makeZip(t, map[string]string{"a.txt": "x"}, "")
	if err := VerifyV2(unsigned); err == nil {
		t.Error("an unsigned APK passed the verification")
	}
}

func TestSignV2RejectsNonZip(t *testing.T) {
	if _, err := debugSigner(t).SignV2([]byte("not a zip file at all, really not")); err == nil {
		t.Error("SignV2 accepted a non-ZIP file")
	}
}

func TestV2DigestChunks(t *testing.T) {
	// Digests of different section splits must differ, and be stable.
	a := V2Digest([]byte("ab"), []byte("c"), []byte("d"))
	b := V2Digest([]byte("a"), []byte("bc"), []byte("d"))
	if bytes.Equal(a, b) {
		t.Error("section boundaries must change the digest")
	}
	if !bytes.Equal(a, V2Digest([]byte("ab"), []byte("c"), []byte("d"))) {
		t.Error("digest not deterministic")
	}
}
