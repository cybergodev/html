// Package html provides HTML content extraction with automatic encoding detection.
//
// This library extracts clean text, links, images, videos, and audio from HTML documents
// with automatic character encoding detection (supporting 15+ encodings including UTF-8,
// Windows-1252, GBK, Shift_JIS, etc.).
//
// Reader mode (ReadPage) keeps the whole page as labeled sections — header,
// navigation, main content, aside, footer — with inline hyperlinks rendered
// as Markdown links or HTML anchors, for feeding a clean page to LLMs and
// other text-only consumers:
//
//	markdown, err := html.ReadPageToMarkdown(htmlBytes)
//
// Basic usage:
//
//	// Simple extraction
//	result, err := html.Extract(htmlBytes)
//	fmt.Println(result.Text)
//
//	// With processor for repeated extractions
//	cfg := html.DefaultConfig()
//	cfg.MaxCacheEntries = 1000
//	cfg.CacheTTL = time.Hour
//	processor, _ := html.New(cfg)
//	defer processor.Close()
//	result, _ := processor.Extract(htmlBytes)
//
// See the examples/ directory for complete runnable programs covering HTTP
// integration, encoding detection, context cancellation, and secure file processing.
package html
