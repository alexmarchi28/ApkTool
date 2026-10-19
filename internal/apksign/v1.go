package apksign

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	_ "crypto/sha1" // registers crypto.SHA1
	_ "crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"math/big"
	"strings"
)

// Names of the v1 signature files written by SignV1.
const (
	ManifestName  = "META-INF/MANIFEST.MF"
	SignatureName = "META-INF/CERT.SF"
	BlockName     = "META-INF/CERT.RSA"
)

// IsSignatureFile reports whether name is a v1 signature file, which must be
// removed before signing again.
func IsSignatureFile(name string) bool {
	if !strings.HasPrefix(name, "META-INF/") || strings.Contains(name[9:], "/") {
		return false
	}
	if name == ManifestName {
		return true
	}
	for _, ext := range []string{".SF", ".RSA", ".DSA", ".EC"} {
		if strings.HasSuffix(strings.ToUpper(name), ext) {
			return true
		}
	}
	return false
}

// DigestAlgorithm is the digest used by a v1 signature.
type DigestAlgorithm int

const (
	SHA256 DigestAlgorithm = iota
	SHA1
)

// V1DigestFor returns the digest that apksigner picks for an app with the
// given minimum API level: SHA-256 with RSA is only supported from API 18.
func V1DigestFor(minSdk int) DigestAlgorithm {
	if minSdk < 18 {
		return SHA1
	}
	return SHA256
}

// String returns the name used in the JAR manifest attributes.
func (d DigestAlgorithm) String() string {
	if d == SHA1 {
		return "SHA1"
	}
	return "SHA-256"
}

// Hash returns the hash function of d.
func (d DigestAlgorithm) Hash() crypto.Hash {
	if d == SHA1 {
		return crypto.SHA1
	}
	return crypto.SHA256
}

func (d DigestAlgorithm) oid() asn1.ObjectIdentifier {
	if d == SHA1 {
		return oidSHA1
	}
	return oidSHA256
}

// Sum returns the digest of b.
func (d DigestAlgorithm) Sum(b []byte) []byte {
	h := d.Hash().New()
	h.Write(b)
	return h.Sum(nil)
}

func (d DigestAlgorithm) b64sum(b []byte) string {
	return base64.StdEncoding.EncodeToString(d.Sum(b))
}

// FileDigest is the digest of the uncompressed content of a ZIP entry.
type FileDigest struct {
	Name   string
	Digest []byte
}

// SignV1 returns the content of MANIFEST.MF, CERT.SF and CERT.RSA (JAR
// signature) for the given entries, whose digests were computed with alg,
// in the order of the constants above. The signature declares that the APK
// is also signed with scheme v2.
func (s *Signer) SignV1(files []FileDigest, alg DigestAlgorithm) (mf, sf, block []byte, err error) {
	digestAttr := alg.String() + "-Digest"
	var m, f bytes.Buffer
	m.WriteString("Manifest-Version: 1.0\r\nCreated-By: ApkTool\r\n\r\n")
	var sections bytes.Buffer
	for _, fd := range files {
		var sec bytes.Buffer
		writeAttr(&sec, "Name", fd.Name)
		writeAttr(&sec, digestAttr, base64.StdEncoding.EncodeToString(fd.Digest))
		sec.WriteString("\r\n")
		m.Write(sec.Bytes())
		writeAttr(&sections, "Name", fd.Name)
		writeAttr(&sections, digestAttr, alg.b64sum(sec.Bytes()))
		sections.WriteString("\r\n")
	}
	f.WriteString("Signature-Version: 1.0\r\nCreated-By: ApkTool\r\n")
	writeAttr(&f, digestAttr+"-Manifest", alg.b64sum(m.Bytes()))
	f.WriteString("X-Android-APK-Signed: 2\r\n\r\n")
	f.Write(sections.Bytes())

	block, err = s.pkcs7(f.Bytes(), alg)
	return m.Bytes(), f.Bytes(), block, err
}

// writeAttr writes "name: value" wrapping lines at 72 bytes as required by
// the JAR manifest format (continuation lines start with a space).
func writeAttr(w *bytes.Buffer, name, value string) {
	line := name + ": " + value
	const max = 70
	for first := true; ; first = false {
		limit := max
		if !first {
			w.WriteByte(' ')
			limit = max - 1
		}
		if len(line) <= limit {
			w.WriteString(line + "\r\n")
			return
		}
		w.WriteString(line[:limit] + "\r\n")
		line = line[limit:]
	}
}

// ASN.1 structures of a detached PKCS#7 SignedData (RFC 2315).
var (
	oidData          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidSignedData    = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
	oidSHA1          = asn1.ObjectIdentifier{1, 3, 14, 3, 2, 26}
	oidSHA256        = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidRSAEncryption = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}
)

type algorithmID struct {
	Algorithm  asn1.ObjectIdentifier
	Parameters asn1.RawValue
}

type contentInfo struct {
	ContentType asn1.ObjectIdentifier
	Content     asn1.RawValue `asn1:"optional"` // [0] EXPLICIT, built by hand
}

type issuerAndSerial struct {
	Issuer asn1.RawValue
	Serial *big.Int
}

type signerInfo struct {
	Version            int
	IssuerAndSerial    issuerAndSerial
	DigestAlgorithm    algorithmID
	SignatureAlgorithm algorithmID
	EncryptedDigest    []byte
}

type signedData struct {
	Version          int
	DigestAlgorithms []algorithmID `asn1:"set"`
	ContentInfo      contentInfo
	Certificates     asn1.RawValue
	SignerInfos      []signerInfo `asn1:"set"`
}

// pkcs7 signs content with RSA and alg and returns a detached PKCS#7
// SignedData.
func (s *Signer) pkcs7(content []byte, alg DigestAlgorithm) ([]byte, error) {
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, alg.Hash(), alg.Sum(content))
	if err != nil {
		return nil, err
	}
	null := asn1.RawValue{Tag: asn1.TagNull}
	digestAlg := algorithmID{alg.oid(), null}
	sd := signedData{
		Version:          1,
		DigestAlgorithms: []algorithmID{digestAlg},
		ContentInfo:      contentInfo{ContentType: oidData},
		Certificates:     asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: s.cert.Raw},
		SignerInfos: []signerInfo{{
			Version:            1,
			IssuerAndSerial:    issuerAndSerial{asn1.RawValue{FullBytes: s.cert.RawIssuer}, s.cert.SerialNumber},
			DigestAlgorithm:    digestAlg,
			SignatureAlgorithm: algorithmID{oidRSAEncryption, null},
			EncryptedDigest:    sig,
		}},
	}
	inner, err := asn1.Marshal(sd)
	if err != nil {
		return nil, err
	}
	return asn1.Marshal(contentInfo{ContentType: oidSignedData, Content: asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: inner}})
}
