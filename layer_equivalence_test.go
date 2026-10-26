package html_test

// layer_equivalence_test.go - D-006 dual-layer API consistency regression tests.
// For identical input and Config, every package-level convenience function
// must return the same output as its Processor-method counterpart. The only
// sanctioned differences are lifecycle ones: package-level calls disable the
// cache and surface config-resolution errors per call.
// ProcessingTime is excluded from Result comparisons — it measures
// wall-clock work, which legitimately differs between a cached Processor
// extraction and a one-shot package call.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/cybergodev/html"
)

// equivalenceDoc covers every extraction surface: metadata, links, images,
// and a table, so a format or config field that only affects one aspect still
// shows up in the compared output.
const equivalenceDoc = `<!DOCTYPE html>
<html><head>
<title>Equivalence Page</title>
<meta name="author" content="Jane Doe">
<link rel="canonical" href="https://example.com/canon">
</head><body>
<nav><a href="/nav">Nav</a></nav>
<main>
<h1>Main Title</h1>
<p>Intro paragraph with a <a href="https://example.org/target">link</a>.</p>
<img src="pic.jpg" alt="A picture">
<table><tr><td>cell</td><td>two</td></tr></table>
</main>
<footer>Footer text</footer>
</body></html>`

// assertResultsEqual compares every deterministic field of two Results.
func assertResultsEqual(t *testing.T, name string, got, want *html.Result) {
	t.Helper()
	if got == nil || want == nil {
		t.Fatalf("%s: nil result (got=%v want=%v)", name, got, want)
	}
	if got.Text != want.Text {
		t.Errorf("%s: Text = %q, want %q", name, got.Text, want.Text)
	}
	if got.Title != want.Title {
		t.Errorf("%s: Title = %q, want %q", name, got.Title, want.Title)
	}
	if got.Author != want.Author || got.Description != want.Description ||
		got.SiteName != want.SiteName || got.CanonicalURL != want.CanonicalURL {
		t.Errorf("%s: metadata = (%q,%q,%q,%q), want (%q,%q,%q,%q)", name,
			got.Author, got.Description, got.SiteName, got.CanonicalURL,
			want.Author, want.Description, want.SiteName, want.CanonicalURL)
	}
	if got.WordCount != want.WordCount {
		t.Errorf("%s: WordCount = %d, want %d", name, got.WordCount, want.WordCount)
	}
	if got.ReadingTime != want.ReadingTime {
		t.Errorf("%s: ReadingTime = %v, want %v", name, got.ReadingTime, want.ReadingTime)
	}
	if got.Confidence != want.Confidence {
		t.Errorf("%s: Confidence = %v, want %v", name, got.Confidence, want.Confidence)
	}
	if got.ExtractionMethod != want.ExtractionMethod {
		t.Errorf("%s: ExtractionMethod = %q, want %q", name, got.ExtractionMethod, want.ExtractionMethod)
	}
	if !reflect.DeepEqual(got.Images, want.Images) {
		t.Errorf("%s: Images = %+v, want %+v", name, got.Images, want.Images)
	}
	if !reflect.DeepEqual(got.Links, want.Links) {
		t.Errorf("%s: Links = %+v, want %+v", name, got.Links, want.Links)
	}
	if !reflect.DeepEqual(got.Videos, want.Videos) {
		t.Errorf("%s: Videos = %+v, want %+v", name, got.Videos, want.Videos)
	}
	if !reflect.DeepEqual(got.Audios, want.Audios) {
		t.Errorf("%s: Audios = %+v, want %+v", name, got.Audios, want.Audios)
	}
}

