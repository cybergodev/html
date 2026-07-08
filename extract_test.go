package html

import (
	"context"
	"errors"
	"testing"
)

// TestContainsASCIIFold pins the boundary behavior of the unexported
// containsASCIIFold helper. The function ASCII-case-folds the *haystack* and
// compares against the needle verbatim, so per its sole production caller
// (extract.go, needle "nofollow") the needle must be supplied lower-case. These
// cases exercise that contract at its edges: case folding, no-match, and the
// empty / over-long needle boundaries.
func TestContainsASCIIFold(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		s      string
		substr string
		want   bool
	}{
		{"exact lowercase match", "rel=nofollow", "nofollow", true},
		{"mixed-case haystack", "rel=NoFollow", "nofollow", true},
		{"uppercase haystack", "REL=NOFOLLOW", "nofollow", true},
		{"substring embedded", "a nofollow b", "nofollow", true},
		{"no match different word", "rel=dofollow", "nofollow", false},
		{"needle longer than haystack", "nf", "nofollow", false},
		{"empty needle matches anywhere", "anything", "", true},
		{"empty haystack empty needle", "", "", true},
		{"empty haystack non-empty needle", "", "x", false},
		{"ascii prefix before multibyte tail", "caf\xc3\xa9", "caf", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsASCIIFold(tt.s, tt.substr); got != tt.want {
				t.Errorf("containsASCIIFold(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}

// TestPackageLevelWrappers_ConfigErrors drives the two shared error branches
// that all package-level Extract*/ExtractText* convenience wrappers funnel
// through:
//
//  1. resolveConfig rejecting >=2 variadic configs (ErrMultipleConfigs) — the
//     "multiple configs" subtests. Covers resolveConfig's default branch and
//     each wrapper's `if err != nil` early return.
//  2. withProcessor failing when New(cfg) rejects an invalid single config —
//     the "invalid config" subtests (processor_pool.go: the New(cfg) error
//     path). A negative MaxInputSize is rejected by Config.Validate, so the
//     processor is never built.
//
// Both paths return before any I/O or context work, so the file path passed to
// the *FromFile variants need not exist.
func TestPackageLevelWrappers_ConfigErrors(t *testing.T) {
	t.Parallel()

	valid := DefaultConfig()
	invalid := DefaultConfig()
	invalid.MaxInputSize = -1 // Config.Validate rejects this
	ctx := context.Background()
	htmlBytes := []byte("<p>x</p>")

	wrappers := []struct {
		name       string
		multi      func() error // passes two configs -> ErrMultipleConfigs
		oneInvalid func() error // passes one invalid config -> New() failure
	}{
		{"Extract", func() error {
			_, err := Extract(htmlBytes, valid, valid)
			return err
		}, func() error {
			_, err := Extract(htmlBytes, invalid)
			return err
		}},
		{"ExtractFromFile", func() error {
			_, err := ExtractFromFile("does-not-exist.html", valid, valid)
			return err
		}, func() error {
			_, err := ExtractFromFile("does-not-exist.html", invalid)
			return err
		}},
		{"ExtractText", func() error {
			_, err := ExtractText(htmlBytes, valid, valid)
			return err
		}, func() error {
			_, err := ExtractText(htmlBytes, invalid)
			return err
		}},
		{"ExtractTextFromFile", func() error {
			_, err := ExtractTextFromFile("does-not-exist.html", valid, valid)
			return err
		}, func() error {
			_, err := ExtractTextFromFile("does-not-exist.html", invalid)
			return err
		}},
		{"ExtractWithContext", func() error {
			_, err := ExtractWithContext(ctx, htmlBytes, valid, valid)
			return err
		}, func() error {
			_, err := ExtractWithContext(ctx, htmlBytes, invalid)
			return err
		}},
		{"ExtractFromFileWithContext", func() error {
			_, err := ExtractFromFileWithContext(ctx, "does-not-exist.html", valid, valid)
			return err
		}, func() error {
			_, err := ExtractFromFileWithContext(ctx, "does-not-exist.html", invalid)
			return err
		}},
		{"ExtractTextWithContext", func() error {
			_, err := ExtractTextWithContext(ctx, htmlBytes, valid, valid)
			return err
		}, func() error {
			_, err := ExtractTextWithContext(ctx, htmlBytes, invalid)
			return err
		}},
		{"ExtractTextFromFileWithContext", func() error {
			_, err := ExtractTextFromFileWithContext(ctx, "does-not-exist.html", valid, valid)
			return err
		}, func() error {
			_, err := ExtractTextFromFileWithContext(ctx, "does-not-exist.html", invalid)
			return err
		}},
	}

	for _, w := range wrappers {
		t.Run(w.name+"/multiple configs", func(t *testing.T) {
			t.Parallel()
			if err := w.multi(); err == nil {
				t.Errorf("%s with two configs: expected ErrMultipleConfigs, got nil", w.name)
			} else if !errors.Is(err, ErrMultipleConfigs) {
				t.Errorf("%s with two configs: expected ErrMultipleConfigs, got %v", w.name, err)
			}
		})
		t.Run(w.name+"/invalid config", func(t *testing.T) {
			t.Parallel()
			if err := w.oneInvalid(); err == nil {
				t.Errorf("%s with invalid config: expected validation error, got nil", w.name)
			}
		})
	}
}
