package apksign

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"errors"
)

var errBadBlock = errors.New("blocco di firma v2 non valido")

// VerifyV2 checks the v2 signature of apk: the content digest and the RSA
// signature against the certificate it carries. It only understands what
// SignV2 writes (one signer, RSA PKCS#1 v1.5 with SHA-256) and is used to
// double-check every APK before it is saved.
func VerifyV2(apk []byte) error {
	blockStart, cdOffset, eocdOffset, value, err := findV2Block(apk)
	if err != nil {
		return err
	}
	signers, err := readLP(&value)
	if err != nil {
		return err
	}
	signer, err := readLP(&signers)
	if err != nil {
		return err
	}
	signedData, err1 := readLP(&signer)
	sigs, err2 := readLP(&signer)
	pubDER, err3 := readLP(&signer)
	if err := errors.Join(err1, err2, err3); err != nil {
		return err
	}

	pub, err := x509.ParsePKIXPublicKey(pubDER)
	if err != nil {
		return err
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return errors.New("chiave della firma v2 non RSA")
	}
	sig, err := readAlgValue(&sigs)
	if err != nil {
		return err
	}
	h := sha256.Sum256(signedData)
	if err := rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, h[:], sig); err != nil {
		return errors.New("firma v2 non valida")
	}

	sd := signedData
	digests, err1 := readLP(&sd)
	certs, err2 := readLP(&sd)
	if err := errors.Join(err1, err2); err != nil {
		return err
	}
	certDER, err := readLP(&certs)
	if err != nil {
		return err
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return err
	}
	certPub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok || !certPub.Equal(rsaPub) {
		return errors.New("il certificato non corrisponde alla chiave della firma v2")
	}
	want, err := readAlgValue(&digests)
	if err != nil {
		return err
	}
	// The digest is computed as if the signing block were not there.
	eocd := append([]byte(nil), apk[eocdOffset:]...)
	le.PutUint32(eocd[16:], uint32(blockStart))
	if !bytes.Equal(V2Digest(apk[:blockStart], apk[cdOffset:eocdOffset], eocd), want) {
		return errors.New("il contenuto dell'APK non corrisponde alla firma v2")
	}
	return nil
}

// findV2Block returns the position of the APK Signing Block and the value of
// its v2 entry.
func findV2Block(apk []byte) (blockStart, cdOffset, eocdOffset int, value []byte, err error) {
	if cdOffset, eocdOffset, err = zipSections(apk); err != nil {
		return
	}
	if cdOffset < 24 || string(apk[cdOffset-16:cdOffset]) != blockMagic {
		err = errors.New("firma v2 assente")
		return
	}
	size := le.Uint64(apk[cdOffset-24:])
	if size < 24 || size > uint64(cdOffset-8) {
		err = errBadBlock
		return
	}
	blockStart = cdOffset - int(size) - 8
	pairs := apk[blockStart+8 : cdOffset-24]
	for len(pairs) >= 12 {
		n := le.Uint64(pairs)
		if n < 4 || n > uint64(len(pairs)-8) {
			break
		}
		if le.Uint32(pairs[8:]) == v2BlockID {
			return blockStart, cdOffset, eocdOffset, pairs[12 : 8+n], nil
		}
		pairs = pairs[8+n:]
	}
	err = errors.New("firma v2 assente")
	return
}

// readLP reads a uint32 length-prefixed value from the start of b.
func readLP(b *[]byte) ([]byte, error) {
	if len(*b) < 4 {
		return nil, errBadBlock
	}
	n := le.Uint32(*b)
	if uint64(n) > uint64(len(*b)-4) {
		return nil, errBadBlock
	}
	v := (*b)[4 : 4+n]
	*b = (*b)[4+n:]
	return v, nil
}

// readAlgValue reads the first (algorithm ID, value) pair of a sequence and
// checks that the algorithm is RSA PKCS#1 v1.5 with SHA-256.
func readAlgValue(seq *[]byte) ([]byte, error) {
	item, err := readLP(seq)
	if err != nil {
		return nil, err
	}
	if len(item) < 4 || le.Uint32(item) != algRSAPKCS1SHA256 {
		return nil, errors.New("algoritmo di firma v2 non supportato")
	}
	item = item[4:]
	return readLP(&item)
}