// writeEquivalenceDoc writes the fixture to a temp file and returns its path.
func writeEquivalenceDoc(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "doc.html")
	if err := os.WriteFile(path, []byte(equivalenceDoc), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

// TestLayerEquivalenceExtract verifies the Extract family returns identical
// Results through both layers, with the default and a custom Config.
func TestLayerEquivalenceExtract(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	doc := []byte(equivalenceDoc)
	for _, cfg := range []html.Config{html.DefaultConfig(), html.MarkdownConfig()} {
		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer func() { _ = p.Close() }()

		want, err := p.Extract(doc)
		if err != nil {
			t.Fatalf("processor Extract() failed: %v", err)
		}
		got, err := html.Extract(doc, cfg)
		if err != nil {
			t.Fatalf("package Extract() failed: %v", err)
		}
		assertResultsEqual(t, "Extract", got, want)

		wantCtx, err := p.ExtractWithContext(ctx, doc)
		if err != nil {
			t.Fatalf("processor ExtractWithContext() failed: %v", err)
		}
		gotCtx, err := html.ExtractWithContext(ctx, doc, cfg)
		if err != nil {
			t.Fatalf("package ExtractWithContext() failed: %v", err)
		}
		assertResultsEqual(t, "ExtractWithContext", gotCtx, wantCtx)
	}
}

// TestLayerEquivalenceFromFile verifies the file-based Extract variants agree
// across layers (they share the file-validation path, but the package wrapper
// is a separate entry point that must not diverge).
func TestLayerEquivalenceFromFile(t *testing.T) {
	t.Parallel()

	path := writeEquivalenceDoc(t)
	p, err := html.New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = p.Close() }()

	want, err := p.ExtractFromFile(path)
	if err != nil {
		t.Fatalf("processor ExtractFromFile() failed: %v", err)
	}
	got, err := html.ExtractFromFile(path)
	if err != nil {
		t.Fatalf("package ExtractFromFile() failed: %v", err)
	}
	assertResultsEqual(t, "ExtractFromFile", got, want)

	wantText, err := p.ExtractTextFromFile(path)
	if err != nil {
		t.Fatalf("processor ExtractTextFromFile() failed: %v", err)
	}
	gotText, err := html.ExtractTextFromFile(path)
	if err != nil {
		t.Fatalf("package ExtractTextFromFile() failed: %v", err)
	}
	if gotText != wantText {
		t.Errorf("ExtractTextFromFile = %q, want %q", gotText, wantText)
	}
}

// TestLayerEquivalenceTextAndOutput verifies the plain-string outputs are
// byte-identical across layers: text, Markdown, and the link-free reader form.
// JSON is compared after unmarshaling, because processing_time_ms measures
// wall-clock time and legitimately differs between runs.
func TestLayerEquivalenceTextAndOutput(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	doc := []byte(equivalenceDoc)
	p, err := html.New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = p.Close() }()

	stringPairs := []struct {
		name      string
		processor func() (string, error)
		packageFn func() (string, error)
	}{
		{"ExtractText", func() (string, error) { return p.ExtractText(doc) },
			func() (string, error) { return html.ExtractText(doc) }},
		{"ExtractTextWithContext", func() (string, error) { return p.ExtractTextWithContext(ctx, doc) },
			func() (string, error) { return html.ExtractTextWithContext(ctx, doc) }},
		{"ExtractToMarkdown", func() (string, error) { return p.ExtractToMarkdown(doc) },
			func() (string, error) { return html.ExtractToMarkdown(doc) }},
		{"ExtractToMarkdownWithContext", func() (string, error) { return p.ExtractToMarkdownWithContext(ctx, doc) },
			func() (string, error) { return html.ExtractToMarkdownWithContext(ctx, doc) }},
		{"ReadPageToMarkdown", func() (string, error) { return p.ReadPageToMarkdown(doc) },
			func() (string, error) { return html.ReadPageToMarkdown(doc) }},
		{"ReadPageToMarkdownText", func() (string, error) { return p.ReadPageToMarkdownText(doc) },
			func() (string, error) { return html.ReadPageToMarkdownText(doc) }},
	}
	for _, pair := range stringPairs {
		want, err := pair.processor()
		if err != nil {
			t.Fatalf("%s: processor layer failed: %v", pair.name, err)
		}
		got, err := pair.packageFn()
		if err != nil {
			t.Fatalf("%s: package layer failed: %v", pair.name, err)
		}
		if got != want {
			t.Errorf("%s: package = processor mismatch\npackage:   %q\nprocessor: %q", pair.name, got, want)
		}
	}

	wantJSON, err := p.ExtractToJSON(doc)
	if err != nil {
		t.Fatalf("processor ExtractToJSON() failed: %v", err)
	}
	gotJSON, err := html.ExtractToJSON(doc)
	if err != nil {
		t.Fatalf("package ExtractToJSON() failed: %v", err)
	}
	var wantMap, gotMap map[string]any
	if err := json.Unmarshal(wantJSON, &wantMap); err != nil {
		t.Fatalf("unmarshal processor JSON: %v", err)
	}
	if err := json.Unmarshal(gotJSON, &gotMap); err != nil {
		t.Fatalf("unmarshal package JSON: %v", err)
	}
	delete(wantMap, "processing_time_ms")
	delete(gotMap, "processing_time_ms")
	if !reflect.DeepEqual(gotMap, wantMap) {
		t.Errorf("ExtractToJSON differs after dropping processing_time_ms:\npackage:   %v\nprocessor: %v", gotMap, wantMap)
	}
}

