package html_test

// panic_test.go — black-box panic-safety suite (SEC-003), consolidating
// panic_protection_test.go (the recoverable ErrInternalPanic contract across
// processor methods, package functions, batch operations, and concurrent
// extraction) and panic_audit_test.go (the panic-audit gap-fill: the
// ExtractAllLinks family’s nil/closed/hostile-input coverage and the ReadPage
// custom-Scorer panic recovery). The white-box counterpart lives in
// panic_api_regression_test.go (package html).

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cybergodev/html"
)

// /////////////////////////////////////////////////////////////////////////////
// From panic_protection_test.go
// /////////////////////////////////////////////////////////////////////////////

// recover from internal panics and return ErrInternalPanic instead of crashing.
// This implements the SEC-003 security hardening requirement.

// panicScorer is a Scorer implementation that panics when Score is called.
type panicScorer struct{}

func (panicScorer) Score(html.ContentNode) int {
	panic("SEC-003 test: intentional scorer panic")
}

func (panicScorer) ShouldRemove(html.ContentNode) bool {
	panic("SEC-003 test: intentional ShouldRemove panic")
}

func newPanicScorerConfig() html.Config {
	cfg := html.DefaultConfig()
	cfg.ExtractArticle = true
	cfg.Scorer = panicScorer{}
	return cfg
}

// panickingSink is an AuditSink whose Write always panics. It exercises the
// synchronous sink-write recover path (SEC-003): a misbehaving custom sink
// must never crash the process. auditCollector.Record invokes sink.Write
// inline under a recover, so the panic is caught before Record returns.
type panickingSink struct{}

func (panickingSink) Write(html.AuditEntry) {
	panic("SEC-003 test: intentional audit sink panic")
}

func (panickingSink) Close() error { return nil }

// closePanickingSink is an AuditSink whose Close panics. It exercises the
// sink-close recover path (SEC-003): a misbehaving custom sink's Close must
// never crash the process via the canonical `defer processor.Close()` pattern.
// Write is benign so the sink can be installed on an enabled audit config
// without every Record aborting.
type closePanickingSink struct{}

func (closePanickingSink) Write(html.AuditEntry) {}

func (closePanickingSink) Close() error {
	panic("SEC-003 test: intentional audit sink close panic")
}

const testHTML = `<html><head><title>Test</title></head><body><p>Hello World</p><article><p>Content</p></article></body></html>`

// createTestHTMLFile creates a temp HTML file for testing.
func createTestHTMLFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := dir + "/" + name
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestPanicRecovery_ProcessorMethods verifies that all Processor methods
// recover from internal panics and return ErrInternalPanic.
func TestPanicRecovery_ProcessorMethods(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpFile := createTestHTMLFile(t, tmpDir, "test.html", testHTML)

	tests := []struct {
		name string
		fn   func(p *html.Processor) error
	}{
		{"Extract", func(p *html.Processor) error { _, err := p.Extract([]byte(testHTML)); return err }},
		{"ExtractWithContext", func(p *html.Processor) error {
			_, err := p.ExtractWithContext(context.Background(), []byte(testHTML))
			return err
		}},
		{"ExtractFromFile", func(p *html.Processor) error { _, err := p.ExtractFromFile(tmpFile); return err }},
		{"ExtractFromFileWithContext", func(p *html.Processor) error {
			_, err := p.ExtractFromFileWithContext(context.Background(), tmpFile)
			return err
		}},
		{"ExtractText", func(p *html.Processor) error { _, err := p.ExtractText([]byte(testHTML)); return err }},
		{"ExtractTextWithContext", func(p *html.Processor) error {
			_, err := p.ExtractTextWithContext(context.Background(), []byte(testHTML))
			return err
		}},
		{"ExtractToMarkdown", func(p *html.Processor) error { _, err := p.ExtractToMarkdown([]byte(testHTML)); return err }},
		{"ExtractToMarkdownWithContext", func(p *html.Processor) error {
			_, err := p.ExtractToMarkdownWithContext(context.Background(), []byte(testHTML))
			return err
		}},
		{"ExtractToJSON", func(p *html.Processor) error { _, err := p.ExtractToJSON([]byte(testHTML)); return err }},
		{"ExtractToJSONFromFile", func(p *html.Processor) error {
			_, err := p.ExtractToJSONFromFile(tmpFile)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, err := html.New(newPanicScorerConfig())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = p.Close() }()

			err = tt.fn(p)
			if !errors.Is(err, html.ErrInternalPanic) {
				t.Fatalf("expected ErrInternalPanic, got: %v", err)
			}
		})
	}
}

