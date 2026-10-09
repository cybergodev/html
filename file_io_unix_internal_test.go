//go:build unix

package html

// file_io_unix_internal_test.go — integration test for the read-side
// MaxInputSize cap against a real non-regular file. /dev/zero is a character
// device whose Stat size is 0, so the Stat-based pre-check in
// validateAndReadFile never rejects it; before readBounded, extracting it
// streamed unbounded bytes into memory.

import (
	"errors"
	"os"
	"testing"
)

func TestExtractFromFileNonRegularBounded(t *testing.T) {
	t.Parallel()

	if _, err := os.Stat("/dev/zero"); err != nil {
		t.Skip("/dev/zero not available on this system")
	}

	cfg := DefaultConfig()
	cfg.MaxInputSize = 64 * 1024
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = p.Close() }()

	// Without the read-side cap this call would attempt to read /dev/zero
	// (an endless stream of zeros) into memory.
	_, err = p.ExtractFromFile("/dev/zero")
	if !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("ExtractFromFile(/dev/zero) error = %v, want ErrInputTooLarge", err)
	}
}
