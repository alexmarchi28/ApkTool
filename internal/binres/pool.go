// Package binres reads and writes the binary resource formats that Android
// stores inside an APK: compiled XML (AndroidManifest.xml) and the resource
// table (resources.arsc). Only the parts needed by ApkTool are implemented.
package binres

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unicode/utf16"
)

// Chunk types (frameworks/base/libs/androidfw/include/androidfw/ResourceTypes.h).
const (
	typeStringPool   = 0x0001
	typeTable        = 0x0002
	typeXML          = 0x0003
	typeXMLStartNS   = 0x0100
	typeXMLEndNS     = 0x0101
	typeXMLStart     = 0x0102
	typeXMLEnd       = 0x0103
	typeXMLResMap    = 0x0180
	typeTablePackage = 0x0200
	typeTableType    = 0x0201
)

const (
	poolSorted = 1 << 0
	poolUTF8   = 1 << 8
)

// NoIndex marks a missing string reference.
const NoIndex = 0xFFFFFFFF

var le = binary.LittleEndian

var errTruncated = errors.New("dati binari troncati")

// chunk is a parsed chunk header with its full data (header included).
type chunk struct {
	typ        uint16
	headerSize int
	data       []byte
}

// readChunk reads the chunk that starts at b[0].
func readChunk(b []byte) (chunk, error) {
	if len(b) < 8 {
		return chunk{}, errTruncated
	}
	c := chunk{typ: le.Uint16(b), headerSize: int(le.Uint16(b[2:])), data: b}
	size := int(le.Uint32(b[4:]))
	if c.headerSize < 8 || size < c.headerSize || size > len(b) {
		return chunk{}, fmt.Errorf("chunk 0x%04x non valido", c.typ)
	}
	c.data = b[:size]
	return c, nil
}

// StringPool is a decoded string pool. Strings can be appended; styles are
// kept as they are, since they only refer to the first strings of the pool.
type StringPool struct {
	Strings []string
	utf8    bool
	styles  []byte   // raw style data
	styleOf []uint32 // style offsets
}

func parseStringPool(c chunk) (*StringPool, error) {
	if c.typ != typeStringPool || c.headerSize < 28 {
		return nil, errors.New("string pool non valido")
	}
	h := c.data
	count := int(le.Uint32(h[8:]))
	styleCount := int(le.Uint32(h[12:]))
	flags := le.Uint32(h[16:])
	stringsStart := int(le.Uint32(h[20:]))
	stylesStart := int(le.Uint32(h[24:]))
	offsetsEnd := c.headerSize + 4*(count+styleCount)
	if count < 0 || styleCount < 0 || offsetsEnd > len(h) || stringsStart > len(h) {
		return nil, errors.New("string pool non valido")
	}
	p := &StringPool{Strings: make([]string, count), utf8: flags&poolUTF8 != 0}
	for i := 0; i < count; i++ {
		off := stringsStart + int(le.Uint32(h[c.headerSize+4*i:]))
		s, err := decodeString(h, off, p.utf8)
		if err != nil {
			return nil, fmt.Errorf("stringa %d: %w", i, err)
		}
		p.Strings[i] = s
	}
	if styleCount > 0 {
		if stylesStart == 0 || stylesStart > len(h) {
			return nil, errors.New("stili del string pool non validi")
		}
		p.styleOf = make([]uint32, styleCount)
		for i := range p.styleOf {
			p.styleOf[i] = le.Uint32(h[c.headerSize+4*(count+i):])
		}
		p.styles = append([]byte(nil), h[stylesStart:]...)
	}
	return p, nil
}

func decodeString(b []byte, off int, isUTF8 bool) (string, error) {
	if off < 0 || off >= len(b) {
		return "", errTruncated
	}
	if isUTF8 {
		_, n1 := utf8Len(b, off)       // length in UTF-16 units, unused
		size, n2 := utf8Len(b, off+n1) // length in bytes
		start := off + n1 + n2
		if n1 == 0 || n2 == 0 || start+size > len(b) {
			return "", errTruncated
		}
		return string(b[start : start+size]), nil
	}
	if off+2 > len(b) {
		return "", errTruncated
	}
	n := int(le.Uint16(b[off:]))
	start := off + 2
	if n&0x8000 != 0 {
		if off+4 > len(b) {
			return "", errTruncated
		}
		n = (n&0x7FFF)<<16 | int(le.Uint16(b[off+2:]))
		start += 2
	}
	if start+2*n > len(b) {
		return "", errTruncated
	}
	u := make([]uint16, n)
	for i := range u {
		u[i] = le.Uint16(b[start+2*i:])
	}
	return string(utf16.Decode(u)), nil
}