// TestPanicRecovery_PackageFunctions verifies package-level functions
// recover from internal panics.
func TestPanicRecovery_PackageFunctions(t *testing.T) {
	t.Parallel()

	cfg := newPanicScorerConfig()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"Extract", func() error { _, err := html.Extract([]byte(testHTML), cfg); return err }},
		{"ExtractWithContext", func() error {
			_, err := html.ExtractWithContext(context.Background(), []byte(testHTML), cfg)
			return err
		}},
		{"ExtractText", func() error { _, err := html.ExtractText([]byte(testHTML), cfg); return err }},
		{"ExtractToMarkdown", func() error { _, err := html.ExtractToMarkdown([]byte(testHTML), cfg); return err }},
		{"ExtractToJSON", func() error { _, err := html.ExtractToJSON([]byte(testHTML), cfg); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.fn()
			if !errors.Is(err, html.ErrInternalPanic) {
				t.Fatalf("expected ErrInternalPanic, got: %v", err)
			}
		})
	}
}

// TestPanicRecovery_BatchOperations verifies batch methods recover from panics.
func TestPanicRecovery_BatchOperations(t *testing.T) {
	t.Parallel()

	t.Run("ExtractBatch recovers per-item", func(t *testing.T) {
		p, err := html.New(newPanicScorerConfig())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = p.Close() }()

		br := p.ExtractBatch([][]byte{[]byte(testHTML), []byte(testHTML), []byte(testHTML)})
		if br.Success != 0 {
			t.Fatalf("expected 0 successes, got %d", br.Success)
		}
		if br.Failed != 3 {
			t.Fatalf("expected 3 failures, got %d", br.Failed)
		}
		for i, e := range br.Errors {
			if !errors.Is(e, html.ErrInternalPanic) {
				t.Errorf("item %d: expected ErrInternalPanic, got: %v", i, e)
			}
		}
	})

	t.Run("ExtractBatchWithContext recovers", func(t *testing.T) {
		p, err := html.New(newPanicScorerConfig())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = p.Close() }()

		br := p.ExtractBatchWithContext(context.Background(), [][]byte{[]byte(testHTML), []byte(testHTML)})
		if br.Failed != 2 {
			t.Fatalf("expected 2 failures, got %d", br.Failed)
		}
	})

	t.Run("ExtractBatchFiles recovers", func(t *testing.T) {
		p, err := html.New(newPanicScorerConfig())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = p.Close() }()

		tmpDir := t.TempDir()
		paths := []string{
			createTestHTMLFile(t, tmpDir, "a.html", testHTML),
			createTestHTMLFile(t, tmpDir, "b.html", testHTML),
		}

		br := p.ExtractBatchFiles(paths)
		if br.Failed != 2 {
			t.Fatalf("expected 2 failures, got %d", br.Failed)
		}
		for i, e := range br.Errors {
			if !errors.Is(e, html.ErrInternalPanic) {
				t.Errorf("file %d: expected ErrInternalPanic, got: %v", i, e)
			}
		}
	})

	t.Run("package ExtractBatch recovers", func(t *testing.T) {
		cfg := newPanicScorerConfig()
		br := html.ExtractBatch([][]byte{[]byte(testHTML)}, cfg)
		if br.Failed != 1 {
			t.Fatalf("expected 1 failure, got %d", br.Failed)
		}
		if !errors.Is(br.Errors[0], html.ErrInternalPanic) {
			t.Fatalf("expected ErrInternalPanic, got: %v", br.Errors[0])
		}
	})

}

