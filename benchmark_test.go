package html_test

// benchmark_test.go — core extraction/media/links benchmarks plus the
// realistic-document profiling benchmarks merged from pprof_bench_test.go
// (sections marked below).

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cybergodev/html"
)

// /////////////////////////////////////////////////////////////////////////////
// From benchmark_test.go
// /////////////////////////////////////////////////////////////////////////////

func BenchmarkExtract(b *testing.B) {
	p, _ := html.New()
	defer func() { _ = p.Close() }()

	htmlContent := `
		<html>
		<head><title>Benchmark Test</title></head>
		<body>
			<article>
				<h1>Article Title</h1>
				<p>This is a paragraph with some content.</p>
				<p>Another paragraph with more text.</p>
				<img src="image.jpg" alt="Test Image">
				<a href="link.html">Test Link</a>
			</article>
		</body>
		</html>
	`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.Extract([]byte(htmlContent))
		if err != nil {
			b.Fatalf("Extract() failed: %v", err)
		}
	}
}

func BenchmarkExtractWithCache(b *testing.B) {
	p, _ := html.New()
	defer func() { _ = p.Close() }()

	htmlContent := `<html><body><p>Cached content</p></body></html>`

	// Prime the cache
	_, _ = p.Extract([]byte(htmlContent))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.Extract([]byte(htmlContent))
		if err != nil {
			b.Fatalf("Extract() failed: %v", err)
		}
	}
}

func BenchmarkExtractLargeDocument(b *testing.B) {
	p, _ := html.New()
	defer func() { _ = p.Close() }()

	// Create a large HTML document
	var sb strings.Builder
	sb.WriteString("<html><body><article>")
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&sb, "<h2>Section %d</h2>", i)
		for j := 0; j < 10; j++ {
			fmt.Fprintf(&sb, "<p>Paragraph %d in section %d with some content.</p>", j, i)
		}
	}
	sb.WriteString("</article></body></html>")
	htmlContent := sb.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.Extract([]byte(htmlContent))
		if err != nil {
			b.Fatalf("Extract() failed: %v", err)
		}
	}
}

func BenchmarkExtractWithImages(b *testing.B) {
	p, _ := html.New()
	defer func() { _ = p.Close() }()

	var sb strings.Builder
	sb.WriteString("<html><body>")
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&sb, `<img src="image%d.jpg" alt="Image %d">`, i, i)
	}
	sb.WriteString("</body></html>")
	htmlContent := sb.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.Extract([]byte(htmlContent))
		if err != nil {
			b.Fatalf("Extract() failed: %v", err)
		}
	}
}

func BenchmarkExtractWithLinks(b *testing.B) {
	p, _ := html.New()
	defer func() { _ = p.Close() }()

	var sb strings.Builder
	sb.WriteString("<html><body>")
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&sb, `<a href="link%d.html">Link %d</a>`, i, i)
	}
	sb.WriteString("</body></html>")
	htmlContent := sb.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.Extract([]byte(htmlContent))
		if err != nil {
			b.Fatalf("Extract() failed: %v", err)
		}
	}
}

func BenchmarkExtractBatch(b *testing.B) {
	p, _ := html.New()
	defer func() { _ = p.Close() }()

	htmlContents := make([][]byte, 10)
	for i := range htmlContents {
		htmlContents[i] = []byte(fmt.Sprintf(`<html><body><p>Content %d</p></body></html>`, i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		br := p.ExtractBatch(htmlContents)
		if br.Failed > 0 {
			b.Fatalf("ExtractBatch() failed: %v", br.Errors[0])
		}
	}
}

func BenchmarkNew(b *testing.B) {
	cfg := html.DefaultConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p, err := html.New(cfg)
		if err != nil {
			b.Fatalf("New() failed: %v", err)
		}
		_ = p.Close()
	}
}

func BenchmarkConcurrentExtract(b *testing.B) {
	p, _ := html.New()
	defer func() { _ = p.Close() }()

	htmlContent := `<html><body><p>Concurrent test</p></body></html>`

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := p.Extract([]byte(htmlContent))
			if err != nil {
				b.Fatalf("Extract() failed: %v", err)
			}
		}
	})
}

func BenchmarkArticleExtraction(b *testing.B) {
	p, _ := html.New()
	defer func() { _ = p.Close() }()

	htmlContent := `
		<html>
		<body>
			<nav>Navigation</nav>
			<article>
				<h1>Main Article</h1>
				<p>This is the main content.</p>
				<p>More content here.</p>
				<p>Even more content.</p>
			</article>
			<aside>Sidebar</aside>
			<footer>Footer</footer>
		</body>
		</html>
	`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.Extract([]byte(htmlContent))
		if err != nil {
			b.Fatalf("Extract() failed: %v", err)
		}
	}
}

