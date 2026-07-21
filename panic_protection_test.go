package html_test

// panic_protection_test.go - Verifies that all public API surfaces
// recover from internal panics and return ErrInternalPanic instead of crashing.
// This implements the SEC-003 security hardening requirement.

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cybergodev/html"
)

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
const testLinkHTML = `<html><body><a href="https://example.com">Link</a></body></html>`

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
			defer p.Close()

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
		defer p.Close()

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
		defer p.Close()

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
		defer p.Close()

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

	t.Run("batch with mixed valid/empty content", func(t *testing.T) {
		p, err := html.New()
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		br := p.ExtractBatch([][]byte{
			[]byte(testHTML),
			[]byte{},
			[]byte(`<html><body><p>Another</p></body></html>`),
		})
		if br.Failed != 0 {
			t.Fatalf("expected 0 failures, got %d", br.Failed)
		}
		if br.Success != 3 {
			t.Fatalf("expected 3 successes, got %d", br.Success)
		}
	})
}

// TestPanicRecovery_LinkExtraction verifies link extraction safety.
func TestPanicRecovery_LinkExtraction(t *testing.T) {
	t.Parallel()

	t.Run("normal links don't panic", func(t *testing.T) {
		p, err := html.New()
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		links, err := p.ExtractAllLinks([]byte(testLinkHTML))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(links) == 0 {
			t.Fatal("expected at least one link")
		}
	})

	t.Run("empty input returns empty links", func(t *testing.T) {
		p, err := html.New()
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		links, err := p.ExtractAllLinks([]byte{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(links) != 0 {
			t.Fatalf("expected 0 links, got %d", len(links))
		}
	})

	t.Run("context with links", func(t *testing.T) {
		p, err := html.New()
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		links, err := p.ExtractAllLinksWithContext(context.Background(), []byte(testLinkHTML))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(links) == 0 {
			t.Fatal("expected at least one link")
		}
	})

	t.Run("cancelled context returns context.Canceled", func(t *testing.T) {
		p, err := html.New()
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = p.ExtractAllLinksWithContext(ctx, []byte(testLinkHTML))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got: %v", err)
		}
	})

	t.Run("package-level link extraction", func(t *testing.T) {
		links, err := html.ExtractAllLinks([]byte(testLinkHTML))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(links) == 0 {
			t.Fatal("expected at least one link")
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
		p.Close()

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
	defer p.Close()

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
	defer p.Close()

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

// TestPanicRecovery_ConfigErrorsNoPanic verifies config errors don't panic.
func TestPanicRecovery_ConfigErrorsNoPanic(t *testing.T) {
	t.Parallel()

	t.Run("invalid config returns error", func(t *testing.T) {
		cfg := html.Config{MaxInputSize: -1}
		_, err := html.New(cfg)
		if !errors.Is(err, html.ErrInvalidConfig) {
			t.Fatalf("expected ErrInvalidConfig, got: %v", err)
		}
	})

	t.Run("multiple configs returns error", func(t *testing.T) {
		cfg1 := html.DefaultConfig()
		cfg2 := html.DefaultConfig()
		_, err := html.New(cfg1, cfg2)
		if !errors.Is(err, html.ErrMultipleConfigs) {
			t.Fatalf("expected ErrMultipleConfigs, got: %v", err)
		}
	})
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
