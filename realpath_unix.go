//go:build !windows

package html

import (
	"fmt"
	"os"
	"path/filepath"
)

// realPath returns the true on-disk path of the already-open file f.
//
// On Unix, symlinks are resolved by reading the open file descriptor's link,
// which is race-free: the handle is already open, so swapping a path component
// after this point cannot change which inode is read. /proc/self/fd (Linux) and
// /dev/fd (macOS, BSDs) both expose the link. If neither is available, the
// path-based filepath.EvalSymlinks is used as a fallback (still symlink-safe,
// with only a minor TOCTOU window).
func realPath(f *os.File) (string, error) {
	fd := f.Fd()
	for _, root := range []string{"/proc/self/fd", "/dev/fd"} {
		if real, err := os.Readlink(fmt.Sprintf("%s/%d", root, fd)); err == nil {
			return filepath.Clean(real), nil
		}
	}
	// Fallback: resolve the path used to open the file.
	real, err := filepath.EvalSymlinks(f.Name())
	if err != nil {
		return "", err
	}
	return filepath.Abs(real)
}
