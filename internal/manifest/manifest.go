// Package manifest reads and changes the package name and the label of a
// compiled AndroidManifest.xml.
package manifest

import (
	"errors"
	"fmt"
	"strings"

	"apktool/internal/binres"
)

const androidNS = "http://schemas.android.com/apk/res/android"

// Resource IDs of the android: attributes used here (android.R.attr).
const (
	attrLabel               = 0x01010001
	attrName                = 0x01010003
	attrManageSpaceActivity = 0x01010004
	attrAuthorities         = 0x01010018
	attrTargetActivity      = 0x01010202
	attrBackupAgent         = 0x0101027f
)

// classAttrs lists, per tag, the attributes holding a class name that may be
// relative to the package (".MainActivity").
var classAttrs = map[string][]uint32{
	"application":     {attrName, attrBackupAgent, attrManageSpaceActivity},
	"activity":        {attrName},
	"activity-alias":  {attrName, attrTargetActivity},
	"service":         {attrName},
	"receiver":        {attrName},
	"provider":        {attrName},
	"instrumentation": {attrName},
}

var permissionTags = map[string]bool{
	"permission":             true,
	"permission-group":       true,
	"permission-tree":        true,
	"uses-permission":        true,
	"uses-permission-sdk-23": true,
}

// Manifest is a parsed AndroidManifest.xml.
type Manifest struct {
	x        *binres.XML
	root     *binres.Element // <manifest>
	app      *binres.Element // <application>, may be nil
	launcher *binres.Element // launcher activity, may be nil
}

// Parse decodes a compiled AndroidManifest.xml.
func Parse(b []byte) (*Manifest, error) {
	x, err := binres.ParseXML(b)
	if err != nil {
		return nil, fmt.Errorf("AndroidManifest.xml: %w", err)
	}
	m := &Manifest{x: x}
	m.findElements()
	if m.root == nil || m.Package() == "" {
		return nil, errors.New("package name non trovato nel manifest")
	}
	return m, nil
}

// findElements locates <manifest>, <application> and the first activity
// whose intent filter has action MAIN and category LAUNCHER.
func (m *Manifest) findElements() {
	var stack []*binres.Element
	var component *binres.Element // activity being scanned
	componentDepth := 0
	hasMain, hasLauncher := false, false
	for _, n := range m.x.Nodes {
		switch {
		case n.Start != nil:
			e := n.Start
			stack = append(stack, e)
			switch tag := m.x.ElementName(e); {
			case tag == "manifest" && m.root == nil:
				m.root = e
			case tag == "application" && m.app == nil:
				m.app = e
			case (tag == "activity" || tag == "activity-alias") && component == nil:
				component, componentDepth = e, len(stack)
				hasMain, hasLauncher = false, false
			case tag == "action" && component != nil:
				hasMain = hasMain || m.stringAttr(e, attrName) == "android.intent.action.MAIN"
			case tag == "category" && component != nil:
				hasLauncher = hasLauncher || m.stringAttr(e, attrName) == "android.intent.category.LAUNCHER"
			}
		case n.End && len(stack) > 0:
			if component != nil && len(stack) == componentDepth {
				if hasMain && hasLauncher && m.launcher == nil {
					m.launcher = component
				}
				component = nil
			}
			stack = stack[:len(stack)-1]
		}
	}
}

func (m *Manifest) stringAttr(e *binres.Element, id uint32) string {
	if a := m.x.FindAttr(e, id); a != nil {
		s, _ := m.x.StringValue(a)
		return s
	}
	return ""
}

// Package returns the package name.
func (m *Manifest) Package() string {
	if a := m.x.FindPlainAttr(m.root, "package"); a != nil {
		s, _ := m.x.StringValue(a)
		return s
	}
	return ""
}

// Label returns the visible name of the app: the label of the launcher
// activity, or else the one of the application. String resources are
// resolved with table, which may be nil. It returns "" if there is no label.
func (m *Manifest) Label(table *binres.Table) string {
	for _, e := range []*binres.Element{m.launcher, m.app} {
		if e == nil {
			continue
		}
		a := m.x.FindAttr(e, attrLabel)
		if a == nil {
			continue
		}
		if s, ok := m.x.StringValue(a); ok {
			return s
		}
		if a.Type == binres.TypeReference && table != nil {
			if s, err := table.String(a.Data); err == nil {
				return s
			}
		}
		return fmt.Sprintf("@0x%08x", a.Data)
	}
	return ""
}

// SetPackage changes the package name, like aapt's --rename-manifest-package:
// relative class names are made absolute with the old package, so the code
// keeps working. Provider authorities and permissions that start with the old
// package are renamed too, otherwise the APK cannot be installed next to the
// original one.
func (m *Manifest) SetPackage(pkg string) {
	old := m.Package()
	if pkg == old {
		return
	}
	for _, n := range m.x.Nodes {
		e := n.Start
		if e == nil {
			continue
		}
		tag := m.x.ElementName(e)
		for _, id := range classAttrs[tag] {
			m.update(e, id, func(v string) string { return qualify(old, v) })
		}
		if tag == "provider" {
			m.update(e, attrAuthorities, func(v string) string {
				parts := strings.Split(v, ";")
				for i, p := range parts {
					parts[i] = swapPrefix(old, pkg, p, true)
				}
				return strings.Join(parts, ";")
			})
		}
		if permissionTags[tag] {
			m.update(e, attrName, func(v string) string { return swapPrefix(old, pkg, v, false) })
		}
	}
	m.x.SetString(m.x.FindPlainAttr(m.root, "package"), pkg)
}

// update replaces the string attribute id of e with change(value).
func (m *Manifest) update(e *binres.Element, id uint32, change func(string) string) {
	a := m.x.FindAttr(e, id)
	if a == nil {
		return
	}
	if v, ok := m.x.StringValue(a); ok {
		if nv := change(v); nv != v {
			m.x.SetString(a, nv)
		}
	}
}

// qualify turns a class name relative to pkg into an absolute one.
func qualify(pkg, class string) string {
	switch {
	case strings.HasPrefix(class, "."):
		return pkg + class
	case class != "" && !strings.Contains(class, "."):
		return pkg + "." + class
	}
	return class
}

// swapPrefix replaces the package old with pkg at the start of v.
// If exact is true, v equal to old is replaced as well.
func swapPrefix(old, pkg, v string, exact bool) string {
	if exact && v == old {
		return pkg
	}
	if strings.HasPrefix(v, old+".") {
		return pkg + v[len(old):]
	}
	return v
}

// SetLabel sets the label of the application, and of the launcher activity
// if it has its own. The label becomes a plain string, so it is the same in
// every language.
func (m *Manifest) SetLabel(label string) error {
	if m.app == nil {
		return errors.New("il manifest non ha l'elemento <application>")
	}
	m.x.SetAttrString(m.app, androidNS, "label", attrLabel, label)
	if m.launcher != nil {
		if a := m.x.FindAttr(m.launcher, attrLabel); a != nil {
			m.x.SetString(a, label)
		}
	}
	return nil
}

// Encode returns the compiled manifest.
func (m *Manifest) Encode() []byte { return m.x.Encode() }
