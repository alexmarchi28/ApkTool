//go:build windows

package platform

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	comdlg32 = syscall.NewLazyDLL("comdlg32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")

	pSetConsoleOutputCP   = kernel32.NewProc("SetConsoleOutputCP")
	pSetConsoleCP         = kernel32.NewProc("SetConsoleCP")
	pGetConsoleMode       = kernel32.NewProc("GetConsoleMode")
	pSetConsoleMode       = kernel32.NewProc("SetConsoleMode")
	pSetConsoleTitleW     = kernel32.NewProc("SetConsoleTitleW")
	pReadConsoleW         = kernel32.NewProc("ReadConsoleW")
	pGetConsoleWindow     = kernel32.NewProc("GetConsoleWindow")
	pGetOpenFileNameW     = comdlg32.NewProc("GetOpenFileNameW")
	pSHGetKnownFolderPath = shell32.NewProc("SHGetKnownFolderPath")
	pCoInitializeEx       = ole32.NewProc("CoInitializeEx")
	pCoTaskMemFree        = ole32.NewProc("CoTaskMemFree")
	pSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
)

// The file dialog needs COM on the thread that initialized it.
func init() { runtime.LockOSThread() }

// OS is the Windows platform: UTF-8 console with colors, Unicode input, the
// Explorer file dialog and the Downloads known folder.
type OS struct {
	in      *bufio.Reader
	console syscall.Handle // valid if the input is a console
	isTTY   bool
}

// New returns the platform, reading the answers of the user from in. If in
// is the console, it is read with ReadConsoleW for full Unicode support.
func New(in io.Reader) *OS {
	o := &OS{in: bufio.NewReader(in)}
	if f, ok := in.(*os.File); ok {
		o.console = syscall.Handle(f.Fd())
		o.isTTY = consoleMode(o.console) != nil
	}
	return o
}

func consoleMode(h syscall.Handle) *uint32 {
	var mode uint32
	if r, _, _ := pGetConsoleMode.Call(uintptr(h), uintptr(unsafe.Pointer(&mode))); r == 0 {
		return nil
	}
	return &mode
}

// SetupConsole switches the console to UTF-8 with ANSI colors, sets its
// title and initializes COM for the file dialog.
func (o *OS) SetupConsole() {
	pSetConsoleOutputCP.Call(65001)
	pSetConsoleCP.Call(65001)
	t, _ := syscall.UTF16PtrFromString("ApkTool")
	pSetConsoleTitleW.Call(uintptr(unsafe.Pointer(t)))
	for _, h := range []syscall.Handle{syscall.Stdout, syscall.Stderr} {
		if mode := consoleMode(h); mode != nil {
			pSetConsoleMode.Call(uintptr(h), uintptr(*mode|0x0004)) // ENABLE_VIRTUAL_TERMINAL_PROCESSING
		}
	}
	pCoInitializeEx.Call(0, 2) // COINIT_APARTMENTTHREADED
}

// ReadLine reads one line typed by the user.
func (o *OS) ReadLine() (string, error) {
	if !o.isTTY {
		return readLine(o.in)
	}
	var out []uint16
	buf := make([]uint16, 512)
	for {
		var n uint32
		r, _, _ := pReadConsoleW.Call(uintptr(o.console), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&n)), 0)
		if r == 0 || n == 0 {
			if len(out) == 0 {
				return "", io.EOF
			}
			break
		}
		out = append(out, buf[:n]...)
		if buf[n-1] == '\n' {
			break
		}
	}
	return strings.TrimRight(syscall.UTF16ToString(out), "\r\n"), nil
}

type openFileName struct {
	lStructSize       uint32
	hwndOwner         uintptr
	hInstance         uintptr
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	flagsEx           uint32
}

// PickHint is shown before PickAPK opens the file dialog.
func (o *OS) PickHint() string { return "Si apre Esplora file: scegli il file .apk da modificare." }

// PickAPK opens the Explorer file dialog on the Downloads folder.
func (o *OS) PickAPK() string {
	// The filter is a list of NUL-terminated strings ending with an empty one.
	filter := utf16.Encode([]rune("File APK (*.apk)\x00*.apk\x00Tutti i file (*.*)\x00*.*\x00\x00"))
	title, _ := syscall.UTF16PtrFromString("ApkTool - Seleziona l'APK da modificare")
	ext, _ := syscall.UTF16PtrFromString("apk")
	initial, _ := syscall.UTF16PtrFromString(o.DownloadsDir())
	file := make([]uint16, 4096)
	hwnd, _, _ := pGetConsoleWindow.Call()
	ofn := openFileName{
		hwndOwner:       hwnd,
		lpstrFilter:     &filter[0],
		nFilterIndex:    1,
		lpstrFile:       &file[0],
		nMaxFile:        uint32(len(file)),
		lpstrInitialDir: initial,
		lpstrTitle:      title,
		lpstrDefExt:     ext,
		flags:           0x00001000 | 0x00000800 | 0x00080000 | 0x00000008, // FILEMUSTEXIST|PATHMUSTEXIST|EXPLORER|NOCHANGEDIR
	}
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	r, _, _ := pGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if hwnd != 0 {
		pSetForegroundWindow.Call(hwnd)
	}
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(file)
}

// DownloadsDir returns the Downloads known folder, or ~/Downloads.
func (o *OS) DownloadsDir() string {
	// FOLDERID_Downloads {374DE290-123F-4565-9164-39C4925E467B}
	guid := [16]byte{0x90, 0xE2, 0x4D, 0x37, 0x3F, 0x12, 0x65, 0x45, 0x91, 0x64, 0x39, 0xC4, 0x92, 0x5E, 0x46, 0x7B}
	var p *uint16
	if r, _, _ := pSHGetKnownFolderPath.Call(uintptr(unsafe.Pointer(&guid[0])), 0, 0, uintptr(unsafe.Pointer(&p))); r == 0 && p != nil {
		defer pCoTaskMemFree.Call(uintptr(unsafe.Pointer(p)))
		if s := utf16PtrToString(p); s != "" {
			return s
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Downloads")
}

func utf16PtrToString(p *uint16) string {
	var u []uint16
	for ptr := unsafe.Pointer(p); ; ptr = unsafe.Add(ptr, 2) {
		c := *(*uint16)(ptr)
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return syscall.UTF16ToString(u)
}

// Reveal opens Explorer with the file selected.
func (o *OS) Reveal(path string) {
	exec.Command("explorer.exe", "/select,", path).Start()
}
