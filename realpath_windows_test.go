//go:build windows

package html

// realpath_windows_test.go — table-driven coverage for normalizeWindowsPath
// (realpath_windows.go): the \\?\UNC\ and \\?\ extended-length prefixes returned
// by GetFinalPathNameByHandle must be stripped so containment checks compare
// against ordinary absolute paths. realPath itself needs an open OS file handle,
// so its success path is exercised indirectly through the AllowedBaseDir tests
// in file_io_test.go; its error branch is driven here with a closed handle.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeWindowsPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"dos prefix stripped", `\\?\C:\Users\test\file.html`, `C:\Users\test\file.html`},
		{"unc prefix becomes share path", `\\?\UNC\server\share\file.html`, `\\server\share\file.html`},
		{"plain path cleaned unchanged", `C:\Users\test\..\test\file.html`, `C:\Users\test\file.html`},
		{"already plain path", `C:\a\b.html`, `C:\a\b.html`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeWindowsPath(tt.in); got != tt.want {
				t.Errorf("normalizeWindowsPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestRealPathSuccessAndClosedHandle exercises realPath both ways: a live
// handle resolves to the file's cleaned absolute path, and a handle that was
// closed before the lookup fails with an error rather than panicking or
// slicing out of bounds.
func TestRealPathSuccessAndClosedHandle(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "target.html")
	if err := os.WriteFile(path, []byte("<html><body><p>x</p></body></html>"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	real, err := realPath(f)
	if err != nil {
		t.Fatalf("realPath(live handle): %v", err)
	}
	// The \\?\ prefix (if any) is stripped and the path compares equal to the
	// ordinary absolute form — the invariant containment checks rely on.
	if want := filepath.Clean(path); real != want {
		t.Errorf("realPath = %q, want %q", real, want)
	}
	_ = f.Close()

	// Closed handle: the OS call fails (n == 0), which must surface as an
	// error from the guarded branch.
	closed, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open(2): %v", err)
	}
	_ = closed.Close()
	if _, err := realPath(closed); err == nil {
		t.Error("realPath(closed handle) = nil error, want an error")
	}
}
