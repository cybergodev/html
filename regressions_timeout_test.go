package html

// regressions_timeout_test.go — regressions for the D-002/P-002 timeout and
// ownership fixes, gathered from the former single-topic files (plus the
// timeout-audit parity test from d002_quality_regression_test.go):
//
//   - any context.DeadlineExceeded was once rewritten to ErrProcessingTimeout
//     whenever ProcessingTimeout > 0, losing the caller's own earlier deadline
//     identity; the *WithContext entry points must preserve it;
//   - with a processing timeout configured, detectEncoding must decode into a
//     private copy (work runs on another goroutine), while the synchronous path
//     keeps the zero-copy decode;
//   - a processing timeout on the links path records an AuditEventTimeout
//     entry, matching the Extract path (extractAllLinksCore previously bumped
//     only the error counter, leaving an audit blind spot);
//   - a UTF-8 BOM must be stripped identically on the encoding fast path and
//     the forced-encoding slow path (moved here from the former
//     bom_fastpath_regression_test.go — another fast-vs-slow-path parity
//     regression alongside the detectEncoding pair);
//   - the remaining *FromFileWithContext entry points observe a cancelled
//     context before their uncancellable file read, matching the
//     ExtractFromFileWithContext precedence contract.

import (
	"context"
	"errors"
	"path/filepath"
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

// TestExtractAllLinksTimeoutAudited pins the audit parity fix: a document
// whose link extraction exceeds ProcessingTimeout must surface
// ErrProcessingTimeout AND record a timeout audit entry, exactly as the
// Extract path does. The deadline is 1µs so the timeout fires before the
// parse of a many-element document can complete, independent of machine speed.
func TestExtractAllLinksTimeoutAudited(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	cfg.ProcessingTimeout = time.Microsecond
	cfg.Audit.Enabled = true
	cfg.Audit.LogTimeouts = true

	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	_, err = p.ExtractAllLinks(userDeadlineDoc())
	if !errors.Is(err, ErrProcessingTimeout) {
		t.Fatalf("ExtractAllLinks error = %v, want ErrProcessingTimeout", err)
	}

	for _, e := range p.GetAuditLog() {
		if e.EventType == AuditEventTimeout {
			return
		}
	}
	t.Errorf("no %s audit entry recorded for a links-path processing timeout; entries: %+v",
		AuditEventTimeout, p.GetAuditLog())
}

// TestExtractStripsUTF8BOM is the end-to-end regression for the D-002 P0 fix:
// a UTF-8 BOM used to survive the encoding fast path into Result.Text
// (observed: a stray U+FEFF plus a spurious leading newline in front of
// "hello world"), while the same bytes forced down the slow path via
// Config.Encoding = "utf-8" came back clean.
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

// TestFromFileContextCancellationPrecedence pins the early-cancellation
// ordering for the remaining *FromFileWithContext entry points whose
// cancelled-context branch had no test: ExtractAllLinksFromFileWithContext
// and ExtractToMarkdownFromFileWithContext (the ReadPage FromFile family is
// covered by TestReadPageFromFileErrors / TestReadPageContextVariants in
// read_test.go). A cancelled context must win over the file error, because
// the file read itself is not cancellable.
func TestFromFileContextCancellationPrecedence(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "missing.html")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	if _, err := p.ExtractAllLinksFromFileWithContext(ctx, missing); !errors.Is(err, context.Canceled) {
		t.Errorf("ExtractAllLinksFromFileWithContext: cancelled-context error = %v, want context.Canceled", err)
	}
	if _, err := p.ExtractToMarkdownFromFileWithContext(ctx, missing); !errors.Is(err, context.Canceled) {
		t.Errorf("ExtractToMarkdownFromFileWithContext: cancelled-context error = %v, want context.Canceled", err)
	}
}
