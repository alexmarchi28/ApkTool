package platform

import (
	"bufio"
	"io"
	"strings"
	"testing"
)

func TestReadLine(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("uno\r\ndue\n\nàè 😀\nultima"))
	for _, want := range []string{"uno", "due", "", "àè 😀", "ultima"} {
		got, err := readLine(r)
		if err != nil || got != want {
			t.Fatalf("readLine = %q, %v; want %q", got, err, want)
		}
	}
	for i := 0; i < 2; i++ {
		if got, err := readLine(r); err != io.EOF || got != "" {
			t.Errorf("after the end: %q, %v; want io.EOF", got, err)
		}
	}
}

func TestNewReadsFromItsInput(t *testing.T) {
	o := New(strings.NewReader("a\nb\n"))
	for _, want := range []string{"a", "b"} {
		if got, err := o.ReadLine(); err != nil || got != want {
			t.Fatalf("ReadLine = %q, %v; want %q", got, err, want)
		}
	}
	if _, err := o.ReadLine(); err != io.EOF {
		t.Errorf("err = %v, want io.EOF", err)
	}
	if o.DownloadsDir() == "" {
		t.Error("no Downloads folder")
	}
}
