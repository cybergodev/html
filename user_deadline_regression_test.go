package html

// user_deadline_regression_test.go — regression for the D-002 P1 fix in
// runWithTimeout: any context.DeadlineExceeded was rewritten to
// ErrProcessingTimeout whenever ProcessingTimeout > 0 (the default), so the
// caller's own earlier deadline lost its identity, contradicting the
// documented contract of the *WithContext entry points and recording the
// user's cancellation in the audit log as a processing timeout.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// userDeadlineDoc is large enough that full processing reliably outlives a
// ~1ms caller deadline (each <p> adds an element plus a text node).
func userDeadlineDoc() []byte {
	var b strings.Builder
	b.WriteString("<html><body>")
	for i := 0; i < 20000; i++ {
		b.WriteString("<p>filler text</p>")
	}
	b.WriteString("</body></html>")
	return []byte(b.String())
}

func TestUserDeadlineIdentityPreserved(t *testing.T) {
	t.Parallel()

	doc := userDeadlineDoc()

	// Cache off: the sanity run below would otherwise cache the result and
	// the deadline run would return instantly from the cache with nil error.
	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0

	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	// Sanity: without a user deadline the document processes fine.
	if _, err := p.Extract(doc); err != nil {
		t.Fatalf("Extract(no deadline): %v", err)
	}

	// A caller deadline well inside the default 30s ProcessingTimeout must
	// surface as context.DeadlineExceeded, not ErrProcessingTimeout.
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_, err = p.ExtractWithContext(ctx, doc)
	if err == nil {
		t.Fatal("ExtractWithContext(user deadline): expected an error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded identity", err)
	}
	if errors.Is(err, ErrProcessingTimeout) {
		t.Errorf("user deadline was rewritten to ErrProcessingTimeout: %v", err)
	}
}
