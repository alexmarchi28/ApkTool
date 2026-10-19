package apksign

import (
	"archive/zip"
	"bytes"
	"crypto"
	"crypto/rsa"
	_ "crypto/sha1"
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

func TestV1DigestFor(t *testing.T) {
	// SHA-256 in JAR signatures is only supported from API level 18.
	for minSdk, want := range map[int]DigestAlgorithm{1: SHA1, 17: SHA1, 18: SHA256, 34: SHA256} {
		if got := V1DigestFor(minSdk); got != want {
			t.Errorf("V1DigestFor(%d) = %s, want %s", minSdk, got, want)
		}
	}
}

func TestSignV1(t *testing.T) {
	tests := []struct {
		alg  DigestAlgorithm
		name string
		hash crypto.Hash
		oid  asn1.ObjectIdentifier
	}{
		{SHA256, "SHA-256", crypto.SHA256, asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}},
		{SHA1, "SHA1", crypto.SHA1, asn1.ObjectIdentifier{1, 3, 14, 3, 2, 26}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkSignV1(t, tt.alg, tt.name, tt.hash, tt.oid)
		})
	}
}

func checkSignV1(t *testing.T, alg DigestAlgorithm, name string, hash crypto.Hash, oid asn1.ObjectIdentifier) {
	s := debugSigner(t)
	sum := func(b []byte) []byte { h := hash.New(); h.Write(b); return h.Sum(nil) }
	b64 := func(b []byte) string { return base64.StdEncoding.EncodeToString(sum(b)) }
	files := []FileDigest{{Name: "AndroidManifest.xml", Digest: sum([]byte("m"))}, {Name: "classes.dex", Digest: sum([]byte("d"))}}
	mf, sf, block, err := s.SignV1(files, alg)
	if err != nil {
		t.Fatal(err)
	}
	section := "Name: AndroidManifest.xml\r\n" + name + "-Digest: " + b64([]byte("m")) + "\r\n\r\n"
	if !bytes.Contains(mf, []byte(section)) {
		t.Errorf("MANIFEST.MF without the entry digest:\n%s", mf)
	}
	if !bytes.Contains(sf, []byte(name+"-Digest-Manifest: "+b64(mf)+"\r\n")) {
		t.Error("CERT.SF does not hold the digest of MANIFEST.MF")
	}
	if !bytes.Contains(sf, []byte("Name: AndroidManifest.xml\r\n"+name+"-Digest: "+b64([]byte(section))+"\r\n")) {
		t.Errorf("CERT.SF does not hold the digest of the manifest section:\n%s", sf)
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
	if len(sd.DigestAlgorithms) != 1 || !sd.DigestAlgorithms[0].Algorithm.Equal(oid) {
		t.Errorf("digest algorithms %v, want %v", sd.DigestAlgorithms, oid)
	}
	if len(sd.SignerInfos) != 1 {
		t.Fatalf("%d signers", len(sd.SignerInfos))
	}
	si := sd.SignerInfos[0]
	if si.IssuerAndSerial.Serial.Cmp(s.cert.SerialNumber) != 0 {
		t.Error("wrong serial number")
	}
	if !si.DigestAlgorithm.Algorithm.Equal(oid) {
		t.Errorf("signer digest algorithm %v, want %v", si.DigestAlgorithm.Algorithm, oid)
	}
	if err := rsa.VerifyPKCS1v15(&s.key.PublicKey, hash, sum(sf), si.EncryptedDigest); err != nil {
		t.Errorf("signature of CERT.SF: %v", err)
	}
}

func makeZip(t testing.TB, files map[string]string, comment string) []byte {
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
