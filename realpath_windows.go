//go:build windows

package html

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// kernel32 holds a lazy reference to kernel32.dll for the final-path lookup.
var kernel32 = syscall.NewLazyDLL("kernel32.dll")

// procGetFinalPathNameByHandle is the entry point that resolves an open file
// handle to its real on-disk path, following symlinks and junctions.
var procGetFinalPathNameByHandle = kernel32.NewProc("GetFinalPathNameByHandleW")

// realPath returns the true on-disk path of the already-open file f.
//
// On Windows, filepath.EvalSymlinks does NOT resolve directory junctions
// (mount-point reparse points), which require no privilege to create and are
// therefore the primary AllowedBaseDir bypass on this platform. The OS handle
// API GetFinalPathNameByHandle resolves symlinks, junctions, and all other name
// surrogates to the actual location, so containment can be checked against the
// real path. The returned path is cleaned and has its \\?\ extended-length
// prefix removed so it compares against ordinary absolute paths.
func realPath(f *os.File) (string, error) {
	var buf [32768]uint16
	n, _, callErr := procGetFinalPathNameByHandle.Call(
		uintptr(f.Fd()),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		0, // VOLUME_NAME_DOS | FILE_NAME_NORMALIZED -> yields "\\?\C:\..."
	)
	// GetFinalPathNameByHandleW returns the path length excluding the NUL
	// terminator on success. Per MSDN, n == 0 signals an error; if the buffer is
	// too small, n is the required length (including the terminator) and exceeds
	// len(buf), in which case buf[:n] would index past the array. Guard both
	// rather than slicing out of bounds.
	if n == 0 || n > uintptr(len(buf)) {
		if callErr == nil {
			callErr = syscall.ERROR_INSUFFICIENT_BUFFER
		}
		return "", callErr
	}
	return normalizeWindowsPath(syscall.UTF16ToString(buf[:n])), nil
}

// normalizeWindowsPath strips the \\?\ (and \\?\UNC\) extended-length prefix
// returned by GetFinalPathNameByHandle and cleans the result, yielding a path in
// the same form as filepath.Abs.
func normalizeWindowsPath(p string) string {
	const (
		uncPrefix = `\\?\UNC\`
		dosPrefix = `\\?\`
	)
	switch {
	case strings.HasPrefix(p, uncPrefix):
		return filepath.Clean(`\\` + p[len(uncPrefix):])
	case strings.HasPrefix(p, dosPrefix):
		return filepath.Clean(p[len(dosPrefix):])
	}
	return filepath.Clean(p)
}
