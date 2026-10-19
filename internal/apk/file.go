package apk

import (
	"os"
	"path/filepath"
)

// writeFile writes data through a temporary file in the same folder, so a
// failure never leaves a half-written APK at name.
func writeFile(name string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(name), ".apktool-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), name)
}
