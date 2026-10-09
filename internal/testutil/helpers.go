// Package testutil provides common test utilities for the html package.
package testutil

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/cybergodev/html"
)

// CreateTempHTML creates a temporary HTML file with the given content.
// Returns the file path. The file is automatically cleaned up after the test.
func CreateTempHTML(t *testing.T, content string) string {
	t.Helper()
	return CreateTempHTMLWithEncoding(t, []byte(content), "test_*.html")
}

// CreateTempHTMLWithEncoding creates a temporary HTML file with specific encoding bytes.
// The name parameter should be a pattern like "test_*.html".
func CreateTempHTMLWithEncoding(t *testing.T, content []byte, name string) string {
	t.Helper()

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, name)

	// Remove the * if present and replace with a timestamp-free name
	if filepath.Ext(tmpFile) == "" || len(filepath.Ext(tmpFile)) < 2 {
		tmpFile = tmpFile + ".html"
	}

	// Create the file without the glob pattern
	ext := filepath.Ext(tmpFile)
	base := tmpFile[:len(tmpFile)-len(ext)]
	if base[len(base)-1] == '*' {
		base = base[:len(base)-1] + "test"
		tmpFile = base + ext
	}

	if err := os.WriteFile(tmpFile, content, 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	return tmpFile
}

// AssertNoError asserts that err is nil, failing the test with msg if not.
func AssertNoError(t *testing.T, err error, msg string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", msg, err)
	}
}

// AssertError asserts that err is not nil, failing the test with msg if it is nil.
func AssertError(t *testing.T, err error, msg string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected error but got nil", msg)
	}
}

// AssertEqual asserts that two values are equal.
func AssertEqual[T comparable](t *testing.T, got, want T, msg string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %v, want %v", msg, got, want)
	}
}

// AssertContains asserts that s contains substr.
func AssertContains(t *testing.T, s, substr, msg string) {
	t.Helper()
	if !containsString(s, substr) {
		t.Errorf("%s: string does not contain substring\ngot: %s\nwant to contain: %s", msg, s, substr)
	}
}

// AssertNotContains asserts that s does not contain substr.
func AssertNotContains(t *testing.T, s, substr, msg string) {
	t.Helper()
	if containsString(s, substr) {
		t.Errorf("%s: string should not contain substring\ngot: %s\nshould not contain: %s", msg, s, substr)
	}
}

// AssertTrue asserts that condition is true.
func AssertTrue(t *testing.T, condition bool, msg string) {
	t.Helper()
	if !condition {
		t.Errorf("%s: expected true but got false", msg)
	}
}

// AssertFalse asserts that condition is false.
func AssertFalse(t *testing.T, condition bool, msg string) {
	t.Helper()
	if condition {
		t.Errorf("%s: expected false but got true", msg)
	}
}

// NewTestProcessor creates a Processor for testing with automatic cleanup.
// If no config is provided, uses DefaultConfig().
func NewTestProcessor(t *testing.T, cfg ...html.Config) *html.Processor {
	t.Helper()

	var c html.Config
	if len(cfg) > 0 {
		c = cfg[0]
	} else {
		c = html.DefaultConfig()
	}

	p, err := html.New(c)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	t.Cleanup(func() {
		_ = p.Close()
	})

	return p
}

// containsString is a helper to check if s contains substr.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

// findSubstring finds substr in s.
func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// CreateBatchTempFiles creates multiple temporary HTML files for batch testing.
// Returns slice of file paths. Files are automatically cleaned up after the test.
func CreateBatchTempFiles(t *testing.T, contents []string) []string {
	t.Helper()

	tmpDir := t.TempDir()
	files := make([]string, len(contents))

	for i, content := range contents {
		files[i] = filepath.Join(tmpDir, "file_"+string(rune('A'+i%26))+".html")
		if err := os.WriteFile(files[i], []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create temp file %d: %v", i, err)
		}
	}

	return files
}

// RunConcurrent executes fn concurrently n times, collecting any errors.
// Returns a slice of errors from each goroutine (nil if successful).
func RunConcurrent(n int, fn func(int) error) []error {
	var wg sync.WaitGroup
	errs := make([]error, n)
	var errsMu sync.Mutex

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if err := fn(idx); err != nil {
				errsMu.Lock()
				errs[idx] = err
				errsMu.Unlock()
			}
		}(i)
	}

	wg.Wait()
	return errs
}

// CommonHTMLSnippets contains common HTML test snippets.
var CommonHTMLSnippets = struct {
	// SimpleArticle is a basic article with title and content
	SimpleArticle string
	// ArticleWithLinks is an article containing links
	ArticleWithLinks string
	// ArticleWithImages is an article containing images
	ArticleWithImages string
	// ArticleWithVideos is an article containing video elements
	ArticleWithVideos string
	// XSSAttempt contains XSS payloads for security testing
	XSSAttempt string
	// ComplexArticle has nested elements and multiple content types
	ComplexArticle string
	// UnicodeContent contains various unicode characters
	UnicodeContent string
}{
	SimpleArticle: `<html><head><title>Test Article</title></head><body>
		<article>
			<h1>Article Title</h1>
			<p>This is the first paragraph with some content.</p>
			<p>This is the second paragraph with more content.</p>
		</article>
	</body></html>`,

	ArticleWithLinks: `<html><head><title>Article with Links</title></head><body>
		<article>
			<h1>Links Article</h1>
			<p>Check out <a href="https://example.com">Example Site</a> for more info.</p>
			<p>Also visit <a href="/internal" title="Internal Page">Internal Link</a>.</p>
		</article>
	</body></html>`,

	ArticleWithImages: `<html><head><title>Article with Images</title></head><body>
		<article>
			<h1>Images Article</h1>
			<p>Here is an image:</p>
			<img src="https://example.com/image.jpg" alt="Test Image" />
			<img src="/local-image.png" alt="Local Image" width="100" height="100" />
		</article>
	</body></html>`,

	ArticleWithVideos: `<html><head><title>Article with Videos</title></head><body>
		<article>
			<h1>Videos Article</h1>
			<video src="https://example.com/video.mp4" poster="poster.jpg">
				<source src="video.webm" type="video/webm">
			</video>
			<iframe src="https://www.youtube.com/embed/abc123"></iframe>
		</article>
	</body></html>`,

	XSSAttempt: `<html><body>
		<script>alert('XSS')</script>
		<div onclick="alert('XSS')">Content</div>
		<a href="javascript:alert('XSS')">Link</a>
		<img src="x" onerror="alert('XSS')" />
	</body></html>`,

	ComplexArticle: `<html><head><title>Complex Article</title></head><body>
		<header><nav><a href="/">Home</a></nav></header>
		<main>
			<article>
				<h1>Main Title</h1>
				<section>
					<h2>Section 1</h2>
					<p>Paragraph with <strong>bold</strong> and <em>italic</em> text.</p>
					<ul><li>Item 1</li><li>Item 2</li></ul>
				</section>
				<section>
					<h2>Section 2</h2>
					<table>
						<tr><th>Header</th></tr>
						<tr><td>Data</td></tr>
					</table>
				</section>
			</article>
		</main>
		<footer>Copyright</footer>
	</body></html>`,

	UnicodeContent: `<html><head><title>Unicode Test</title></head><body>
		<p>English: Hello World</p>
		<p>Chinese: 你好世界</p>
		<p>Japanese: こんにちは世界</p>
		<p>Korean: 안녕하세요 세계</p>
		<p>Emoji: 😀🎉🌍</p>
		<p>Special: — – " " ' ' … © ® ™</p>
	</body></html>`,
}
