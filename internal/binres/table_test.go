package binres

import (
	"strings"
	"testing"
)

// resValue is a simple resource value for the test tables.
type resValue struct {
	typ  uint8
	data uint32
}

// typeSpec describes one ResTable_type chunk to build.
type typeSpec struct {
	id         uint8
	flags      uint8 // typeFlagSparse, typeFlagOffset16
	compact    bool  // entries use the compact encoding
	language   string
	entryCount int
	entries    map[int]resValue
	bags       []int // entries written as complex (bag) entries
}

func (s typeSpec) encode() []byte {
	const configSize = 64
	headerSize := 20 + configSize
	cfg := make([]byte, configSize)
	le.PutUint32(cfg, configSize)
	copy(cfg[8:10], s.language) // ResTable_config.language
	count := s.entryCount

	var entries []byte
	offsets := map[int]int{}
	write := func(i int, v resValue, bag bool) {
		offsets[i] = len(entries)
		switch {
		case bag:
			entries = le.AppendUint16(entries, 16)
			entries = le.AppendUint16(entries, entryFlagComplex)
			entries = le.AppendUint32(entries, uint32(i)) // key
			entries = le.AppendUint32(entries, 0)         // parent
			entries = le.AppendUint32(entries, 0)         // count
		case s.compact:
			entries = le.AppendUint16(entries, uint16(i)) // key
			entries = le.AppendUint16(entries, entryFlagCompact|uint16(v.typ)<<8)
			entries = le.AppendUint32(entries, v.data)
		default:
			entries = le.AppendUint16(entries, 8)
			entries = le.AppendUint16(entries, 0)
			entries = le.AppendUint32(entries, uint32(i)) // key
			entries = le.AppendUint16(entries, 8)
			entries = append(entries, 0, v.typ)
			entries = le.AppendUint32(entries, v.data)
		}
	}
	var ids []int
	for i := 0; i < count; i++ {
		if v, ok := s.entries[i]; ok {
			write(i, v, false)
			ids = append(ids, i)
		}
		for _, b := range s.bags {
			if b == i {
				write(i, resValue{}, true)
				ids = append(ids, i)
			}
		}
	}

	var index []byte
	switch {
	case s.flags&typeFlagSparse != 0:
		for _, i := range ids {
			index = le.AppendUint16(index, uint16(i))
			index = le.AppendUint16(index, uint16(offsets[i]/4))
		}
		count = len(ids)
	case s.flags&typeFlagOffset16 != 0:
		for i := 0; i < count; i++ {
			if off, ok := offsets[i]; ok {
				index = le.AppendUint16(index, uint16(off/4))
			} else {
				index = le.AppendUint16(index, 0xFFFF)
			}
		}
	default:
		for i := 0; i < count; i++ {
			if off, ok := offsets[i]; ok {
				index = le.AppendUint32(index, uint32(off))
			} else {
				index = le.AppendUint32(index, NoIndex)
			}
		}
	}
	for len(index)%4 != 0 {
		index = append(index, 0)
	}

	b := make([]byte, 20)
	le.PutUint16(b[0:], typeTableType)
	le.PutUint16(b[2:], uint16(headerSize))
	b[8], b[9] = s.id, s.flags
	le.PutUint32(b[12:], uint32(count))
	le.PutUint32(b[16:], uint32(headerSize+len(index)))
	b = append(b, cfg...)
	b = append(b, index...)
	b = append(b, entries...)
	le.PutUint32(b[4:], uint32(len(b)))
	return b
}

// buildTable returns a resources.arsc with the global strings and one
// package with the given ID, type ID offset and type chunks.
func buildTable(strs []string, pkgID uint8, typeIDOffset uint8, types ...typeSpec) []byte {
	pkg := make([]byte, 288)
	le.PutUint16(pkg[0:], typeTablePackage)
	le.PutUint16(pkg[2:], 288)
	le.PutUint32(pkg[8:], uint32(pkgID))
	le.PutUint32(pkg[284:], uint32(typeIDOffset))
	for _, t := range types {
		pkg = append(pkg, t.encode()...)
	}
	le.PutUint32(pkg[4:], uint32(len(pkg)))

	b := make([]byte, 12)
	le.PutUint16(b[0:], typeTable)
	le.PutUint16(b[2:], 12)
	le.PutUint32(b[8:], 1)
	b = append(b, (&StringPool{Strings: strs}).encode()...)
	b = append(b, pkg...)
	le.PutUint32(b[4:], uint32(len(b)))
	return b
}

func parseTestTable(t *testing.T, b []byte) *Table {
	t.Helper()
	tb, err := ParseTable(b)
	if err != nil {
		t.Fatal(err)
	}
	return tb
}

func str(i uint32) resValue { return resValue{TypeString, i} }