// TestLayerEquivalenceLinks verifies ExtractAllLinks returns the identical
// (URL-sorted) slice through both layers.
func TestLayerEquivalenceLinks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	doc := []byte(equivalenceDoc)
	p, err := html.New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = p.Close() }()

	want, err := p.ExtractAllLinks(doc)
	if err != nil {
		t.Fatalf("processor ExtractAllLinks() failed: %v", err)
	}
	got, err := html.ExtractAllLinks(doc)
	if err != nil {
		t.Fatalf("package ExtractAllLinks() failed: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractAllLinks = %+v, want %+v", got, want)
	}

	wantCtx, err := p.ExtractAllLinksWithContext(ctx, doc)
	if err != nil {
		t.Fatalf("processor ExtractAllLinksWithContext() failed: %v", err)
	}
	gotCtx, err := html.ExtractAllLinksWithContext(ctx, doc)
	if err != nil {
		t.Fatalf("package ExtractAllLinksWithContext() failed: %v", err)
	}
	if !reflect.DeepEqual(gotCtx, wantCtx) {
		t.Errorf("ExtractAllLinksWithContext = %+v, want %+v", gotCtx, wantCtx)
	}
}

// TestLayerEquivalenceBatch verifies the batch families agree on counts and
// per-item text (ProcessingTime inside each Result is excluded, as above).
func TestLayerEquivalenceBatch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	docs := [][]byte{[]byte(equivalenceDoc), []byte("<p>tiny</p>"), []byte(equivalenceDoc)}
	path := writeEquivalenceDoc(t)
	p, err := html.New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = p.Close() }()

	assertBatchEqual := func(name string, got, want *html.BatchResult) {
		t.Helper()
		if got.Success != want.Success || got.Failed != want.Failed || got.Cancelled != want.Cancelled {
			t.Errorf("%s: counts = (%d,%d,%d), want (%d,%d,%d)", name,
				got.Success, got.Failed, got.Cancelled, want.Success, want.Failed, want.Cancelled)
		}
		for i := range want.Results {
			if (got.Results[i] == nil) != (want.Results[i] == nil) {
				t.Errorf("%s: item %d nil mismatch (got=%v want=%v)", name, i, got.Results[i], want.Results[i])
				continue
			}
			if got.Results[i] != nil && got.Results[i].Text != want.Results[i].Text {
				t.Errorf("%s: item %d Text = %q, want %q", name, i, got.Results[i].Text, want.Results[i].Text)
			}
		}
	}

	assertBatchEqual("ExtractBatch", html.ExtractBatch(docs), p.ExtractBatch(docs))
	assertBatchEqual("ExtractBatchWithContext",
		html.ExtractBatchWithContext(ctx, docs), p.ExtractBatchWithContext(ctx, docs))
	assertBatchEqual("ExtractBatchFiles",
		html.ExtractBatchFiles([]string{path, "missing-file.html", path}),
		p.ExtractBatchFiles([]string{path, "missing-file.html", path}))
}

