package binres

import (
	"errors"
	"sort"
)

// Value types of Res_value used here.
const (
	TypeReference = 0x01
	TypeString    = 0x03
	TypeIntDec    = 0x10
	TypeIntHex    = 0x11
)

// XML is a compiled Android XML document. Elements and their attributes can
// be read and changed; every other node is kept byte for byte.
type XML struct {
	Pool   *StringPool
	ResMap []uint32 // ResMap[i] is the attribute resource ID of string i
	Nodes  []Node
}

// Node is one XML node. Start is set for start elements, End for end
// elements; other nodes (namespaces, text) only carry their raw bytes.
type Node struct {
	Start *Element
	End   bool
	raw   []byte
}

// Element is a start tag.
type Element struct {
	Name  uint32 // string index
	Attrs []Attr

	header []byte // node header and attribute extension, without attributes
	// 1-based positions of the id, class and style attributes (0 = none)
	idIndex, classIndex, styleIndex uint16
}

// Attr is an attribute of an element.
type Attr struct {
	NS, Name, Raw uint32 // string indexes (NoIndex if missing)
	Type          uint8
	Data          uint32
}

const attrSize = 20

// ParseXML decodes a compiled XML document.
func ParseXML(b []byte) (*XML, error) {
	root, err := readChunk(b)
	if err != nil || root.typ != typeXML {
		return nil, errors.New("il file non è un XML Android compilato")
	}
	x := &XML{}
	for off := root.headerSize; off < len(root.data); {
		c, err := readChunk(root.data[off:])
		if err != nil {
			return nil, err
		}
		off += len(c.data)
		switch c.typ {
		case typeStringPool:
			if x.Pool != nil {
				return nil, errors.New("XML con più string pool")
			}
			if x.Pool, err = parseStringPool(c); err != nil {
				return nil, err
			}
		case typeXMLResMap:
			for i := c.headerSize; i+4 <= len(c.data); i += 4 {
				x.ResMap = append(x.ResMap, le.Uint32(c.data[i:]))
			}
		case typeXMLStart:
			e, err := parseElement(c)
			if err != nil {
				return nil, err
			}
			x.Nodes = append(x.Nodes, Node{Start: e})
		case typeXMLEnd:
			x.Nodes = append(x.Nodes, Node{End: true, raw: c.data})
		default:
			x.Nodes = append(x.Nodes, Node{raw: c.data})
		}
	}
	if x.Pool == nil {
		return nil, errors.New("XML senza string pool")
	}
	return x, nil
}

func parseElement(c chunk) (*Element, error) {
	ext := c.headerSize
	if len(c.data) < ext+attrSize {
		return nil, errTruncated
	}
	start := int(le.Uint16(c.data[ext+8:]))
	size := int(le.Uint16(c.data[ext+10:]))
	count := int(le.Uint16(c.data[ext+12:]))
	if start < attrSize || size != attrSize || ext+start+count*size > len(c.data) {
		return nil, errors.New("elemento XML non valido")
	}
	e := &Element{
		Name:       le.Uint32(c.data[ext+4:]),
		header:     c.data[:ext+start],
		idIndex:    le.Uint16(c.data[ext+14:]),
		classIndex: le.Uint16(c.data[ext+16:]),
		styleIndex: le.Uint16(c.data[ext+18:]),
	}
	for i := 0; i < count; i++ {
		a := c.data[ext+start+i*size:]
		e.Attrs = append(e.Attrs, Attr{
			NS:   le.Uint32(a),
			Name: le.Uint32(a[4:]),
			Raw:  le.Uint32(a[8:]),
			Type: a[15],
			Data: le.Uint32(a[16:]),
		})
	}
	return e, nil
}

// Encode serializes the document.
func (x *XML) Encode() []byte {
	out := make([]byte, 8)
	le.PutUint16(out[0:], typeXML)
	le.PutUint16(out[2:], 8)
	out = append(out, x.Pool.encode()...)
	if len(x.ResMap) > 0 {
		m := make([]byte, 8, 8+4*len(x.ResMap))
		le.PutUint16(m[0:], typeXMLResMap)
		le.PutUint16(m[2:], 8)
		le.PutUint32(m[4:], uint32(8+4*len(x.ResMap)))
		for _, id := range x.ResMap {
			m = le.AppendUint32(m, id)
		}
		out = append(out, m...)
	}
	for _, n := range x.Nodes {
		if n.Start != nil {
			out = append(out, n.Start.encode()...)
		} else {
			out = append(out, n.raw...)
		}
	}
	le.PutUint32(out[4:], uint32(len(out)))
	return out
}

