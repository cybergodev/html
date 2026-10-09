package html_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cybergodev/html"
)

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