// TestLayerEquivalenceRead verifies the reader-mode family agrees across
// layers via the deterministic RenderText projection and section count.
func TestLayerEquivalenceRead(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	doc := []byte(equivalenceDoc)
	path := writeEquivalenceDoc(t)
	p, err := html.New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = p.Close() }()

	want, err := p.ReadPage(doc)
	if err != nil {
		t.Fatalf("processor ReadPage() failed: %v", err)
	}
	got, err := html.ReadPage(doc)
	if err != nil {
		t.Fatalf("package ReadPage() failed: %v", err)
	}
	if got.Title != want.Title {
		t.Errorf("ReadPage Title = %q, want %q", got.Title, want.Title)
	}
	if len(got.Sections) != len(want.Sections) {
		t.Errorf("ReadPage section count = %d, want %d", len(got.Sections), len(want.Sections))
	} else {
		for i := range want.Sections {
			if got.Sections[i].Kind != want.Sections[i].Kind ||
				got.Sections[i].Text != want.Sections[i].Text ||
				got.Sections[i].PlainText != want.Sections[i].PlainText {
				t.Errorf("ReadPage section %d = (%q, %d chars), want (%q, %d chars)", i,
					got.Sections[i].Kind, len(got.Sections[i].Text),
					want.Sections[i].Kind, len(want.Sections[i].Text))
			}
		}
	}
	if got.RenderText() != want.RenderText() {
		t.Errorf("ReadPage RenderText differs between layers")
	}

	wantCtx, err := p.ReadPageWithContext(ctx, doc)
	if err != nil {
		t.Fatalf("processor ReadPageWithContext() failed: %v", err)
	}
	gotCtx, err := html.ReadPageWithContext(ctx, doc)
	if err != nil {
		t.Fatalf("package ReadPageWithContext() failed: %v", err)
	}
	if gotCtx.RenderText() != wantCtx.RenderText() {
		t.Errorf("ReadPageWithContext RenderText differs between layers")
	}

	wantFile, err := p.ReadPageFromFileToMarkdown(path)
	if err != nil {
		t.Fatalf("processor ReadPageFromFileToMarkdown() failed: %v", err)
	}
	gotFile, err := html.ReadPageFromFileToMarkdown(path)
	if err != nil {
		t.Fatalf("package ReadPageFromFileToMarkdown() failed: %v", err)
	}
	if gotFile != wantFile {
		t.Errorf("ReadPageFromFileToMarkdown differs between layers")
	}
}