// TestPanicRecovery_NilAndClosedProcessor verifies nil/closed processor safety
// across all methods via table-driven subtests.
func TestPanicRecovery_NilAndClosedProcessor(t *testing.T) {
	t.Parallel()

	methods := []struct {
		name string
		fn   func(p *html.Processor) error
	}{
		{"Extract", func(p *html.Processor) error { _, err := p.Extract([]byte(testHTML)); return err }},
		{"ExtractWithContext", func(p *html.Processor) error {
			_, err := p.ExtractWithContext(context.Background(), []byte(testHTML))
			return err
		}},
		{"ExtractText", func(p *html.Processor) error { _, err := p.ExtractText([]byte(testHTML)); return err }},
		{"ExtractTextWithContext", func(p *html.Processor) error {
			_, err := p.ExtractTextWithContext(context.Background(), []byte(testHTML))
			return err
		}},
		{"ExtractFromFile", func(p *html.Processor) error { _, err := p.ExtractFromFile("test.html"); return err }},
		{"ExtractToMarkdown", func(p *html.Processor) error { _, err := p.ExtractToMarkdown([]byte(testHTML)); return err }},
		{"ExtractToJSON", func(p *html.Processor) error { _, err := p.ExtractToJSON([]byte(testHTML)); return err }},
	}

	t.Run("nil processor", func(t *testing.T) {
		var p *html.Processor
		for _, m := range methods {
			err := m.fn(p)
			if !errors.Is(err, html.ErrProcessorClosed) {
				t.Errorf("%s: expected ErrProcessorClosed, got: %v", m.name, err)
			}
		}
		if p.Close() != nil {
			t.Error("nil processor Close should return nil")
		}
	})

	t.Run("closed processor", func(t *testing.T) {
		p, err := html.New()
		if err != nil {
			t.Fatal(err)
		}
		_ = p.Close()

		for _, m := range methods {
			err := m.fn(p)
			if !errors.Is(err, html.ErrProcessorClosed) {
				t.Errorf("%s: expected ErrProcessorClosed, got: %v", m.name, err)
			}
		}

		br := p.ExtractBatch([][]byte{[]byte(testHTML)})
		if br.Failed != 1 {
			t.Errorf("batch: expected 1 failure, got %d", br.Failed)
		}
	})
}

// TestPanicRecovery_ConcurrentExtract verifies concurrent panic recovery.
func TestPanicRecovery_ConcurrentExtract(t *testing.T) {
	t.Parallel()

	p, err := html.New(newPanicScorerConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	var panicCount atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := p.Extract([]byte(testHTML))
			if err != nil && errors.Is(err, html.ErrInternalPanic) {
				panicCount.Add(1)
			}
		}()
	}

	wg.Wait()

	count := panicCount.Load()
	if count != 10 {
		t.Fatalf("expected 10 recovered panics, got %d", count)
	}
}

// TestPanicRecovery_ErrInternalPanicMessage verifies the error message
// preserves original panic details.
func TestPanicRecovery_ErrInternalPanicMessage(t *testing.T) {
	t.Parallel()

	p, err := html.New(newPanicScorerConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	_, err = p.Extract([]byte(testHTML))
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, html.ErrInternalPanic) {
		t.Fatalf("expected ErrInternalPanic, got: %v", err)
	}
	if err.Error() == html.ErrInternalPanic.Error() {
		t.Error("error message should contain original panic details")
	}
}

