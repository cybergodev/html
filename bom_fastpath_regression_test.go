package html

// bom_fastpath_regression_test.go — end-to-end regression for the D-002 P0
// fix: a UTF-8 BOM used to survive the encoding fast path into Result.Text
// (observed: a stray U+FEFF plus a spurious leading newline in front of
// "hello world"), while the same bytes forced down the slow path via
// Config.Encoding = "utf-8" came back clean.

import (
	"strings"
	"testing"
)

func TestExtractStripsUTF8BOM(t *testing.T) {
	t.Parallel()

	bom := []byte{0xEF, 0xBB, 0xBF}
	body := []byte("<html><body><p>hello world</p></body></html>")

	in := append(append([]byte{}, bom...), body...)

	// Default route: fast path (no forced encoding).
	got, err := Extract(in)
	if err != nil {
		t.Fatalf("Extract(BOM doc): %v", err)
	}
	if strings.ContainsRune(got.Text, rune(0xFEFF)) {
		t.Errorf("default route leaked a UTF-8 BOM into Text: %q", got.Text)
	}
	if want := "hello world"; got.Text != want {
		t.Errorf("Text = %q, want %q", got.Text, want)
	}

	// Slow route (forced encoding): must agree byte-for-byte with the fast
	// route on the same input.
	cfg := DefaultConfig()
	cfg.Encoding = "utf-8"
	slow, err := Extract(in, cfg)
	if err != nil {
		t.Fatalf("Extract(BOM doc, forced utf-8): %v", err)
	}
	if slow.Text != got.Text {
		t.Errorf("forced-encoding route Text = %q, default route = %q; want identical", slow.Text, got.Text)
	}
}
