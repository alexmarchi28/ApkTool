// Package apksign signs APKs with the v1 (JAR) and v2 schemes.
package apksign

import (
	"crypto/rsa"
	"crypto/x509"
	_ "embed"
	"encoding/pem"
	"errors"
)

// The debug key is the one shipped with uber-apk-signer, which earlier
// versions of ApkTool used: APKs made by either version can update each other.
//
//go:embed debug_key.pem
var debugKeyPEM []byte

//go:embed debug_cert.pem
var debugCertPEM []byte

// Signer holds an RSA key and its certificate.
type Signer struct {
	key  *rsa.PrivateKey
	cert *x509.Certificate
}

// DebugSigner returns the signer with the embedded debug key.
func DebugSigner() (*Signer, error) {
	return NewSigner(debugKeyPEM, debugCertPEM)
}

// NewSigner parses a PKCS#8 RSA key and a certificate, both PEM encoded.
func NewSigner(keyPEM, certPEM []byte) (*Signer, error) {
	kb, _ := pem.Decode(keyPEM)
	cb, _ := pem.Decode(certPEM)
	if kb == nil || cb == nil {
		return nil, errors.New("chiave o certificato non validi")
	}
	k, err := x509.ParsePKCS8PrivateKey(kb.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("la chiave non è RSA")
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, err
	}
	return &Signer{key: key, cert: cert}, nil
}