// TestPanicRecovery_PanickingAuditSink verifies that a custom AuditSink whose
// Write panics does not crash the process. The async sink-write goroutine must
// recover so a misbehaving sink never propagates a panic through the public API.
func TestPanicRecovery_PanickingAuditSink(t *testing.T) {
	t.Parallel()

	cfg := html.DefaultConfig()
	cfg.Audit.Enabled = true
	cfg.Audit.Sink = panickingSink{}
	// A tiny MaxInputSize guarantees every call records an input-violation
	// audit entry — which Record writes to the panicking sink synchronously,
	// under its own recover — regardless of sanitization behavior. This
	// deterministically exercises the sink-write recover path.
	cfg.MaxInputSize = 8

	p, err := html.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	oversized := []byte("<html><body>exceeds the tiny max input size</body></html>")
	for i := 0; i < 5; i++ {
		// The size error is expected; the goal is to force the async sink
		// goroutine to run. An unrecovered panic there would crash the process.
		if _, err := p.Extract(oversized); !errors.Is(err, html.ErrInputTooLarge) {
			t.Fatalf("call %d: expected ErrInputTooLarge, got: %v", i, err)
		}
	}

	// Sink writes are synchronous (see auditCollector.Record), so by the time
	// each Extract above returned, the panicking Write had already been
	// recovered inline. Reaching Close confirms the process never aborted and
	// that a misbehaving sink cannot propagate a panic to the public API.
	if err := p.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
}

// TestPanicRecovery_PanickingAuditSinkClose verifies that a custom AuditSink
// whose Close panics is recovered inside auditCollector.Close (which
// Processor.Close delegates to) rather than crashing the process. Before the
// recover was added, this panic propagated through the `defer p.Close()`
// pattern and aborted the program — defeating SEC-003, which Record already
// honored for Write.
func TestPanicRecovery_PanickingAuditSinkClose(t *testing.T) {
	t.Parallel()

	cfg := html.DefaultConfig()
	cfg.Audit.Enabled = true
	cfg.Audit.Sink = closePanickingSink{}

	p, err := html.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// If the sink's Close panic escapes Processor.Close, this recover catches
	// it and fails the test. With the recover in auditCollector.Close, the
	// panic is caught there and never reaches this defer.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("audit sink Close panic escaped Processor.Close: %v", r)
		}
	}()

	// Processor.Close treats audit cleanup as best-effort and discards the
	// recovered error; the assertion is simply that Close returns without
	// aborting the process.
	_ = p.Close()
}

// TestPanicRecovery_ZeroValueProcessor verifies that a hand-built zero-value
// Processor (legal for external code: exported type, unexported fields, empty
// composite literal) cannot panic the public API. The Extract family already
// folds its nil-config dereference into ErrInternalPanic inside recoverPanic,
// but the batch family previously dereferenced p.config on the caller's
// goroutine with no recover at all, and ClearCache/Close dereferenced a nil
// p.cache — both escaped as process-crashing panics. The batch cases run
// before the lifecycle calls because Close marks the processor closed, which
// would short-circuit them to ErrProcessorClosed instead.
func TestPanicRecovery_ZeroValueProcessor(t *testing.T) {
	t.Parallel()

	p := &html.Processor{} // zero value: nil config, nil cache, nil audit

	batchCases := []struct {
		name string
		run  func() *html.BatchResult
	}{
		{"ExtractBatch", func() *html.BatchResult { return p.ExtractBatch([][]byte{[]byte(testHTML)}) }},
		{"ExtractBatchWithContext", func() *html.BatchResult {
			return p.ExtractBatchWithContext(context.Background(), [][]byte{[]byte(testHTML)})
		}},
		{"ExtractBatchFiles", func() *html.BatchResult { return p.ExtractBatchFiles([]string{"a.html"}) }},
		{"ExtractBatchFilesWithContext", func() *html.BatchResult {
			return p.ExtractBatchFilesWithContext(context.Background(), []string{"a.html"})
		}},
	}
	for _, tc := range batchCases {
		br := tc.run()
		if br == nil {
			t.Fatalf("%s: returned nil BatchResult", tc.name)
		}
		if br.Failed != 1 || br.Success != 0 {
			t.Errorf("%s: expected Failed=1 Success=0, got Failed=%d Success=%d", tc.name, br.Failed, br.Success)
		}
		if len(br.Errors) != 1 || !errors.Is(br.Errors[0], html.ErrInternalPanic) {
			t.Errorf("%s: expected ErrInternalPanic, got: %v", tc.name, br.Errors)
		}
	}

	// Lifecycle methods must be safe no-ops on a zero-value Processor.
	p.ClearCache()
	if err := p.Close(); err != nil {
		t.Errorf("zero-value Close should return nil, got: %v", err)
	}
}

