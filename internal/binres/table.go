package binres

import (
	"errors"
	"fmt"
)

// Table is a resource table (resources.arsc), read only.
type Table struct {
	strings  *StringPool
	packages map[uint8][]typeChunk // package ID → its type chunks
}

// typeChunk is a ResTable_type chunk with its type ID.
type typeChunk struct {
	id uint8
	chunk
}

const (
	typeFlagSparse   = 0x01
	typeFlagOffset16 = 0x02
	entryFlagComplex = 0x0001
	entryFlagCompact = 0x0008
)

// ParseTable decodes resources.arsc.
func ParseTable(b []byte) (*Table, error) {
	root, err := readChunk(b)
	if err != nil || root.typ != typeTable {
		return nil, errors.New("resources.arsc non valido")
	}
	t := &Table{packages: map[uint8][]typeChunk{}}
	for off := root.headerSize; off < len(root.data); {
		c, err := readChunk(root.data[off:])
		if err != nil {
			return nil, err
		}
		off += len(c.data)
		switch c.typ {
		case typeStringPool:
			if t.strings, err = parseStringPool(c); err != nil {
				return nil, err
			}
		case typeTablePackage:
			if err := t.addPackage(c); err != nil {
				return nil, err
			}
		}
	}
	if t.strings == nil {
		return nil, errors.New("resources.arsc senza string pool")
	}
	return t, nil
}

func (t *Table) addPackage(c chunk) error {
	if c.headerSize < 284 {
		return errors.New("pacchetto di risorse non valido")
	}
	id := uint8(le.Uint32(c.data[8:]))
	var typeIDOffset uint8
	if c.headerSize >= 288 {
		typeIDOffset = uint8(le.Uint32(c.data[284:]))
	}
	for off := c.headerSize; off < len(c.data); {
		sub, err := readChunk(c.data[off:])
		if err != nil {
			return err
		}
		off += len(sub.data)
		if sub.typ == typeTableType && sub.headerSize >= 24 {
			t.packages[id] = append(t.packages[id], typeChunk{sub.data[8] + typeIDOffset, sub})
		}
	}
	return nil
}

// String resolves a string resource, following references. The default
// configuration is preferred; otherwise the first value found is used.
func (t *Table) String(id uint32) (string, error) {
	for depth := 0; depth < 8; depth++ {
		typ, data, err := t.value(id)
		if err != nil {
			return "", err
		}
		switch typ {
		case TypeString:
			return t.strings.Get(data), nil
		case TypeReference:
			id = data
		default:
			return "", fmt.Errorf("la risorsa 0x%08x non è una stringa", id)
		}
	}
	return "", fmt.Errorf("riferimenti circolari nella risorsa 0x%08x", id)
}

// value returns the value of resource id.
func (t *Table) value(id uint32) (typ uint8, data uint32, err error) {
	pkg, typeID, entry := uint8(id>>24), uint8(id>>16), int(id&0xFFFF)
	found := false
	for _, c := range t.packages[pkg] {
		if c.id != typeID {
			continue
		}
		vt, vd, ok := entryValue(c.chunk, entry)
		if !ok {
			continue
		}
		if isDefaultConfig(c.chunk) {
			return vt, vd, nil
		}
		if !found {
			typ, data, found = vt, vd, true
		}
	}
	if !found {
		return 0, 0, fmt.Errorf("risorsa 0x%08x non trovata", id)
	}
	return typ, data, nil
}

// isDefaultConfig reports whether a type chunk has the default configuration
// (every field of ResTable_config is zero).
func isDefaultConfig(c chunk) bool {
	cfg := c.data[20:c.headerSize]
	size := int(le.Uint32(cfg))
	if size < 4 || size > len(cfg) {
		return false
	}
	for _, v := range cfg[4:size] {
		if v != 0 {
			return false
		}
	}
	return true
}

// entryValue returns the simple value stored for entry in a type chunk.
func entryValue(c chunk, entry int) (typ uint8, data uint32, ok bool) {
	d := c.data
	flags := d[9]
	count := int(le.Uint32(d[12:]))
	entriesStart := int(le.Uint32(d[16:]))
	idx := c.headerSize
	off := -1
	switch {
	case flags&typeFlagSparse != 0:
		for i := 0; i < count && idx+4*i+4 <= len(d); i++ {
			if int(le.Uint16(d[idx+4*i:])) == entry {
				off = 4 * int(le.Uint16(d[idx+4*i+2:]))
				break
			}
		}
	case entry >= count:
	case flags&typeFlagOffset16 != 0:
		if p := idx + 2*entry; p+2 <= len(d) && le.Uint16(d[p:]) != 0xFFFF {
			off = 4 * int(le.Uint16(d[p:]))
		}
	default:
		if p := idx + 4*entry; p+4 <= len(d) && le.Uint32(d[p:]) != NoIndex {
			off = int(le.Uint32(d[p:]))
		}
	}
	if off < 0 {
		return 0, 0, false
	}
	e := entriesStart + off
	if e+8 > len(d) {
		return 0, 0, false
	}
	eflags := le.Uint16(d[e+2:])
	if eflags&entryFlagCompact != 0 {
		return uint8(eflags >> 8), le.Uint32(d[e+4:]), true
	}
	if eflags&entryFlagComplex != 0 {
		return 0, 0, false // a bag (style, array...), not a simple value
	}
	v := e + int(le.Uint16(d[e:]))
	if v+8 > len(d) {
		return 0, 0, false
	}
	return d[v+3], le.Uint32(d[v+4:]), true
}
