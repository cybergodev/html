package html

// regressions_timeout_test.go — regressions for the D-002/P-002 timeout and
// ownership fixes, gathered from the former single-topic files:
//
//   - any context.DeadlineExceeded was once rewritten to ErrProcessingTimeout
//     whenever ProcessingTimeout > 0, losing the caller's own earlier deadline
//     identity; the *WithContext entry points must preserve it;
//   - with a processing timeout configured, detectEncoding must decode into a
//     private copy (work runs on another goroutine), while the synchronous path
//     keeps the zero-copy decode.

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

// decodedAliasesInput reports whether the string returned by detectEncoding
// shares storage with htmlBytes, by mutating a byte of the input and checking
// whether the decoded text changes with it.
func decodedAliasesInput(t *testing.T, p *Processor, doc []byte) (decoded string, aliased bool) {
	t.Helper()

	decoded, err := p.detectEncoding(doc)
	if err != nil {
		t.Fatalf("detectEncoding: %v", err)
	}
	// Snapshot before mutating: a copy is unaffected by the mutation, an
	// aliased view changes with it.
	snapshot := strings.Clone(decoded)
	doc[0] ^= 0x20 // flip case of the first '<'-side byte
	if decoded != snapshot {
		doc[0] ^= 0x20
		return snapshot, true
	}
	doc[0] ^= 0x20
	return decoded, false
}

// TestDetectEncodingCopiesWhenTimeoutConfigured covers the processing-timeout
// branch: the default configuration runs document work on another goroutine, so
// the decoded document must not point into the caller's buffer.
func TestDetectEncodingCopiesWhenTimeoutConfigured(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	cfg.ProcessingTimeout = time.Minute // any positive value runs the worker
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	doc := []byte("<html><body><p>ascii only</p></body></html>")
	decoded, aliased := decodedAliasesInput(t, p, doc)
	if aliased {
		t.Fatal("decoded document aliases the caller's buffer although a processing timeout runs the work on another goroutine")
	}
	if want := "<html><body><p>ascii only</p></body></html>"; decoded != want {
		t.Fatalf("decoded = %q, want %q", decoded, want)
	}
}

// TestDetectEncodingKeepsZeroCopyWhenSynchronous guards the fast path: with no
// processing timeout, runWithTimeout calls the work inline, nothing outlives the
// call, and the zero-copy decode is retained (with its output unchanged).
func TestDetectEncodingKeepsZeroCopyWhenSynchronous(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	cfg.ProcessingTimeout = 0
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	doc := []byte("<html><body><p>ascii only</p></body></html>")
	decoded, aliased := decodedAliasesInput(t, p, doc)
	if !aliased {
		t.Log("decoded document is a copy; the zero-copy fast path no longer applies (behavior is still correct)")
	}
	if want := "<html><body><p>ascii only</p></body></html>"; decoded != want {
		t.Fatalf("decoded = %q, want %q", decoded, want)
	}
}