// TestPanicRecovery_BatchNilContext verifies that a nil context — caller error
// by Go convention — surfaces as a uniform ErrInternalPanic BatchResult
// instead of a caller-goroutine panic in the Done() selects, matching what
// ExtractWithContext already does for the same input via recoverPanic.
func TestPanicRecovery_BatchNilContext(t *testing.T) {
	t.Parallel()

	p, err := html.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	// The nil ctx is the input under test (SA1012 is the point: callers do
	// pass nil by mistake, and the API must not crash their process).
	br := p.ExtractBatchWithContext(nil, [][]byte{[]byte(testHTML)}) //nolint:staticcheck // deliberately nil
	if br.Failed != 1 || br.Success != 0 {
		t.Fatalf("expected Failed=1 Success=0, got Failed=%d Success=%d", br.Failed, br.Success)
	}
	if len(br.Errors) != 1 || !errors.Is(br.Errors[0], html.ErrInternalPanic) {
		t.Fatalf("expected ErrInternalPanic for nil context, got: %v", br.Errors)
	}

	brf := p.ExtractBatchFilesWithContext(nil, []string{"a.html"}) //nolint:staticcheck // deliberately nil
	if brf.Failed != 1 || brf.Success != 0 || !errors.Is(brf.Errors[0], html.ErrInternalPanic) {
		t.Fatalf("expected ErrInternalPanic for nil context (files), got: %v", brf.Errors)
	}
}

// /////////////////////////////////////////////////////////////////////////////
// From panic_audit_test.go
// /////////////////////////////////////////////////////////////////////////////

// audit: the ExtractAllLinks family had no panic-path coverage at all (nil and
// closed processors, hostile inputs, scorer independence), and the ReadPage
// family never asserted the recoverable ErrInternalPanic contract when a
// custom Scorer panics on the article-node fallback path.

// mustNotPanic runs fn, converting a panic into a test failure that names
// the call. It is the html_test counterpart of mustNotPanic in the internal
// panic_api_regression_test.go.
func mustNotPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%s panicked: %v", what, r)
		}
	}()
	fn()
}

// linksDoc carries several resource kinds (anchor, stylesheet, image, script)
// so ExtractAllLinks has something to find on every path below.
const linksDoc = `<html><head>
<link rel="stylesheet" href="/style.css">
<script src="/app.js"></script>
</head><body>
<p>See <a href="https://example.com/docs">the docs</a> and
<img src="/img/pic.png" alt="pic">.</p>
</body></html>`