func TestTableEncodings(t *testing.T) {
	strs := []string{"zero", "uno", "due"}
	entries := map[int]resValue{0: str(0), 2: str(2), 5: str(1)}
	tests := map[string]typeSpec{
		"offsets32":        {id: 1, entryCount: 6, entries: entries},
		"offset16":         {id: 1, flags: typeFlagOffset16, entryCount: 6, entries: entries},
		"sparse":           {id: 1, flags: typeFlagSparse, entryCount: 6, entries: entries},
		"compact":          {id: 1, compact: true, entryCount: 6, entries: entries},
		"sparse compact":   {id: 1, flags: typeFlagSparse, compact: true, entryCount: 6, entries: entries},
		"offset16 compact": {id: 1, flags: typeFlagOffset16, compact: true, entryCount: 6, entries: entries},
	}
	for name, spec := range tests {
		t.Run(name, func(t *testing.T) {
			tb := parseTestTable(t, buildTable(strs, 0x7f, 0, spec))
			for id, want := range map[uint32]string{0x7f010000: "zero", 0x7f010002: "due", 0x7f010005: "uno"} {
				if got, err := tb.String(id); err != nil || got != want {
					t.Errorf("String(%#x) = %q, %v; want %q", id, got, err, want)
				}
			}
			for _, id := range []uint32{0x7f010001, 0x7f010003, 0x7f010006, 0x7f01ffff, 0x7f020000, 0x01010000} {
				if s, err := tb.String(id); err == nil {
					t.Errorf("String(%#x) = %q, want an error", id, s)
				}
			}
		})
	}
}

func TestTablePrefersTheDefaultConfiguration(t *testing.T) {
	strs := []string{"italiano", "default", "tedesco"}
	it := typeSpec{id: 1, language: "it", entryCount: 2, entries: map[int]resValue{0: str(0), 1: str(0)}}
	def := typeSpec{id: 1, entryCount: 1, entries: map[int]resValue{0: str(1)}}
	de := typeSpec{id: 1, language: "de", entryCount: 2, entries: map[int]resValue{1: str(2)}}
	tb := parseTestTable(t, buildTable(strs, 0x7f, 0, it, def, de))
	if got, _ := tb.String(0x7f010000); got != "default" {
		t.Errorf("String = %q, want the default value", got)
	}
	// Without a default value, the first one found is used.
	if got, _ := tb.String(0x7f010001); got != "italiano" {
		t.Errorf("String = %q, want the first value", got)
	}
}

func TestTableFollowsReferences(t *testing.T) {
	strs := []string{"fine"}
	spec := typeSpec{id: 1, entryCount: 6, entries: map[int]resValue{
		0: {TypeReference, 0x7f010001},
		1: {TypeReference, 0x7f010002},
		2: str(0),
		3: {TypeReference, 0x7f010003}, // itself
		4: {TypeIntDec, 42},
		5: {TypeReference, 0x7f019999}, // missing
	}}
	tb := parseTestTable(t, buildTable(strs, 0x7f, 0, spec))
	if got, err := tb.String(0x7f010000); err != nil || got != "fine" {
		t.Errorf("String = %q, %v; want fine", got, err)
	}
	for id, want := range map[uint32]string{0x7f010003: "circolari", 0x7f010004: "non è una stringa", 0x7f010005: "non trovata"} {
		if _, err := tb.String(id); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("String(%#x): err = %v, want %q", id, err, want)
		}
	}
}

func TestTableSkipsBags(t *testing.T) {
	spec := typeSpec{id: 1, entryCount: 2, entries: map[int]resValue{1: str(0)}, bags: []int{0}}
	tb := parseTestTable(t, buildTable([]string{"x"}, 0x7f, 0, spec))
	if _, err := tb.String(0x7f010000); err == nil {
		t.Error("a bag was read as a string")
	}
	if got, _ := tb.String(0x7f010001); got != "x" {
		t.Errorf("String = %q, want x", got)
	}
}

func TestTableTypeIDOffset(t *testing.T) {
	spec := typeSpec{id: 1, entryCount: 1, entries: map[int]resValue{0: str(0)}}
	tb := parseTestTable(t, buildTable([]string{"x"}, 0x7f, 2, spec))
	if got, err := tb.String(0x7f030000); err != nil || got != "x" {
		t.Errorf("String = %q, %v; want x with the type ID moved by the offset", got, err)
	}
}

func TestParseTableRejectsBrokenTables(t *testing.T) {
	good := buildTable([]string{"x"}, 0x7f, 0, typeSpec{id: 1, entryCount: 1, entries: map[int]resValue{0: str(0)}})
	// Every truncation must fail cleanly or still give a usable table.
	for n := 0; n < len(good); n++ {
		if tb, err := ParseTable(good[:n]); err == nil {
			tb.String(0x7f010000)
		}
	}
	noPool := append([]byte(nil), good[:12]...)
	le.PutUint32(noPool[4:], 12)
	if _, err := ParseTable(noPool); err == nil {
		t.Error("a table without string pool was accepted")
	}
}
