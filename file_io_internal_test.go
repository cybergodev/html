package html

// file_io_internal_test.go — in-package tests for the file-read path,
// gathered from the former single-topic files (plus
// filename_dots_regression_test.go):
//
//   - the read-side MaxInputSize cap (readBounded), which bounds file reads
//     that the Stat-based pre-checks cannot see: non-regular files (FIFOs,
//     devices) report an implausible Stat size, and a regular file can grow
//     between its Stat and the read;
//   - the path-traversal guard matches whole ".." path components, not the
//     ".." substring, so a legal "notes..txt" filename is not rejected as a
//     traversal attack (D-002 P1) while genuine parent-directory references
//     still are.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReadBounded covers the byte ceiling directly, on all platforms.
func TestReadBounded(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.MaxInputSize = 1024
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = p.Close() }()

	t.Run("empty input", func(t *testing.T) {
		t.Parallel()

		data, err := p.readBounded(strings.NewReader(""), "x.html", "ExtractFromFile")
		if err != nil {
			t.Fatalf("readBounded(empty) error = %v", err)
		}
		if len(data) != 0 {
			t.Errorf("readBounded(empty) = %d bytes, want 0", len(data))
		}
	})

	t.Run("exactly at limit", func(t *testing.T) {
		t.Parallel()

		data, err := p.readBounded(strings.NewReader(strings.Repeat("a", 1024)), "x.html", "ExtractFromFile")
		if err != nil {
			t.Fatalf("readBounded(at limit) error = %v", err)
		}
		if len(data) != 1024 {
			t.Errorf("readBounded(at limit) = %d bytes, want 1024", len(data))
		}
	})

	t.Run("one byte over limit", func(t *testing.T) {
		t.Parallel()

		_, err := p.readBounded(strings.NewReader(strings.Repeat("a", 1025)), "x.html", "ExtractFromFile")
		if !errors.Is(err, ErrInputTooLarge) {
			t.Errorf("readBounded(over limit) error = %v, want ErrInputTooLarge", err)
		}
	})

	t.Run("far over limit reads at most limit+1", func(t *testing.T) {
		t.Parallel()

		_, err := p.readBounded(strings.NewReader(strings.Repeat("a", 1<<20)), "x.html", "ExtractFromFile")
		if !errors.Is(err, ErrInputTooLarge) {
			t.Errorf("readBounded(1MB over 1KB limit) error = %v, want ErrInputTooLarge", err)
		}
		var inputErr *InputError
		if !errors.As(err, &inputErr) || inputErr.Size != 1025 {
			t.Errorf("error should report the capped size 1025, got %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// Path-traversal guard — whole ".." components only (D-002 P1)
// ---------------------------------------------------------------------------

func TestDoubleDotFilenameNotFlaggedAsTraversal(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "notes..txt")
	if err := os.WriteFile(path, []byte("<html><body><p>dots are legal</p></body></html>"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	p, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	res, err := p.ExtractFromFile(path)
	if err != nil {
		t.Fatalf("legal double-dot filename rejected as traversal: %v", err)
	}
	if !strings.Contains(res.Text, "dots are legal") {
		t.Errorf("Text = %q, want it to contain the file content", res.Text)
	}
}

func TestParentDirectoryReferenceStillRejected(t *testing.T) {
	t.Parallel()

	p, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	for _, path := range []string{"../escape.html", "..\\escape.html", "subdir/../../escape.html"} {
		if _, err := p.ExtractFromFile(path); err == nil {
			t.Errorf("ExtractFromFile(%q) succeeded, want traversal rejection", path)
		}
	}
}

// ---------------------------------------------------------------------------
// validateAndReadFile edge paths — empty path and non-regular targets
// ---------------------------------------------------------------------------

// TestValidateAndReadFileEdgePaths covers the remaining validateAndReadFile /
// readBounded error branches through the public ExtractFromFile entry: an
// empty path, and a path that names a directory (a non-regular file whose
// Stat size is implausible, so the read itself must fail with a *FileError
// rather than succeed or panic).
func TestValidateAndReadFileEdgePaths(t *testing.T) {
	t.Parallel()

	p, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	t.Run("empty path", func(t *testing.T) {
		_, err := p.ExtractFromFile("")
		if !errors.Is(err, ErrInvalidFilePath) {
			t.Errorf("ExtractFromFile(\"\") = %v, want ErrInvalidFilePath", err)
		}
	})

	t.Run("directory path", func(t *testing.T) {
		dir := t.TempDir()
		_, err := p.ExtractFromFile(dir)
		var fileErr *FileError
		if !errors.As(err, &fileErr) {
			t.Errorf("ExtractFromFile(directory) = %v, want a *FileError", err)
		}
	})
}

// TestReadContainedDirectoryInsideBaseDir extends the AllowedBaseDir path
// with a non-regular target inside the allowed tree: containment must resolve
// (or fail) without panic, and the caller must receive a *FileError either
// way — never raw OS error text.
func TestReadContainedDirectoryInsideBaseDir(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	cfg := DefaultConfig()
	cfg.AllowedBaseDir = base
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	// A subdirectory of the allowed base: opening it is legal, but reading it
	// as a file must surface as a *FileError.
	sub := filepath.Join(base, "subdir")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	_, err = p.ExtractFromFile(sub)
	var fileErr *FileError
	if !errors.As(err, &fileErr) {
		t.Errorf("ExtractFromFile(subdirectory inside base) = %v, want a *FileError", err)
	}
}

// TestReadContainedOpenErrors pins readContained's os.Open error branches:
// a non-existent path maps to the ErrFileNotFound sentinel, and an invalid
// path name (NUL byte) surfaces as a read error rather than panicking.
func TestReadContainedOpenErrors(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.AllowedBaseDir = t.TempDir()
	p, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	if _, err := p.readContained("no-such-file.html", "ExtractFromFile"); !errors.Is(err, ErrFileNotFound) {
		t.Errorf("readContained(non-existent) = %v, want ErrFileNotFound", err)
	}
	if _, err := p.readContained("bad\x00name.html", "ExtractFromFile"); err == nil {
		t.Error("readContained(NUL in name) = nil error, want an open error")
	}
}