func (e *Element) encode() []byte {
	b := append([]byte(nil), e.header...)
	ext := int(le.Uint16(b[2:]))
	le.PutUint16(b[ext+12:], uint16(len(e.Attrs)))
	le.PutUint16(b[ext+14:], e.idIndex)
	le.PutUint16(b[ext+16:], e.classIndex)
	le.PutUint16(b[ext+18:], e.styleIndex)
	for _, a := range e.Attrs {
		b = le.AppendUint32(b, a.NS)
		b = le.AppendUint32(b, a.Name)
		b = le.AppendUint32(b, a.Raw)
		b = le.AppendUint16(b, 8) // Res_value size
		b = append(b, 0, a.Type)
		b = le.AppendUint32(b, a.Data)
	}
	le.PutUint32(b[4:], uint32(len(b)))
	return b
}

// ElementName returns the tag name of e.
func (x *XML) ElementName(e *Element) string { return x.Pool.Get(e.Name) }

// AttrID returns the resource ID of the attribute name (0 if it has none).
func (x *XML) AttrID(a Attr) uint32 {
	if int64(a.Name) < int64(len(x.ResMap)) {
		return x.ResMap[a.Name]
	}
	return 0
}

// FindAttr returns the attribute of e with the given resource ID, or nil.
func (x *XML) FindAttr(e *Element, id uint32) *Attr {
	for i := range e.Attrs {
		if x.AttrID(e.Attrs[i]) == id {
			return &e.Attrs[i]
		}
	}
	return nil
}

// FindPlainAttr returns the attribute of e without namespace and resource ID
// (such as "package" on <manifest>), or nil.
func (x *XML) FindPlainAttr(e *Element, name string) *Attr {
	for i := range e.Attrs {
		a := &e.Attrs[i]
		if a.NS == NoIndex && x.AttrID(*a) == 0 && x.Pool.Get(a.Name) == name {
			return a
		}
	}
	return nil
}

// StringValue returns the string held by a, and false if a is not a string.
func (x *XML) StringValue(a *Attr) (string, bool) {
	if a.Type == TypeString {
		return x.Pool.Get(a.Data), true
	}
	return "", false
}

// SetString makes a a plain string attribute with value s.
func (x *XML) SetString(a *Attr, s string) {
	i := x.Pool.Add(s)
	a.Raw, a.Type, a.Data = i, TypeString, i
}

// SetAttrString sets the string attribute with resource ID id on e, adding the
// attribute if missing. ns is the namespace URI string of the attribute.
func (x *XML) SetAttrString(e *Element, ns string, name string, id uint32, s string) {
	if a := x.FindAttr(e, id); a != nil {
		x.SetString(a, s)
		return
	}
	a := Attr{NS: x.Pool.Add(ns), Name: x.attrName(name, id)}
	x.SetString(&a, s)
	// Attributes must stay sorted by resource ID.
	pos := sort.Search(len(e.Attrs), func(i int) bool {
		other := x.AttrID(e.Attrs[i])
		return other == 0 || other > id
	})
	e.Attrs = append(e.Attrs, Attr{})
	copy(e.Attrs[pos+1:], e.Attrs[pos:])
	e.Attrs[pos] = a
	for _, idx := range []*uint16{&e.idIndex, &e.classIndex, &e.styleIndex} {
		if int(*idx) > pos {
			*idx++
		}
	}
}

// attrName returns the index of a string mapped to resource ID id, adding one
// to the pool and to the resource map if needed.
func (x *XML) attrName(name string, id uint32) uint32 {
	for i, v := range x.ResMap {
		if v == id {
			return uint32(i)
		}
	}
	// The resource map covers the first strings of the pool: extend it up to
	// the new string, mapping the strings in between to "no resource".
	x.Pool.Strings = append(x.Pool.Strings, name)
	idx := len(x.Pool.Strings) - 1
	for len(x.ResMap) < idx {
		x.ResMap = append(x.ResMap, 0)
	}
	x.ResMap = append(x.ResMap, id)
	return uint32(idx)
}