// TestSEC003ExtractAllLinksNilAndClosedProcessor verifies the nil/closed
// receiver contract on the four ExtractAllLinks methods — the family was
// absent from TestPanicRecovery_NilAndClosedProcessor's method table.
func TestSEC003ExtractAllLinksNilAndClosedProcessor(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	calls := []struct {
		name string
		call func(p *html.Processor) error
	}{
		{"ExtractAllLinks", func(p *html.Processor) error {
			_, err := p.ExtractAllLinks([]byte(linksDoc))
			return err
		}},
		{"ExtractAllLinksWithContext", func(p *html.Processor) error {
			_, err := p.ExtractAllLinksWithContext(ctx, []byte(linksDoc))
			return err
		}},
		{"ExtractAllLinksFromFile", func(p *html.Processor) error {
			_, err := p.ExtractAllLinksFromFile("no-such.html")
			return err
		}},
		{"ExtractAllLinksFromFileWithContext", func(p *html.Processor) error {
			_, err := p.ExtractAllLinksFromFileWithContext(ctx, "no-such.html")
			return err
		}},
	}

	t.Run("nil processor", func(t *testing.T) {
		t.Parallel()
		var p *html.Processor
		for _, c := range calls {
			mustNotPanic(t, "(*Processor)(nil)."+c.name, func() {
				if err := c.call(p); !errors.Is(err, html.ErrProcessorClosed) {
					t.Errorf("%s on nil processor = %v, want ErrProcessorClosed", c.name, err)
				}
			})
		}
	})

	t.Run("closed processor", func(t *testing.T) {
		t.Parallel()
		p, err := html.New()
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Close(); err != nil {
			t.Fatal(err)
		}
		for _, c := range calls {
			mustNotPanic(t, "closed processor "+c.name, func() {
				if err := c.call(p); !errors.Is(err, html.ErrProcessorClosed) {
					t.Errorf("%s on closed processor = %v, want ErrProcessorClosed", c.name, err)
				}
			})
		}
	})
}

// TestSEC003ExtractAllLinksHostileInputsNoPanic drives the package-level
// link extractors with the same hostile input classes the ReadPage surface
// regression covers: nil, empty, blank, binary junk, and oversize input
// (rejected by the MaxInputSize guard), plus an already-cancelled context.
func TestSEC003ExtractAllLinksHostileInputsNoPanic(t *testing.T) {
	t.Parallel()

	inputs := map[string][]byte{
		"nil":    nil,
		"empty":  {},
		"blank":  []byte(" \t\n\r "),
		"binary": {0xFF, 0xFE, 0x00, 0x01, 0x80},
	}
	for name, in := range inputs {
		mustNotPanic(t, "ExtractAllLinks("+name+")", func() { _, _ = html.ExtractAllLinks(in) })
		mustNotPanic(t, "ExtractAllLinksFromFileWithContext("+name+" ctx)", func() {
			_, _ = html.ExtractAllLinksWithContext(context.Background(), in)
		})
	}

	oversize := make([]byte, html.DefaultConfig().MaxInputSize+1)
	mustNotPanic(t, "ExtractAllLinks(oversize)", func() {
		if _, err := html.ExtractAllLinks(oversize); !errors.Is(err, html.ErrInputTooLarge) {
			t.Errorf("ExtractAllLinks(oversize) = %v, want ErrInputTooLarge", err)
		}
	})

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	mustNotPanic(t, "ExtractAllLinksWithContext(cancelled)", func() {
		_, _ = html.ExtractAllLinksWithContext(cancelled, []byte(linksDoc))
	})
}

// TestSEC003ExtractAllLinksIndependentOfScorer pins that the link-enumeration
// path never invokes the configured Scorer: a processor whose scorer panics on
// every Score call still enumerates links successfully. If a future change
// routes ExtractAllLinks through article selection, this test fails and the
// recover contract (rather than success) must be re-established deliberately.
func TestSEC003ExtractAllLinksIndependentOfScorer(t *testing.T) {
	t.Parallel()

	cfg := newPanicScorerConfig()
	p, err := html.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	links, err := p.ExtractAllLinks([]byte(linksDoc))
	if err != nil {
		t.Fatalf("ExtractAllLinks with panicking scorer: %v, want success (links path must not use the scorer)", err)
	}
	if len(links) == 0 {
		t.Fatal("ExtractAllLinks returned no links for linksDoc")
	}
}

