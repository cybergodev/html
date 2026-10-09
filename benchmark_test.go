package html_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cybergodev/html"
)

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
	p, _ := html.New()
	defer func() { _ = p.Close() }()

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
