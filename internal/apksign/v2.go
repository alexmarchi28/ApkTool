package apksign

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"errors"
)

// APK Signature Scheme v2: https://source.android.com/docs/security/features/apksigning/v2
const (
	v2BlockID         = 0x7109871a
	algRSAPKCS1SHA256 = 0x0103
	chunkSize         = 1 << 20
	blockMagic        = "APK Sig Block 42"
)

var le = binary.LittleEndian

// zipSections locates the central directory and the end of central directory
// record of a ZIP file without comment-dependent ambiguity.
func zipSections(apk []byte) (cdOffset, eocdOffset int, err error) {
	const eocdSize = 22
	for i := len(apk) - eocdSize; i >= 0 && i >= len(apk)-eocdSize-0xFFFF; i-- {
		if le.Uint32(apk[i:]) != 0x06054b50 || i+eocdSize+int(le.Uint16(apk[i+20:])) != len(apk) {
			continue
		}
		cdSize := int(le.Uint32(apk[i+12:]))
		cdOffset = int(le.Uint32(apk[i+16:]))
		if cdOffset+cdSize != i {
			return 0, 0, errors.New("ZIP non valido: directory centrale non contigua")
		}
		return cdOffset, i, nil
	}
	return 0, 0, errors.New("ZIP non valido: fine della directory centrale non trovata")
}

// SignV2 returns apk with an APK Signing Block holding a v2 signature,
// inserted right before the central directory.
func (s *Signer) SignV2(apk []byte) ([]byte, error) {
	cdOffset, eocdOffset, err := zipSections(apk)
	if err != nil {
		return nil, err
	}
	digest := V2Digest(apk[:cdOffset], apk[cdOffset:eocdOffset], apk[eocdOffset:])

	pub, err := x509.MarshalPKIXPublicKey(&s.key.PublicKey)
	if err != nil {
		return nil, err
	}
	signedData := cat(
		lp(lp(u32(algRSAPKCS1SHA256), lp(digest))), // digests
		lp(lp(s.cert.Raw)),                         // certificates
		lp(),                                       // additional attributes
	)
	h := sha256.Sum256(signedData)
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, h[:])
	if err != nil {
		return nil, err
	}
	signer := cat(lp(signedData), lp(lp(u32(algRSAPKCS1SHA256), lp(sig))), lp(pub))
	block := signingBlock(v2BlockID, lp(lp(signer)))

	out := make([]byte, 0, len(apk)+len(block))
	out = append(out, apk[:cdOffset]...)
	out = append(out, block...)
	out = append(out, apk[cdOffset:]...)
	// The end of central directory must point to the moved central directory.
	le.PutUint32(out[len(block)+eocdOffset+16:], uint32(cdOffset+len(block)))
	return out, nil
}

// V2Digest computes the v2 content digest of the three ZIP sections.
func V2Digest(entries, centralDir, eocd []byte) []byte {
	var chunks [][]byte
	for _, sec := range [][]byte{entries, centralDir, eocd} {
		for len(sec) > 0 {
			n := min(len(sec), chunkSize)
			chunks = append(chunks, sec[:n])
			sec = sec[n:]
		}
	}
	top := sha256.New()
	top.Write([]byte{0x5a})
	top.Write(u32(uint32(len(chunks))))
	for _, c := range chunks {
		h := sha256.New()
		h.Write([]byte{0xa5})
		h.Write(u32(uint32(len(c))))
		h.Write(c)
		top.Write(h.Sum(nil))
	}
	return top.Sum(nil)
}

// signingBlock builds an APK Signing Block with a single ID-value pair.
func signingBlock(id uint32, value []byte) []byte {
	pairLen := 4 + len(value)
	size := uint64(8 + pairLen + 8 + len(blockMagic)) // everything after the first size field
	b := le.AppendUint64(nil, size)
	b = le.AppendUint64(b, uint64(pairLen))
	b = le.AppendUint32(b, id)
	b = append(b, value...)
	b = le.AppendUint64(b, size)
	return append(b, blockMagic...)
}

// cat concatenates parts.
func cat(parts ...[]byte) []byte {
	var b []byte
	for _, p := range parts {
		b = append(b, p...)
	}
	return b
}

// lp concatenates parts and prefixes the result with its uint32 length.
func lp(parts ...[]byte) []byte {
	b := cat(parts...)
	return append(u32(uint32(len(b))), b...)
}

func u32(v uint32) []byte { return le.AppendUint32(nil, v) }