// TestSEC003ReadPageScorerPanicRecovered exercises the ReadPage family's
// recoverable-panic contract with a panicking custom Scorer. The article-node
// fallback runs the scorer only when no <main>-carrying region is recognized,
// so the div-soup document must come back as ErrInternalPanic (recovered,
// never a crash) while the landmark document succeeds without ever consulting
// the scorer.
func TestSEC003ReadPageScorerPanicRecovered(t *testing.T) {
	t.Parallel()

	const divSoup = `<html><body>` +
		`<div><p>First block with <a href="/x">a link</a>.</p></div>` +
		`<div><p>Second block of body text.</p></div>` +
		`</body></html>`
	const landmark = `<html><body><main><article><h1>Title</h1>` +
		`<p>Body with <a href="/y">a link</a>.</p></article></main></body></html>`

	ctx := context.Background()
	panicCalls := []struct {
		name string
		call func(p *html.Processor) error
	}{
		{"ReadPage", func(p *html.Processor) error {
			_, err := p.ReadPage([]byte(divSoup))
			return err
		}},
		{"ReadPageWithContext", func(p *html.Processor) error {
			_, err := p.ReadPageWithContext(ctx, []byte(divSoup))
			return err
		}},
		{"ReadPageToMarkdown", func(p *html.Processor) error {
			_, err := p.ReadPageToMarkdown([]byte(divSoup))
			return err
		}},
		{"ReadPageToMarkdownText", func(p *html.Processor) error {
			_, err := p.ReadPageToMarkdownText([]byte(divSoup))
			return err
		}},
		{"package ReadPage", func(p *html.Processor) error {
			_, err := html.ReadPage([]byte(divSoup), newPanicScorerConfig())
			return err
		}},
	}

	for _, c := range panicCalls {
		t.Run(c.name+" div-soup panicking scorer", func(t *testing.T) {
			t.Parallel()
			p, err := html.New(newPanicScorerConfig())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = p.Close() }()
			mustNotPanic(t, c.name, func() {
				if err := c.call(p); !errors.Is(err, html.ErrInternalPanic) {
					t.Errorf("%s with panicking scorer = %v, want ErrInternalPanic", c.name, err)
				}
			})
		})
	}

	t.Run("landmark document skips the scorer", func(t *testing.T) {
		t.Parallel()
		p, err := html.New(newPanicScorerConfig())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = p.Close() }()
		mustNotPanic(t, "ReadPage(landmark)", func() {
			rr, err := p.ReadPage([]byte(landmark))
			if err != nil {
				t.Errorf("ReadPage(landmark) with panicking scorer = %v, want success (landmark path does not consult the scorer)", err)
				return
			}
			if len(rr.Sections) == 0 {
				t.Error("ReadPage(landmark) returned no sections")
			}
		})
	})
}

// ---------------------------------------------------------------------------
// Processor method nil-safety (from the dissolved boundary_test.go)
// ---------------------------------------------------------------------------

// TestProcessorMethodsNilSafety checks GetAuditLog/ClearAuditLog/ClearCache/
// ResetStatistics on a no-audit processor (from boundary_test.go; the nil
// receiver guards are pinned white-box in extract_test.go's
// TestProcessorNilGuardMethods and processor_adapter_internal_test.go).
func TestProcessorMethodsNilSafety(t *testing.T) {
	t.Parallel()

	t.Run("GetAuditLog on no-audit processor", func(t *testing.T) {
		p, _ := html.New()
		defer func() { _ = p.Close() }()

		entries := p.GetAuditLog()
		if len(entries) != 0 {
			t.Errorf("expected empty entries without audit, got %d", len(entries))
		}
	})

	t.Run("ClearAuditLog on no-audit processor", func(t *testing.T) {
		p, _ := html.New()
		defer func() { _ = p.Close() }()

		p.ClearAuditLog() // should not panic
	})

	t.Run("ClearCache on valid processor", func(t *testing.T) {
		p, _ := html.New()
		defer func() { _ = p.Close() }()

		p.ClearCache() // should not panic
	})

	t.Run("ResetStatistics on valid processor", func(t *testing.T) {
		p, _ := html.New()
		defer func() { _ = p.Close() }()

		p.ResetStatistics()
		stats := p.GetStatistics()
		if stats.TotalProcessed != 0 {
			t.Error("expected 0 after reset")
		}
	})
}
