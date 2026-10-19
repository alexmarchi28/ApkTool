package apk

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"hash/crc32"
	"io"
	"strings"
	"time"
)

// alignedWriter writes ZIP entries, padding the local header of uncompressed
// entries so their data is aligned as zipalign does: 4 bytes, or 16 KiB for
// native libraries, which Android can then map directly from the APK.
type alignedWriter struct {
	*zip.Writer
	out *countingWriter
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

func newAlignedWriter(w io.Writer) *alignedWriter {
	cw := &countingWriter{w: w}
	return &alignedWriter{Writer: zip.NewWriter(cw), out: cw}
}

// copyRaw copies f without recompressing it.
func (w *alignedWriter) copyRaw(f *zip.File) error {
	raw, err := f.OpenRaw()
	if err != nil {
		return err
	}
	fh := f.FileHeader
	return w.writeRaw(&fh, raw)
}

// addCompressed adds a new deflated entry.
func (w *alignedWriter) addCompressed(name string, data []byte) error {
	var c bytes.Buffer
	fw, err := flate.NewWriter(&c, flate.BestCompression)
	if err != nil {
		return err
	}
	if _, err := fw.Write(data); err != nil {
		return err
	}
	if err := fw.Close(); err != nil {
		return err
	}
	fh := zip.FileHeader{
		Name:               name,
		Method:             zip.Deflate,
		CRC32:              crc32.ChecksumIEEE(data),
		CompressedSize64:   uint64(c.Len()),
		UncompressedSize64: uint64(len(data)),
	}
	fh.ModifiedDate = 0x21 // 1980-01-01, as aapt2 writes: the output does not depend on the clock
	return w.writeRaw(&fh, &c)
}

func (w *alignedWriter) writeRaw(fh *zip.FileHeader, data io.Reader) error {
	fh.Flags &^= 0x8          // sizes go in the local header, not in a data descriptor
	fh.Modified = time.Time{} // keep the DOS time fields, no extended timestamp
	fh.Extra = nil            // old extra fields (such as old alignment padding) are dropped
	if fh.Method == zip.Store && !strings.HasSuffix(fh.Name, "/") {
		if err := w.Flush(); err != nil { // so that out.n is the real offset
			return err
		}
		dataStart := w.out.n + 30 + int64(len(fh.Name))
		fh.Extra = alignmentExtra(dataStart, alignmentFor(fh.Name))
	}
	dw, err := w.CreateRaw(fh)
	if err != nil {
		return err
	}
	_, err = io.Copy(dw, data)
	return err
}

func alignmentFor(name string) int64 {
	if strings.HasPrefix(name, "lib/") && strings.HasSuffix(name, ".so") {
		return 16384
	}
	return 4
}

// alignmentExtra returns an extra field (ID 0xd935, as used by apksigner)
// that moves data starting at offset to a multiple of align.
func alignmentExtra(offset, align int64) []byte {
	const headerLen = 6 // ID, size, alignment
	pad := (align - (offset+headerLen)%align) % align
	extra := make([]byte, headerLen+pad)
	extra[0], extra[1] = 0x35, 0xd9
	extra[2], extra[3] = byte(2+pad), byte((2+pad)>>8)
	extra[4], extra[5] = byte(align), byte(align>>8)
	return extra
}