func BenchmarkInlineImageFormatting(b *testing.B) {
	htmlContent := `
		<html>
		<body>
			<p>Before</p>
			<img src="test1.jpg" alt="Test 1">
			<p>Middle</p>
			<img src="test2.jpg" alt="Test 2">
			<p>After</p>
		</body>
		</html>
	`

	tests := []struct {
		name   string
		format string
	}{
		{"None", "none"},
		{"Placeholder", "placeholder"},
		{"Markdown", "markdown"},
		{"HTML", "html"},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			// One processor per format: the sub-benchmarks previously shared a
			// single default-config processor (InlineImageFormat "none"), so
			// all four measured the same path and tt.format was never applied.
			// Caching is disabled so each iteration runs the full formatting
			// pass the benchmark name promises.
			cfg := html.DefaultConfig()
			cfg.InlineImageFormat = tt.format
			cfg.MaxCacheEntries = 0
			p, err := html.New(cfg)
			if err != nil {
				b.Fatalf("New() failed: %v", err)
			}
			defer func() { _ = p.Close() }()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := p.Extract([]byte(htmlContent))
				if err != nil {
					b.Fatalf("Extract() failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkInlineLinkFormatting measures the [LINK:n] substitution pass on a
// link-dense document for each InlineLinkFormat (cache disabled so every
// iteration formats).
func BenchmarkInlineLinkFormatting(b *testing.B) {
	var sb strings.Builder
	sb.WriteString("<html><body><p>")
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&sb, `Text <a href="/p/%d" title="T%d">link %d</a> and more text. `, i, i, i)
	}
	sb.WriteString("</p></body></html>")
	htmlContent := sb.String()

	tests := []struct {
		name   string
		format string
	}{
		{"Markdown", "markdown"},
		{"HTML", "html"},
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			cfg := html.DefaultConfig()
			cfg.InlineLinkFormat = tt.format
			cfg.MaxCacheEntries = 0
			p, err := html.New(cfg)
			if err != nil {
				b.Fatalf("New() failed: %v", err)
			}
			defer func() { _ = p.Close() }()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				r, err := p.Extract([]byte(htmlContent))
				if err != nil {
					b.Fatalf("Extract() failed: %v", err)
				}
				if len(r.Links) == 0 {
					b.Fatal("expected links")
				}
			}
		})
	}
}

func BenchmarkMediaExtraction(b *testing.B) {
	p, _ := html.New()
	defer func() { _ = p.Close() }()

	htmlContent := `
		<html>
		<body>
			<img src="image.jpg" alt="Image">
			<video src="video.mp4"></video>
			<audio src="audio.mp3"></audio>
			<a href="link.html">Link</a>
		</body>
		</html>
	`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.Extract([]byte(htmlContent))
		if err != nil {
			b.Fatalf("Extract() failed: %v", err)
		}
	}
}

// /////////////////////////////////////////////////////////////////////////////
// From pprof_bench_test.go
// /////////////////////////////////////////////////////////////////////////////

// realisticDoc builds a representative HTML article with headings, paragraphs,
// images, links and a table — the workload a real extractor sees.
func realisticDoc() string {
	var sb strings.Builder
	sb.WriteString(`<html><head><title>Realistic Article</title></head><body>`)
	sb.WriteString(`<nav><a href="/home">Home</a> <a href="/about">About</a></nav>`)
	sb.WriteString(`<article><h1>Main Title</h1>`)
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&sb, "<h2>Section %d &amp; Notes</h2>", i)
		fmt.Fprintf(&sb, `<p>Paragraph %d has <a href="/p/%d">a link</a> and text content.</p>`, i, i)
		if i%5 == 0 {
			fmt.Fprintf(&sb, `<img src="/img/%d.jpg" alt="Image %d">`, i, i)
		}
	}
	sb.WriteString(`<table>`)
	for r := 0; r < 10; r++ {
		sb.WriteString("<tr><td>cell-a</td><td>cell-b</td><td>cell-c</td></tr>")
	}
	sb.WriteString(`</table>`)
	sb.WriteString(`<aside><a href="https://ext.example.com" rel="nofollow">external</a></aside>`)
	sb.WriteString(`<footer>Footer &copy; 2026</footer>`)
	sb.WriteString(`</article></body></html>`)
	return sb.String()
}

// BenchmarkRealisticNoCache measures the FULL extraction path (cache disabled).
func BenchmarkRealisticNoCache(b *testing.B) {
	cfg := html.DefaultConfig()
	cfg.MaxCacheEntries = 0 // disable cache to measure real extraction
	cfg.ProcessingTimeout = 0
	p, err := html.New(cfg)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	doc := []byte(realisticDoc())

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, err := p.Extract(doc)
		if err != nil {
			b.Fatal(err)
		}
		_ = r
	}
}

