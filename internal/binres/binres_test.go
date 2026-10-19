package binres

import (
	"archive/zip"
	"io"
	"strings"
	"testing"
)

// readTestEntry returns an entry of an APK in the repository's testdata.
func readTestEntry(t testing.TB, apk, name string) []byte {
	t.Helper()
	zr, err := zip.OpenReader("../../testdata/" + apk)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	f, err := zr.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestStringPoolRoundTrip(t *testing.T) {
	strs := []string{"", "a", "package", "àèìòù", "emoji 😀", strings.Repeat("x", 200), strings.Repeat("é", 300)}
	for _, utf8 := range []bool{false, true} {
		p := &StringPool{Strings: strs, utf8: utf8}
		c, err := readChunk(p.encode())
		if err != nil {
			t.Fatal(err)
		}
		got, err := parseStringPool(c)
		if err != nil {
			t.Fatalf("utf8=%v: %v", utf8, err)
		}
		if len(got.Strings) != len(strs) {
			t.Fatalf("utf8=%v: %d strings, want %d", utf8, len(got.Strings), len(strs))
		}
		for i := range strs {
			if got.Strings[i] != strs[i] {
				t.Errorf("utf8=%v: string %d = %q, want %q", utf8, i, got.Strings[i], strs[i])
			}
		}
		if got.utf8 != utf8 {
			t.Errorf("utf8 flag = %v, want %v", got.utf8, utf8)
		}
	}
}

func TestStringPoolAdd(t *testing.T) {
	p := &StringPool{Strings: []string{"a", "b"}}
	if i := p.Add("b"); i != 1 {
		t.Errorf("Add(existing) = %d, want 1", i)
	}
	if i := p.Add("c"); i != 2 || p.Get(2) != "c" {
		t.Errorf("Add(new) = %d, want 2", i)
	}
	if p.Get(99) != "" || p.Get(NoIndex) != "" {
		t.Error("Get out of range must return an empty string")
	}
}

func TestStringPoolKeepsStyledStrings(t *testing.T) {
	p := &StringPool{Strings: []string{"bold"}, styleOf: []uint32{0}, styles: []byte{0xff, 0xff, 0xff, 0xff}}
	if i := p.Add("bold"); i == 0 {
		t.Error("a styled string must not be reused for a plain value")
	}
}

func TestParseXMLRoundTrip(t *testing.T) {
	b := readTestEntry(t, "demo.apk", "AndroidManifest.xml")
	x, err := ParseXML(b)
	if err != nil {
		t.Fatal(err)
	}
	y, err := ParseXML(x.Encode())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(x.Pool.Strings, "|") != strings.Join(y.Pool.Strings, "|") {
		t.Error("strings changed after a round trip")
	}
	if len(x.ResMap) != len(y.ResMap) || len(x.Nodes) != len(y.Nodes) {
		t.Fatal("structure changed after a round trip")
	}
	for i := range x.Nodes {
		a, b := x.Nodes[i].Start, y.Nodes[i].Start
		if (a == nil) != (b == nil) {
			t.Fatalf("node %d changed kind", i)
		}
		if a != nil && (a.Name != b.Name || len(a.Attrs) != len(b.Attrs)) {
			t.Fatalf("element %d changed", i)
		}
	}
	if string(y.Encode()) != string(x.Encode()) {
		t.Error("encoding is not stable")
	}
}

func findElement(t *testing.T, x *XML, name string) *Element {
	t.Helper()
	for _, n := range x.Nodes {
		if n.Start != nil && x.ElementName(n.Start) == name {
			return n.Start
		}
	}
	t.Fatalf("element %s not found", name)
	return nil
}

func TestSetAttrStringInsertsSorted(t *testing.T) {
	x, err := ParseXML(readTestEntry(t, "nolabel.apk", "AndroidManifest.xml"))
	if err != nil {
		t.Fatal(err)
	}
	const label = 0x01010001
	app := findElement(t, x, "application")
	before := len(app.Attrs)
	x.SetAttrString(app, "http://schemas.android.com/apk/res/android", "label", label, "Nuova")

	x, err = ParseXML(x.Encode())
	if err != nil {
		t.Fatal(err)
	}
	app = findElement(t, x, "application")
	if len(app.Attrs) != before+1 {
		t.Fatalf("%d attributes, want %d", len(app.Attrs), before+1)
	}
	a := x.FindAttr(app, label)
	if a == nil {
		t.Fatal("label not added")
	}
	if s, ok := x.StringValue(a); !ok || s != "Nuova" {
		t.Errorf("label = %q, want Nuova", s)
	}
	var last uint32
	for _, a := range app.Attrs {
		id := x.AttrID(a)
		if id != 0 && id < last {
			t.Errorf("attributes not sorted by resource ID: %#x after %#x", id, last)
		}
		if id != 0 {
			last = id
		}
	}
}

func TestSetAttrStringUpdatesExisting(t *testing.T) {
	x, err := ParseXML(readTestEntry(t, "demo.apk", "AndroidManifest.xml"))
	if err != nil {
		t.Fatal(err)
	}
	app := findElement(t, x, "application")
	n := len(app.Attrs)
	x.SetAttrString(app, "http://schemas.android.com/apk/res/android", "label", 0x01010001, "X")
	if len(app.Attrs) != n {
		t.Error("an existing attribute must be replaced, not added")
	}
	if s, _ := x.StringValue(x.FindAttr(app, 0x01010001)); s != "X" {
		t.Errorf("label = %q, want X", s)
	}
}

func TestAttrNameExtendsResourceMap(t *testing.T) {
	x := &XML{Pool: &StringPool{Strings: []string{"name", "package", "manifest"}}, ResMap: []uint32{0x01010003}}
	i := x.attrName("label", 0x01010001)
	if i != 3 || len(x.ResMap) != 4 || x.ResMap[3] != 0x01010001 || x.ResMap[1] != 0 || x.ResMap[2] != 0 {
		t.Errorf("attrName = %d, ResMap = %#x", i, x.ResMap)
	}
	if j := x.attrName("label", 0x01010001); j != i {
		t.Errorf("second attrName = %d, want %d", j, i)
	}
}

func TestParseXMLRejectsGarbage(t *testing.T) {
	for _, b := range [][]byte{nil, []byte("hello world"), {3, 0, 8, 0, 255, 255, 255, 255}} {
		if _, err := ParseXML(b); err == nil {
			t.Errorf("ParseXML(%q) succeeded", b)
		}
	}
}

func TestTableString(t *testing.T) {
	tb, err := ParseTable(readTestEntry(t, "demo.apk", "resources.arsc"))
	if err != nil {
		t.Fatal(err)
	}
	// 0x7f010000 is string/app_name: "Demo App", "App di prova" in Italian.
	if s, err := tb.String(0x7f010000); err != nil || s != "Demo App" {
		t.Errorf("String = %q, %v; want the default value Demo App", s, err)
	}
	if s, err := tb.String(0x7f010001); err != nil || s != "x" {
		t.Errorf("String = %q, %v; want x", s, err)
	}
	for _, id := range []uint32{0x7f010099, 0x7f020000, 0x01010000} {
		if _, err := tb.String(id); err == nil {
			t.Errorf("String(%#x) succeeded", id)
		}
	}
}

func TestParseTableRejectsGarbage(t *testing.T) {
	for _, b := range [][]byte{nil, []byte("not a table"), {2, 0, 12, 0, 12, 0, 0, 0, 0, 0, 0, 0}} {
		if _, err := ParseTable(b); err == nil {
			t.Errorf("ParseTable(%q) succeeded", b)
		}
	}
}

// The parsers read untrusted files: they must return an error, never panic.
func FuzzParseXML(f *testing.F) {
	f.Add(readTestEntry(f, "demo.apk", "AndroidManifest.xml"))
	f.Add(readTestEntry(f, "nolabel.apk", "AndroidManifest.xml"))
	f.Fuzz(func(t *testing.T, b []byte) {
		x, err := ParseXML(b)
		if err != nil {
			return
		}
		for _, n := range x.Nodes {
			if n.Start != nil {
				x.SetAttrString(n.Start, "ns", "label", 0x01010001, "x")
			}
		}
		if _, err := ParseXML(x.Encode()); err != nil {
			t.Fatalf("re-encoded document does not parse: %v", err)
		}
	})
}

func FuzzParseTable(f *testing.F) {
	f.Add(readTestEntry(f, "demo.apk", "resources.arsc"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if tb, err := ParseTable(b); err == nil {
			tb.String(0x7f010000)
			tb.String(0x7f010001)
		}
	})
}

func TestStringPoolLongStrings(t *testing.T) {
	// A UTF-8 pool can only hold strings up to 0x7FFF bytes: a longer one
	// must still round-trip, so the pool is written as UTF-16 instead.
	long := strings.Repeat("é", 0x4000) // 0x8000 bytes in UTF-8
	huge := strings.Repeat("x", 0x9000) // more than 0x7FFF UTF-16 units
	for _, utf8 := range []bool{false, true} {
		p := &StringPool{Strings: []string{"a", long, huge}, utf8: utf8}
		c, err := readChunk(p.encode())
		if err != nil {
			t.Fatal(err)
		}
		got, err := parseStringPool(c)
		if err != nil {
			t.Fatalf("utf8=%v: %v", utf8, err)
		}
		for i, want := range p.Strings {
			if got.Strings[i] != want {
				t.Errorf("utf8=%v: string %d has %d bytes, want %d", utf8, i, len(got.Strings[i]), len(want))
			}
		}
	}
}

func TestDecodeStringRejectsTruncatedData(t *testing.T) {
	cases := []struct {
		b    []byte
		utf8 bool
	}{
		{nil, false},
		{[]byte{1}, false},                 // no room for the length
		{[]byte{2, 0, 'a', 0}, false},      // 2 units announced, 1 present
		{[]byte{0x00, 0x80, 0}, false},     // long length cut short
		{[]byte{0x81}, true},               // two-byte length cut short
		{[]byte{1}, true},                  // no byte length
		{[]byte{3, 3, 'a', 'b'}, true},     // 3 bytes announced, 2 present
		{[]byte{1, 0x81, 0x00, 'a'}, true}, // 0x100 bytes announced
	}
	for _, c := range cases {
		if s, err := decodeString(c.b, 0, c.utf8); err == nil {
			t.Errorf("decodeString(% x, utf8=%v) = %q, want an error", c.b, c.utf8, s)
		}
	}
	if _, err := decodeString([]byte{0, 0}, -1, false); err == nil {
		t.Error("negative offset accepted")
	}
}
