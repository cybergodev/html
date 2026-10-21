package table

// p2_regression_test.go — regression tests for the D-002 P2 fixes in the
// table package: TrackedBuilder.SecureClear (pool secure-clear support) and
// the getRenderer format dispatch that replaced the unused registry.

import (
	"testing"
)

func TestTrackedBuilderSecureClear(t *testing.T) {
	t.Parallel()
	tb := NewTrackedBuilder()
	_, _ = tb.WriteString("sensitive document text")
	tb.SecureClear()
	if tb.Len() != 0 || tb.LastChar != 0 {
		t.Errorf("SecureClear left len=%d lastChar=%d", tb.Len(), tb.LastChar)
	}
	// The builder must remain usable after SecureClear.
	if _, _ = tb.WriteString("x"); tb.String() != "x" {
		t.Errorf("builder unusable after SecureClear: %q", tb.String())
	}
}

func TestGetRendererDispatch(t *testing.T) {
	t.Parallel()
	if _, ok := getRenderer("html").(*HTMLRenderer); !ok {
		t.Error(`getRenderer("html") did not return HTMLRenderer`)
	}
	if _, ok := getRenderer("markdown").(*MarkdownRenderer); !ok {
		t.Error(`getRenderer("markdown") did not return MarkdownRenderer`)
	}
	if _, ok := getRenderer("no-such-format").(*MarkdownRenderer); !ok {
		t.Error("unknown format did not fall back to MarkdownRenderer")
	}
}
