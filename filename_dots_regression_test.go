package html

// filename_dots_regression_test.go — regression for the D-002 P1 fix: the
// path-traversal guard used strings.Contains(cleanPath, ".."), which matched
// double dots inside legal filename components. "notes..txt" was rejected as
// a traversal attack and fired a critical path-traversal audit event.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
