package manifest

import (
	"archive/zip"
	"io"
	"testing"

	"apktool/internal/binres"
)

func readTestEntry(t *testing.T, apk, name string) []byte {
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

func parse(t *testing.T, apk string) *Manifest {
	t.Helper()
	m, err := Parse(readTestEntry(t, apk, "AndroidManifest.xml"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// reparse checks that the manifest still decodes after the changes.
func reparse(t *testing.T, m *Manifest) *Manifest {
	t.Helper()
	m2, err := Parse(m.Encode())
	if err != nil {
		t.Fatal(err)
	}
	return m2
}

// values returns the string values of attribute id on every element named tag.
func values(m *Manifest, tag string, id uint32) []string {
	var out []string
	for _, n := range m.x.Nodes {
		if n.Start != nil && m.x.ElementName(n.Start) == tag {
			out = append(out, m.stringAttr(n.Start, id))
		}
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestPackageAndLabel(t *testing.T) {
	m := parse(t, "demo.apk")
	if got := m.Package(); got != "com.example.demo" {
		t.Errorf("Package = %q", got)
	}
	table, err := binres.ParseTable(readTestEntry(t, "demo.apk", "resources.arsc"))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Label(table); got != "Demo App" {
		t.Errorf("Label = %q, want Demo App", got)
	}
	if got := m.Label(nil); got != "@0x7f010000" {
		t.Errorf("Label without resources = %q", got)
	}
}

func TestLabelMissing(t *testing.T) {
	if got := parse(t, "nolabel.apk").Label(nil); got != "" {
		t.Errorf("Label = %q, want empty", got)
	}
}

func TestLauncherIsFound(t *testing.T) {
	m := parse(t, "demo.apk")
	if m.launcher == nil || m.stringAttr(m.launcher, attrName) != ".MainActivity" {
		t.Error("launcher activity not found")
	}
	m = parse(t, "nolabel.apk")
	if m.launcher == nil || m.stringAttr(m.launcher, attrName) != ".Alias" {
		t.Error("launcher activity-alias not found")
	}
}

func TestSetPackage(t *testing.T) {
	m := parse(t, "demo.apk")
	m.SetPackage("it.nuovo.app")
	m = reparse(t, m)

	if got := m.Package(); got != "it.nuovo.app" {
		t.Errorf("Package = %q", got)
	}
	checks := []struct {
		tag  string
		id   uint32
		want []string
	}{
		// Relative class names are made absolute with the OLD package: the code is unchanged.
		{"application", attrName, []string{"com.example.demo.DemoApp"}},
		{"activity", attrName, []string{"com.example.demo.MainActivity", "com.example.demo.Other"}},
		{"provider", attrName, []string{"androidx.core.content.FileProvider"}},
		{"provider", attrAuthorities, []string{"it.nuovo.app.fileprovider;it.nuovo.app.other"}},
		{"permission", attrName, []string{"it.nuovo.app.permission.C2D_MESSAGE"}},
		{"uses-permission", attrName, []string{"android.permission.INTERNET", "it.nuovo.app.permission.C2D_MESSAGE"}},
		{"action", attrName, []string{"android.intent.action.MAIN"}},
	}
	for _, c := range checks {
		if got := values(m, c.tag, c.id); !equal(got, c.want) {
			t.Errorf("%s %#x = %q, want %q", c.tag, c.id, got, c.want)
		}
	}
}

func TestSetPackageQualifiesEveryClassAttribute(t *testing.T) {
	m := parse(t, "nolabel.apk")
	m.SetPackage("it.x")
	m = reparse(t, m)
	const old = "org.sample.nolabel"
	checks := []struct {
		tag  string
		id   uint32
		want string
	}{
		{"application", attrName, old + ".App"},
		{"application", attrBackupAgent, old + ".Backup"},
		{"application", attrManageSpaceActivity, old + ".Space"},
		{"activity-alias", attrTargetActivity, old + ".Main"},
		{"service", attrName, old + ".Svc"},
		{"receiver", attrName, "org.other.Receiver"},
		{"provider", attrAuthorities, "it.x"},
	}
	for _, c := range checks {
		if got := values(m, c.tag, c.id); len(got) != 1 || got[0] != c.want {
			t.Errorf("%s %#x = %q, want %q", c.tag, c.id, got, c.want)
		}
	}
}

func TestSetPackageUnchanged(t *testing.T) {
	m := parse(t, "demo.apk")
	before := m.Encode()
	m.SetPackage(m.Package())
	if string(m.Encode()) != string(before) {
		t.Error("setting the same package must change nothing")
	}
}

func TestSetLabel(t *testing.T) {
	m := parse(t, "demo.apk")
	if err := m.SetLabel("Nuovo nome è 😀"); err != nil {
		t.Fatal(err)
	}
	m = reparse(t, m)
	if got := m.Label(nil); got != "Nuovo nome è 😀" {
		t.Errorf("Label = %q", got)
	}
	// Other activities keep their own label.
	if got := values(m, "activity", attrLabel); !equal(got, []string{"", "Altro"}) {
		t.Errorf("activity labels = %q", got)
	}
}

func TestSetLabelAddsMissingAttribute(t *testing.T) {
	m := parse(t, "nolabel.apk")
	if err := m.SetLabel("Etichetta"); err != nil {
		t.Fatal(err)
	}
	m = reparse(t, m)
	if got := m.Label(nil); got != "Etichetta" {
		t.Errorf("Label = %q", got)
	}
	if got := values(m, "activity-alias", attrLabel); !equal(got, []string{""}) {
		t.Errorf("a launcher without its own label must not get one, got %q", got)
	}
}

func TestSetLabelOnLauncher(t *testing.T) {
	m := parse(t, "nolabel.apk")
	m.x.SetAttrString(m.launcher, androidNS, "label", attrLabel, "Vecchia")
	if got := m.Label(nil); got != "Vecchia" {
		t.Fatalf("the launcher label must win, got %q", got)
	}
	m.SetLabel("Nuova")
	m = reparse(t, m)
	if got := values(m, "activity-alias", attrLabel); !equal(got, []string{"Nuova"}) {
		t.Errorf("launcher label = %q", got)
	}
	if got := values(m, "application", attrLabel); !equal(got, []string{"Nuova"}) {
		t.Errorf("application label = %q", got)
	}
}

func TestQualify(t *testing.T) {
	tests := []struct{ class, want string }{
		{".Main", "a.b.Main"},
		{"Main", "a.b.Main"},
		{"x.y.Main", "x.y.Main"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := qualify("a.b", tt.class); got != tt.want {
			t.Errorf("qualify(%q) = %q, want %q", tt.class, got, tt.want)
		}
	}
}

func TestSwapPrefix(t *testing.T) {
	tests := []struct {
		v     string
		exact bool
		want  string
	}{
		{"a.b", true, "n.p"},
		{"a.b", false, "a.b"},
		{"a.b.files", false, "n.p.files"},
		{"a.bc.files", false, "a.bc.files"},
		{"x.a.b", true, "x.a.b"},
	}
	for _, tt := range tests {
		if got := swapPrefix("a.b", "n.p", tt.v, tt.exact); got != tt.want {
			t.Errorf("swapPrefix(%q, %v) = %q, want %q", tt.v, tt.exact, got, tt.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	if _, err := Parse([]byte("not xml")); err == nil {
		t.Error("Parse(garbage) succeeded")
	}
}

// usesSDK returns the <uses-sdk> element of m.
func usesSDK(t *testing.T, m *Manifest) *binres.Element {
	t.Helper()
	for _, n := range m.x.Nodes {
		if n.Start != nil && m.x.ElementName(n.Start) == "uses-sdk" {
			return n.Start
		}
	}
	t.Fatal("<uses-sdk> not found")
	return nil
}

func TestMinSdk(t *testing.T) {
	if got := parse(t, "demo.apk").MinSdk(); got != 21 {
		t.Errorf("MinSdk = %d, want 21", got)
	}

	m := parse(t, "demo.apk")
	m.x.FindAttr(usesSDK(t, m), attrMinSdkVersion).Data = 15
	if got := reparse(t, m).MinSdk(); got != 15 {
		t.Errorf("MinSdk = %d, want 15", got)
	}

	// A preview codename means the newest platform.
	m = parse(t, "demo.apk")
	m.x.SetString(m.x.FindAttr(usesSDK(t, m), attrMinSdkVersion), "VanillaIceCream")
	if got := reparse(t, m).MinSdk(); got != 10000 {
		t.Errorf("MinSdk with a codename = %d, want 10000", got)
	}

	// Without the attribute Android assumes API level 1.
	m = parse(t, "demo.apk")
	e := usesSDK(t, m)
	e.Attrs = e.Attrs[1:]
	if got := reparse(t, m).MinSdk(); got != 1 {
		t.Errorf("MinSdk without attribute = %d, want 1", got)
	}
}

func TestValidPackage(t *testing.T) {
	valid := []string{"a.b", "it.azienda.app_test", "Com.X1.y", "a_.b_1", "a.b.c.d.e.f"}
	invalid := []string{"", "app", ".a.b", "a.b.", "a..b", "1a.b", "a.1b", "_a.b", "a._b", "a-b.c", "a.b c", " a.b", "a.b\n", "à.b", "a.b/c"}
	for _, s := range valid {
		if !ValidPackage(s) {
			t.Errorf("%q must be valid", s)
		}
	}
	for _, s := range invalid {
		if ValidPackage(s) {
			t.Errorf("%q must be invalid", s)
		}
	}
}