// realisticFormattedDoc builds the same shape of article as realisticDoc but
// pretty-printed: every element on its own line, indented. Server-rendered
// pages commonly look like this, and it exercises a path the compact builders
// do not — whitespace-only text nodes between tags, which the text pass must
// normalize (newlines) and then discard as empty content.
func realisticFormattedDoc() string {
	var sb strings.Builder
	sb.WriteString("<html>\n  <head>\n    <title>Formatted Article</title>\n  </head>\n  <body>\n")
	sb.WriteString("    <nav>\n      <a href=\"/home\">Home</a>\n      <a href=\"/about\">About</a>\n    </nav>\n")
	sb.WriteString("    <article>\n      <h1>Main Title</h1>\n")
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&sb, "      <h2>Section %d &amp; Notes</h2>\n", i)
		fmt.Fprintf(&sb, "      <p>Paragraph %d has <a href=\"/p/%d\">a link</a> and text content.</p>\n", i, i)
		if i%5 == 0 {
			fmt.Fprintf(&sb, "      <img src=\"/img/%d.jpg\" alt=\"Image %d\">\n", i, i)
		}
	}
	sb.WriteString("      <table>\n")
	for r := 0; r < 10; r++ {
		sb.WriteString("        <tr><td>cell-a</td><td>cell-b</td><td>cell-c</td></tr>\n")
	}
	sb.WriteString("      </table>\n")
	sb.WriteString("      <aside><a href=\"https://ext.example.com\" rel=\"nofollow\">external</a></aside>\n")
	sb.WriteString("      <footer>Footer &copy; 2026</footer>\n")
	sb.WriteString("    </article>\n  </body>\n</html>\n")
	return sb.String()
}

// BenchmarkRealisticFormatted measures the full extraction path on the
// pretty-printed document (cache and timeout disabled).
func BenchmarkRealisticFormatted(b *testing.B) {
	cfg := html.DefaultConfig()
	cfg.MaxCacheEntries = 0 // disable cache to measure real extraction
	cfg.ProcessingTimeout = 0
	p, err := html.New(cfg)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	doc := []byte(realisticFormattedDoc())

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, err := p.Extract(doc)
		if err != nil {
			b.Fatal(err)
		}
		if r.Title == "" {
			b.Fatal("expected a title")
		}
	}
}

// BenchmarkRealisticDefault measures the default-config path (cache + timeout goroutine).
func BenchmarkRealisticDefault(b *testing.B) {
	p, _ := html.New()
	defer func() { _ = p.Close() }()
	doc := []byte(realisticDoc())
	_, _ = p.Extract(doc) // prime cache

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, err := p.Extract(doc)
		if err != nil {
			b.Fatal(err)
		}
		_ = r
	}
}

// realisticClassHeavyDoc builds a page whose elements nearly all carry class/id
// attributes — the shape of real-world CMS output. It exercises the scorer's
// class/id pattern matching (ShouldRemove, scoreAttributes) on every element,
// which the class-free realisticDoc does not.
func realisticClassHeavyDoc() string {
	var sb strings.Builder
	sb.WriteString(`<html><head><title>Class Heavy Page</title></head><body>`)
	sb.WriteString(`<div class="site-header header-wrapper" id="page-header"><nav class="main-navigation nav-menu"><ul class="menu-list">`)
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&sb, `<li class="menu-item item-%d"><a class="nav-link" href="/nav/%d">Nav %d</a></li>`, i, i, i)
	}
	sb.WriteString(`</ul></nav></div>`)
	sb.WriteString(`<main class="content-area main-content article-body" role="main">`)
	sb.WriteString(`<article class="post-content entry-content story">`)
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&sb, `<section class="section-block content-section-%d">`, i)
		fmt.Fprintf(&sb, `<h2 class="section-title heading">Section %d &amp; Notes</h2>`, i)
		fmt.Fprintf(&sb, `<div class="text-wrapper prose"><p class="paragraph text-body">Paragraph %d has <a class="inline-link content-link" href="/p/%d">a link</a> and text with commas, and, more commas.</p></div>`, i, i)
		if i%3 == 0 {
			fmt.Fprintf(&sb, `<aside class="related-posts sidebar-widget widget-box"><p class="promo-box banner-ad">Promo %d</p></aside>`, i)
		}
		sb.WriteString(`</section>`)
	}
	sb.WriteString(`</article></main>`)
	sb.WriteString(`<footer class="site-footer footer-bottom"><div class="footer-widgets widget-area"><p class="copyright">Copyright 2026</p></div></footer>`)
	sb.WriteString(`</body></html>`)
	return sb.String()
}

