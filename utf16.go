package main

import "unicode/utf16"

// utf16Encode converts a Go string (which may contain NUL separators) to UTF-16.
func utf16Encode(s string) []uint16 { return utf16.Encode([]rune(s)) }
