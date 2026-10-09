//go:build windows

package html

// realpath_windows_test.go — table-driven coverage for normalizeWindowsPath
// (realpath_windows.go): the \\?\UNC\ and \\?\ extended-length prefixes returned
// by GetFinalPathNameByHandle must be stripped so containment checks compare
// against ordinary absolute paths. realPath itself needs an open OS file handle,
// so its success path is exercised indirectly through the AllowedBaseDir tests
// in file_io_test.go.

import "testing"

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
