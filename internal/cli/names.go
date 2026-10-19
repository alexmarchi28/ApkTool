package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// isApkFile reports whether path is an existing file with .apk extension.
func isApkFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir() && strings.EqualFold(filepath.Ext(path), ".apk")
}

// fileNameFor returns "{label}_{YYYYMMDD}.apk", with the label in lower case
// and every run of spaces and punctuation turned into one underscore.
func fileNameFor(label string, date time.Time) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(strings.TrimSpace(label)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastUnderscore = false
		} else if !lastUnderscore && b.Len() > 0 {
			b.WriteRune('_')
			lastUnderscore = true
		}
	}
	name := strings.Trim(b.String(), "_")
	if name == "" {
		name = "app"
	}
	return name + "_" + date.Format("20060102") + ".apk"
}

// uniquePath returns p, or p with "_2", "_3", ... before the extension if
// a file with that name already exists.
func uniquePath(p string) string {
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	c := p
	for i := 2; exists(c); i++ {
		c = fmt.Sprintf("%s_%d%s", base, i, ext)
	}
	return c
}

// exists reports whether something is at path. A path that cannot be
// checked counts as free: writing there will report the real problem.
func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// cleanPath removes the quotes added when a file is dragged onto the console.
func cleanPath(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'`)
	if strings.HasPrefix(s, "& ") { // PowerShell drag & drop
		s = strings.Trim(strings.TrimSpace(s[2:]), `"'`)
	}
	return s
}