// utf8Len decodes a length prefix of a UTF-8 pool string; it returns the
// value and the number of bytes used (0 if out of bounds).
func utf8Len(b []byte, off int) (int, int) {
	if off >= len(b) {
		return 0, 0
	}
	if b[off]&0x80 == 0 {
		return int(b[off]), 1
	}
	if off+1 >= len(b) {
		return 0, 0
	}
	return int(b[off]&0x7F)<<8 | int(b[off+1]), 2
}

// Add returns the index of s, appending it if the pool does not contain it.
func (p *StringPool) Add(s string) uint32 {
	for i, v := range p.Strings {
		if v == s && i >= len(p.styleOf) { // styled strings must keep their style
			return uint32(i)
		}
	}
	p.Strings = append(p.Strings, s)
	return uint32(len(p.Strings) - 1)
}

// Get returns the string at index i, or "" if i is out of range.
func (p *StringPool) Get(i uint32) string {
	if int64(i) >= int64(len(p.Strings)) {
		return ""
	}
	return p.Strings[i]
}

// maxUTF8Len is the longest string, in bytes and in UTF-16 units, that a
// UTF-8 pool can hold.
const maxUTF8Len = 0x7FFF

// fitsUTF8 reports whether every string can be written in a UTF-8 pool.
func (p *StringPool) fitsUTF8() bool {
	for _, s := range p.Strings {
		if len(s) > maxUTF8Len || len(utf16.Encode([]rune(s))) > maxUTF8Len {
			return false
		}
	}
	return true
}

// encode serializes the pool as a chunk. The "sorted" flag is dropped since
// appended strings break the order. A UTF-8 pool is written as UTF-16 if a
// string is too long for UTF-8: Android reads both.
func (p *StringPool) encode() []byte {
	const headerSize = 28
	isUTF8 := p.utf8 && p.fitsUTF8()
	var data []byte
	offsets := make([]byte, 4*(len(p.Strings)+len(p.styleOf)))
	for i, s := range p.Strings {
		le.PutUint32(offsets[4*i:], uint32(len(data)))
		if isUTF8 {
			data = appendUTF8String(data, s)
		} else {
			data = appendUTF16String(data, s)
		}
	}
	for len(data)%4 != 0 {
		data = append(data, 0)
	}
	for i, o := range p.styleOf {
		le.PutUint32(offsets[4*(len(p.Strings)+i):], o)
	}
	stringsStart := headerSize + len(offsets)
	stylesStart := 0
	if len(p.styleOf) > 0 {
		stylesStart = stringsStart + len(data)
	}
	var flags uint32
	if isUTF8 {
		flags = poolUTF8
	}
	out := make([]byte, headerSize, stringsStart+len(data)+len(p.styles))
	le.PutUint16(out[0:], typeStringPool)
	le.PutUint16(out[2:], headerSize)
	le.PutUint32(out[8:], uint32(len(p.Strings)))
	le.PutUint32(out[12:], uint32(len(p.styleOf)))
	le.PutUint32(out[16:], flags)
	le.PutUint32(out[20:], uint32(stringsStart))
	le.PutUint32(out[24:], uint32(stylesStart))
	out = append(out, offsets...)
	out = append(out, data...)
	out = append(out, p.styles...)
	le.PutUint32(out[4:], uint32(len(out)))
	return out
}

func appendUTF8String(b []byte, s string) []byte {
	b = appendUTF8Len(b, len(utf16.Encode([]rune(s))))
	b = appendUTF8Len(b, len(s))
	b = append(b, s...)
	return append(b, 0)
}

func appendUTF8Len(b []byte, n int) []byte {
	if n > 0x7F {
		return append(b, byte(n>>8)|0x80, byte(n))
	}
	return append(b, byte(n))
}

func appendUTF16String(b []byte, s string) []byte {
	u := utf16.Encode([]rune(s))
	if len(u) > 0x7FFF {
		b = le.AppendUint16(b, uint16(len(u)>>16)|0x8000)
	}
	b = le.AppendUint16(b, uint16(len(u)))
	for _, c := range u {
		b = le.AppendUint16(b, c)
	}
	return le.AppendUint16(b, 0)
}