// TestLayerEquivalenceErrors verifies the two layers surface the same
// sentinel errors for the same conditions, plus the package-only config
// errors (surfaced per call rather than at New).
func TestLayerEquivalenceErrors(t *testing.T) {
	t.Parallel()

	cfg := html.DefaultConfig()
	cfg.MaxInputSize = 64 // smaller than equivalenceDoc
	oversize := []byte(equivalenceDoc)

	p, err := html.New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = p.Close() }()

	_, wantErr := p.Extract(oversize)
	if !errors.Is(wantErr, html.ErrInputTooLarge) {
		t.Fatalf("processor Extract() oversize error = %v, want ErrInputTooLarge", wantErr)
	}
	_, gotErr := html.Extract(oversize, cfg)
	if !errors.Is(gotErr, html.ErrInputTooLarge) {
		t.Errorf("package Extract() oversize error = %v, want ErrInputTooLarge", gotErr)
	}

	_, wantFileErr := p.ExtractFromFile("definitely-missing-d006.html")
	if !errors.Is(wantFileErr, html.ErrFileNotFound) {
		t.Fatalf("processor ExtractFromFile() error = %v, want ErrFileNotFound", wantFileErr)
	}
	_, gotFileErr := html.ExtractFromFile("definitely-missing-d006.html")
	if !errors.Is(gotFileErr, html.ErrFileNotFound) {
		t.Errorf("package ExtractFromFile() error = %v, want ErrFileNotFound", gotFileErr)
	}

	// Package-only: config resolution happens per call.
	if _, err := html.Extract(oversize, cfg, cfg); !errors.Is(err, html.ErrMultipleConfigs) {
		t.Errorf("Extract(2 configs) error = %v, want ErrMultipleConfigs", err)
	}
	badCfg := html.DefaultConfig()
	badCfg.WorkerPoolSize = 0
	if _, err := html.Extract(oversize, badCfg); !errors.Is(err, html.ErrInvalidConfig) {
		t.Errorf("Extract(invalid config) error = %v, want ErrInvalidConfig", err)
	}
	if _, err := html.New(badCfg); !errors.Is(err, html.ErrInvalidConfig) {
		t.Errorf("New(invalid config) error = %v, want ErrInvalidConfig", err)
	}
}

// ---------------------------------------------------------------------------
// WithContext wrapper boundaries (from the dissolved boundary_test.go)
// ---------------------------------------------------------------------------

// TestPackageLevelWithContextFunctions covers the package-level Extract family
// WithContext entry points, including the cancelled-context error contract.
func TestPackageLevelWithContextFunctions(t *testing.T) {
	t.Parallel()

	t.Run("ExtractWithContext", func(t *testing.T) {
		result, err := html.ExtractWithContext(context.Background(), []byte(boundaryTestHTML))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Title != "Boundary Test" {
			t.Errorf("Title = %q, want 'Boundary Test'", result.Title)
		}
	})

	t.Run("ExtractTextWithContext", func(t *testing.T) {
		text, err := html.ExtractTextWithContext(context.Background(), []byte(boundaryTestHTML))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if text == "" {
			t.Error("expected non-empty text")
		}
	})

	t.Run("ExtractFromFileWithContext", func(t *testing.T) {
		tmpFile := writeTempHTML(t, boundaryTestHTML)

		result, err := html.ExtractFromFileWithContext(context.Background(), tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Title != "Boundary Test" {
			t.Errorf("Title = %q, want 'Boundary Test'", result.Title)
		}
	})

	t.Run("ExtractTextFromFileWithContext", func(t *testing.T) {
		tmpFile := writeTempHTML(t, boundaryTestHTML)

		text, err := html.ExtractTextFromFileWithContext(context.Background(), tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if text == "" {
			t.Error("expected non-empty text")
		}
	})

	t.Run("cancelled context returns error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := html.ExtractWithContext(ctx, []byte(boundaryTestHTML))
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got: %v", err)
		}
	})
}

// TestExtractTextFromFileWithContext_Coverage covers the Processor-method
// variant of ExtractTextFromFileWithContext on both its happy and error paths.
func TestExtractTextFromFileWithContext_Coverage(t *testing.T) {
	t.Parallel()

	t.Run("valid file", func(t *testing.T) {
		t.Parallel()
		p, err := html.New()
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = p.Close() }()

		tmpFile := writeTempHTML(t, boundaryTestHTML)
		text, err := p.ExtractTextFromFileWithContext(context.Background(), tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if text == "" {
			t.Error("expected non-empty text")
		}
	})

	t.Run("non-existent file", func(t *testing.T) {
		t.Parallel()
		p, err := html.New()
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = p.Close() }()

		_, err = p.ExtractTextFromFileWithContext(context.Background(), "nonexistent.html")
		if err == nil {
			t.Error("expected error for non-existent file")
		}
	})
}