// BenchmarkRealisticClassHeavy measures the full extraction path on a
// class/id-heavy document (cache and timeout disabled), where per-element
// class/id pattern scoring dominates the library-controlled CPU profile.
func BenchmarkRealisticClassHeavy(b *testing.B) {
	cfg := html.DefaultConfig()
	cfg.MaxCacheEntries = 0 // disable cache to measure real extraction
	cfg.ProcessingTimeout = 0
	p, err := html.New(cfg)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	doc := []byte(realisticClassHeavyDoc())

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, err := p.Extract(doc)
		if err != nil {
			b.Fatal(err)
		}
		if r.Title == "" {
			b.Fatal("expected a title")
		}
	}
}

// realisticCJKDoc builds a representative Chinese-language article. CJK text is
// UTF-8 multi-byte, so the encoding fast path (isPureASCII) does not apply: the
// document exercises the valid-UTF-8 branch of DetectAndConvertToUTF8String and
// the CJK word-counting path, neither of which the ASCII realisticDoc covers.
func realisticCJKDoc() string {
	var sb strings.Builder
	sb.WriteString(`<html><head><title>中文文章标题</title></head><body>`)
	sb.WriteString(`<nav><a href="/首页">首页</a> <a href="/关于">关于我们</a></nav>`)
	sb.WriteString(`<article><h1>正文主标题</h1>`)
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&sb, "<h2>第 %d 节 &amp; 备注</h2>", i)
		fmt.Fprintf(&sb, `<p>第 %d 段包含<a href="/p/%d">一个链接</a>以及正文文字内容，中文之间没有空格。</p>`, i, i)
		if i%5 == 0 {
			fmt.Fprintf(&sb, `<img src="/img/%d.jpg" alt="图片 %d">`, i, i)
		}
	}
	sb.WriteString(`<table>`)
	for r := 0; r < 8; r++ {
		sb.WriteString("<tr><td>单元格甲</td><td>单元格乙</td></tr>")
	}
	sb.WriteString(`</table>`)
	sb.WriteString(`<footer>页脚 &copy; 2026</footer>`)
	sb.WriteString(`</article></body></html>`)
	return sb.String()
}

// BenchmarkRealisticCJK measures the full extraction path on a Chinese-language
// document (cache and timeout disabled), where encoding conversion operates on
// multi-byte UTF-8 rather than pure ASCII.
func BenchmarkRealisticCJK(b *testing.B) {
	cfg := html.DefaultConfig()
	cfg.MaxCacheEntries = 0 // disable cache to measure real extraction
	cfg.ProcessingTimeout = 0
	p, err := html.New(cfg)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	doc := []byte(realisticCJKDoc())

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, err := p.Extract(doc)
		if err != nil {
			b.Fatal(err)
		}
		if r.Title == "" {
			b.Fatal("expected a title")
		}
	}
}

// BenchmarkRealisticDefaultNoCache measures the default-config path for a
// cache miss on a unique document: ProcessingTimeout stays at its 30s default,
// so every call pays the timeout machinery (watchdog goroutine, derived
// context, buffered result channel) and the safe-copy encoding decode that the
// timeout's straggler contract requires. BenchmarkRealisticNoCache (timeout 0)
// isolates the same extraction without that overhead.
func BenchmarkRealisticDefaultNoCache(b *testing.B) {
	cfg := html.DefaultConfig()
	cfg.MaxCacheEntries = 0 // disable cache to measure real extraction
	p, err := html.New(cfg)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	doc := []byte(realisticDoc())

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, err := p.Extract(doc)
		if err != nil {
			b.Fatal(err)
		}
		if r.Title == "" {
			b.Fatal("expected a title")
		}
	}
}

// BenchmarkRealisticWithMedia exercises the gate's "true" branch: the document
// actually contains video/audio URLs, so the regex scan still runs. This guards
// against a regression where gating would make media-bearing documents slower or
// miss results; numbers should be ~unchanged versus the pre-gate behavior.
func BenchmarkRealisticWithMedia(b *testing.B) {
	cfg := html.DefaultConfig()
	cfg.MaxCacheEntries = 0 // disable cache to measure real extraction
	cfg.ProcessingTimeout = 0
	p, err := html.New(cfg)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	var sb strings.Builder
	sb.WriteString(`<html><body><article>`)
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&sb, `<p>Section <a href="/s/%d">%d</a> media below.</p>`, i, i)
		fmt.Fprintf(&sb, `<video src="https://cdn.example.com/v/%d.mp4"></video>`, i)
		fmt.Fprintf(&sb, `<audio src="https://cdn.example.com/a/%d.mp3"></audio>`, i)
	}
	sb.WriteString(`</article></body></html>`)
	doc := []byte(sb.String())

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, err := p.Extract(doc)
		if err != nil {
			b.Fatal(err)
		}
		if len(r.Videos) == 0 || len(r.Audios) == 0 {
			b.Fatalf("expected media results, got videos=%d audios=%d", len(r.Videos), len(r.Audios))
		}
	}
}
