package html

// file_io_internal_test.go — in-package tests for the read-side MaxInputSize
// cap (readBounded), which bounds file reads that the Stat-based pre-checks
// cannot see: non-regular files (FIFOs, devices) report an implausible Stat
// size, and a regular file can grow between its Stat and the read.

import (
	"errors"
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

		data, err := p.readBounded(strings.NewReader(""), "x.html")
		if err != nil {
			t.Fatalf("readBounded(empty) error = %v", err)
		}
		if len(data) != 0 {
			t.Errorf("readBounded(empty) = %d bytes, want 0", len(data))
		}
	})

	t.Run("exactly at limit", func(t *testing.T) {
		t.Parallel()

		data, err := p.readBounded(strings.NewReader(strings.Repeat("a", 1024)), "x.html")
		if err != nil {
			t.Fatalf("readBounded(at limit) error = %v", err)
		}
		if len(data) != 1024 {
			t.Errorf("readBounded(at limit) = %d bytes, want 1024", len(data))
		}
	})

	t.Run("one byte over limit", func(t *testing.T) {
		t.Parallel()

		_, err := p.readBounded(strings.NewReader(strings.Repeat("a", 1025)), "x.html")
		if !errors.Is(err, ErrInputTooLarge) {
			t.Errorf("readBounded(over limit) error = %v, want ErrInputTooLarge", err)
		}
	})

	t.Run("far over limit reads at most limit+1", func(t *testing.T) {
		t.Parallel()

		_, err := p.readBounded(strings.NewReader(strings.Repeat("a", 1<<20)), "x.html")
		if !errors.Is(err, ErrInputTooLarge) {
			t.Errorf("readBounded(1MB over 1KB limit) error = %v, want ErrInputTooLarge", err)
		}
		var inputErr *InputError
		if !errors.As(err, &inputErr) || inputErr.Size != 1025 {
			t.Errorf("error should report the capped size 1025, got %v", err)
		}
	})
}
