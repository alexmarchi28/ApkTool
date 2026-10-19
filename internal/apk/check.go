package apk

import (
	"archive/zip"
	"bytes"
	"fmt"

	"apktool/internal/apksign"
)

// check reads back the new APK before it is saved: the signature must be
// valid and the manifest must hold the requested values.
func check(out []byte, pkg, label string) error {
	if err := apksign.VerifyV2(out); err != nil {
		return err
	}
	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		return err
	}
	m, table, err := load(zr)
	if err != nil {
		return err
	}
	if got := m.Package(); got != pkg {
		return fmt.Errorf("package %q invece di %q", got, pkg)
	}
	if got := m.Label(table); got != label {
		return fmt.Errorf("etichetta %q invece di %q", got, label)
	}
	return nil
}
