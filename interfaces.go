package html

import "context"

// Extractor defines the complete interface for HTML content extraction.
// This is the primary interface for mocking and testing.
//
// Package-level convenience functions mirror every extraction method of this
// interface, accepting an optional Config variadic parameter (cfg ...Config).
// When no config is provided, DefaultConfig() is used. Close is the one
// exception: it has no package-level counterpart, because the convenience
// functions run on pooled or one-shot processors whose cleanup they manage
// internally.
//
// Every method is implemented by [Processor]; see those methods for full
// semantics and the errors they can return.
type Extractor interface {
	// Core extraction

	// Extract extracts content (text, media, metadata) from HTML bytes.
	Extract(htmlBytes []byte) (*Result, error)
	// ExtractWithContext is like Extract but honors context cancellation.
	ExtractWithContext(ctx context.Context, htmlBytes []byte) (*Result, error)
	// ExtractFromFile extracts content from an HTML file on disk.
	ExtractFromFile(filePath string) (*Result, error)
	// ExtractFromFileWithContext is like ExtractFromFile but honors context cancellation.
	ExtractFromFileWithContext(ctx context.Context, filePath string) (*Result, error)

	// Text extraction

	// ExtractText extracts only the plain text from HTML bytes.
	ExtractText(htmlBytes []byte) (string, error)
	// ExtractTextFromFile extracts only the plain text from an HTML file.
	ExtractTextFromFile(filePath string) (string, error)
	// ExtractTextWithContext is like ExtractText but honors context cancellation.
	ExtractTextWithContext(ctx context.Context, htmlBytes []byte) (string, error)
	// ExtractTextFromFileWithContext is like ExtractTextFromFile but honors context cancellation.
	ExtractTextFromFileWithContext(ctx context.Context, filePath string) (string, error)

	// Formatted output

	// ExtractToMarkdown extracts content and renders inline images/links as Markdown.
	ExtractToMarkdown(htmlBytes []byte) (string, error)
	// ExtractToMarkdownFromFile is the file-path variant of ExtractToMarkdown.
	ExtractToMarkdownFromFile(filePath string) (string, error)
	// ExtractToJSON extracts content and marshals the Result to JSON.
	ExtractToJSON(htmlBytes []byte) ([]byte, error)
	// ExtractToJSONFromFile is the file-path variant of ExtractToJSON.
	ExtractToJSONFromFile(filePath string) ([]byte, error)
	// ExtractToMarkdownWithContext is like ExtractToMarkdown but honors context cancellation.
	ExtractToMarkdownWithContext(ctx context.Context, htmlBytes []byte) (string, error)
	// ExtractToMarkdownFromFileWithContext is like ExtractToMarkdownFromFile but honors context cancellation.
	ExtractToMarkdownFromFileWithContext(ctx context.Context, filePath string) (string, error)
	// ExtractToJSONWithContext is like ExtractToJSON but honors context cancellation.
	ExtractToJSONWithContext(ctx context.Context, htmlBytes []byte) ([]byte, error)
	// ExtractToJSONFromFileWithContext is like ExtractToJSONFromFile but honors context cancellation.
	ExtractToJSONFromFileWithContext(ctx context.Context, filePath string) ([]byte, error)

	// Batch processing

	// ExtractBatch extracts content from many HTML byte slices concurrently.
	ExtractBatch(htmlContents [][]byte) *BatchResult
	// ExtractBatchWithContext is like ExtractBatch but honors context cancellation.
	ExtractBatchWithContext(ctx context.Context, htmlContents [][]byte) *BatchResult
	// ExtractBatchFiles extracts content from many HTML files concurrently.
	ExtractBatchFiles(filePaths []string) *BatchResult
	// ExtractBatchFilesWithContext is like ExtractBatchFiles but honors context cancellation.
	ExtractBatchFilesWithContext(ctx context.Context, filePaths []string) *BatchResult

	// Link extraction

	// ExtractAllLinks enumerates every link resource (a, img, media, script, link, iframe) without applying sanitization.
	ExtractAllLinks(htmlBytes []byte) ([]LinkResource, error)
	// ExtractAllLinksFromFile is the file-path variant of ExtractAllLinks.
	ExtractAllLinksFromFile(filePath string) ([]LinkResource, error)
	// ExtractAllLinksWithContext is like ExtractAllLinks but honors context cancellation.
	ExtractAllLinksWithContext(ctx context.Context, htmlBytes []byte) ([]LinkResource, error)
	// ExtractAllLinksFromFileWithContext is like ExtractAllLinksFromFile but honors context cancellation.
	ExtractAllLinksFromFileWithContext(ctx context.Context, filePath string) ([]LinkResource, error)

	// Resource cleanup

	// Close releases the processor's resources (cache, background goroutines).
	Close() error
}

// StatsProvider defines the interface for statistics and cache management.
type StatsProvider interface {
	// GetStatistics returns current processing statistics.
	GetStatistics() Statistics
	// ClearCache clears the cache contents but preserves cumulative statistics.
	ClearCache()
	// ResetStatistics resets all statistics counters to zero.
	ResetStatistics()
}

// Compile-time assertions to ensure Processor implements all interfaces.
var (
	_ Extractor     = (*Processor)(nil)
	_ StatsProvider = (*Processor)(nil)
)
