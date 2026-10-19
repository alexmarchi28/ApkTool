package apksign

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"
)

func TestNewSignerRejectsNonRSAKeys(t *testing.T) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	if _, err := NewSigner(keyPEM, debugCertPEM); err == nil {
		t.Error("an EC key was accepted")
	}
	badDER := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("nope")})
	if _, err := NewSigner(badDER, debugCertPEM); err == nil {
		t.Error("a broken key was accepted")
	}
	badCert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("nope")})
	if _, err := NewSigner(debugKeyPEM, badCert); err == nil {
		t.Error("a broken certificate was accepted")
	}
}

func signedTestZip(t testing.TB) []byte {
	t.Helper()
	s, err := DebugSigner()
	if err != nil {
		t.Fatal(err)
	}
	signed, err := s.SignV2(makeZip(t, map[string]string{"a.txt": "hello"}, "c"))
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func TestVerifyV2RejectsEveryTruncation(t *testing.T) {
	signed := signedTestZip(t)
	for n := 0; n < len(signed); n++ {
		if err := VerifyV2(signed[:n]); err == nil {
			t.Fatalf("a copy cut at %d of %d bytes passed the verification", n, len(signed))
		}
	}
}

func TestVerifyV2RejectsEveryFlippedByte(t *testing.T) {
	signed := signedTestZip(t)
	// Every byte is covered: the content by the digest, the signing block by
	// its signature or by its structure.
	for i := range signed {
		bad := append([]byte(nil), signed...)
		bad[i] ^= 0x01
		if err := VerifyV2(bad); err == nil {
			t.Errorf("a change at byte %d of %d was not detected", i, len(signed))
		}
	}
}

// The verifier reads the APK that was just built, but it must never panic.
func FuzzVerifyV2(f *testing.F) {
	f.Add(signedTestZip(f))
	f.Fuzz(func(t *testing.T, b []byte) {
		VerifyV2(b)
	})
}
