# HTML Content Extraction Library

`github.com/cybergodev/html` is a feature-rich HTML content extraction library with automatic encoding detection, content sanitization, caching, and batch processing.

> **Note:** This library uses `golang.org/x/net/html` internally for HTML parsing but does **not** re-export its types or functions. It is not a drop-in replacement for `golang.org/x/net/html`. It is an independent content extraction library built on top of it.

## Quick Start

```go
package main

import (
    "fmt"

    "github.com/cybergodev/html"
)

func main() {
    // Create a processor with default configuration
    processor, err := html.New()
    if err != nil {
        panic(err)
    }
    defer processor.Close()

    // Extract content from HTML bytes
    htmlBytes := []byte(`<article>
        <h1>Article Title</h1>
        <p>Main content here.</p>
        <img src="image.jpg" alt="Test">
    </article>`)

    result, err := processor.Extract(htmlBytes)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Title: %s\n", result.Title)
    fmt.Printf("Text: %s\n", result.Text)
    fmt.Printf("Word Count: %d\n", result.WordCount)
    fmt.Printf("Images: %d\n", len(result.Images))
}
```

## Configuration

### Config Struct

All configuration is done through the `Config` struct. Start from `DefaultConfig()` and modify as needed:

```go
cfg := html.DefaultConfig()
cfg.MaxInputSize = 10 * 1024 * 1024
cfg.InlineImageFormat = "markdown"
processor, err := html.New(cfg)
if err != nil {
    log.Fatal(err)
}
defer processor.Close()
```

### Configuration Options

```go
cfg := html.Config{
    // Resource Management
    MaxInputSize:      50 * 1024 * 1024, // 50MB max input (default)
    MaxCacheEntries:   2000,             // Cache up to 2000 results (default)
    CacheTTL:          time.Hour,        // 1 hour TTL (default)
    CacheCleanup:      5 * time.Minute,  // Background cleanup interval (default)
    WorkerPoolSize:    4,                // 4 parallel workers (default)
    ProcessingTimeout: 30 * time.Second, // 30s timeout (default)

    // Security
    EnableSanitization: true,   // Sanitize HTML (default: true)
    MaxDepth:           500,    // Max nesting depth (default)
    AllowedBaseDir:     "",     // Restrict file ops to this dir; empty = no restriction (default). Use with ExtractFromFile on untrusted paths
    Audit:              html.DefaultAuditConfig(), // Security audit logging (default: disabled; see Audit section in docs/SECURITY.md)

    // Content Extraction
    ExtractArticle: true, // Enable article detection (default: true)
    PreserveImages: true, // Extract image metadata (default: true)
    PreserveLinks:  true, // Extract link metadata (default: true)
    PreserveVideos: true, // Extract video metadata (default: true)
    PreserveAudios: true, // Extract audio metadata (default: true)

    // Output Formats
    InlineImageFormat: "none",      // "none", "markdown", "html", "placeholder" (default: "none")
    InlineLinkFormat:  "none",      // "none", "markdown", "html" (default: "none")
    TableFormat:       "markdown",  // "markdown", "html" (default: "markdown")
    Encoding:          "",          // Auto-detect if empty (default)

    // Link Extraction
    ResolveRelativeURLs:  true,  // Resolve relative URLs (default: true)
    BaseURL:              "",    // Base URL for resolution
    IncludeImages:        true,  // Include image URLs in links (default: true)
    IncludeVideos:        true,  // Include video URLs in links (default: true)
    IncludeAudios:        true,  // Include audio URLs in links (default: true)
    IncludeCSS:           true,  // Include CSS URLs in links (default: true)
    IncludeJS:            true,  // Include JS URLs in links (default: true)
    IncludeContentLinks:  true,  // Include <a href> links (default: true)
    IncludeExternalLinks: true,  // Include external links (default: true)
    IncludeIcons:         true,  // Include favicon URLs (default: true)
}
processor, err := html.New(cfg)
```

### Preset Configurations

```go
// Default configuration
processor, err := html.New()

// High security (reduced limits, strict settings)
processor, err := html.New(html.HighSecurityConfig())

// Plain text only (no media metadata)
processor, err := html.New(html.TextOnlyConfig())

// Markdown output format
processor, err := html.New(html.MarkdownConfig())

// Custom configuration
cfg := html.DefaultConfig()
cfg.MaxInputSize = 10 * 1024 * 1024
processor, err := html.New(cfg)
```

## API Reference

### Processor Methods

The `Processor` is the main processing engine. Create one with `html.New(cfg ...Config)`.

#### Content Extraction

```go
// From bytes (primary method)
result, err := processor.Extract(htmlBytes []byte) (*Result, error)

// From file
result, err := processor.ExtractFromFile(filePath string) (*Result, error)

// Plain text only
text, err := processor.ExtractText(htmlBytes []byte) (string, error)
text, err := processor.ExtractTextFromFile(filePath string) (string, error)
```

#### Context-Aware Extraction

All extraction methods have context-aware variants that support cooperative cancellation:

```go
// From bytes with context
result, err := processor.ExtractWithContext(ctx context.Context, htmlBytes []byte) (*Result, error)

// From file with context
result, err := processor.ExtractFromFileWithContext(ctx context.Context, filePath string) (*Result, error)

// Plain text with context
text, err := processor.ExtractTextWithContext(ctx context.Context, htmlBytes []byte) (string, error)
text, err := processor.ExtractTextFromFileWithContext(ctx context.Context, filePath string) (string, error)
```

#### Output Format Methods

```go
// Markdown output
markdown, err := processor.ExtractToMarkdown(htmlBytes []byte) (string, error)
markdown, err := processor.ExtractToMarkdownFromFile(filePath string) (string, error)

// JSON output
jsonBytes, err := processor.ExtractToJSON(htmlBytes []byte) ([]byte, error)
jsonBytes, err := processor.ExtractToJSONFromFile(filePath string) ([]byte, error)

// Context-aware variants
markdown, err := processor.ExtractToMarkdownWithContext(ctx, htmlBytes) (string, error)
markdown, err := processor.ExtractToMarkdownFromFileWithContext(ctx, filePath) (string, error)
jsonBytes, err := processor.ExtractToJSONWithContext(ctx, htmlBytes) ([]byte, error)
jsonBytes, err := processor.ExtractToJSONFromFileWithContext(ctx, filePath) ([]byte, error)
```

#### Batch Processing

```go
// Process multiple byte slices concurrently
br := processor.ExtractBatch(htmlContents [][]byte) *BatchResult

// Process multiple files concurrently
br := processor.ExtractBatchFiles(filePaths []string) *BatchResult

// Context-aware variants
br := processor.ExtractBatchWithContext(ctx, htmlContents [][]byte) *BatchResult
br := processor.ExtractBatchFilesWithContext(ctx, filePaths []string) *BatchResult
```

#### Link Extraction

```go
// Extract all links as LinkResource
links, err := processor.ExtractAllLinks(htmlBytes []byte) ([]LinkResource, error)
links, err := processor.ExtractAllLinksFromFile(filePath string) ([]LinkResource, error)

// Context-aware variants
links, err := processor.ExtractAllLinksWithContext(ctx, htmlBytes) ([]LinkResource, error)
links, err := processor.ExtractAllLinksFromFileWithContext(ctx, filePath) ([]LinkResource, error)
```

#### Statistics & Cache

```go
// Get processing statistics
stats := processor.GetStatistics()
// stats.TotalProcessed, stats.CacheHits, stats.CacheMisses,
// stats.ErrorCount, stats.AverageProcessTime

// Clear cache
processor.ClearCache()

// Reset statistics
processor.ResetStatistics()

// Get audit log entries
entries := processor.GetAuditLog()
processor.ClearAuditLog()

// Always close when done
defer processor.Close()
```

### Package-Level Convenience Functions

All `Processor` methods have package-level convenience functions that use a pooled processor. They accept an optional `cfg ...Config` parameter:

```go
// Content extraction
result, err := html.Extract(htmlBytes, cfg ...Config) (*Result, error)
result, err := html.ExtractFromFile(filePath, cfg ...Config) (*Result, error)
text, err := html.ExtractText(htmlBytes, cfg ...Config) (string, error)

// Context-aware
result, err := html.ExtractWithContext(ctx, htmlBytes, cfg ...Config) (*Result, error)

// Output formats
markdown, err := html.ExtractToMarkdown(htmlBytes, cfg ...Config) (string, error)
jsonBytes, err := html.ExtractToJSON(htmlBytes, cfg ...Config) ([]byte, error)

// Batch
br := html.ExtractBatch(htmlContents, cfg ...Config) *BatchResult
br := html.ExtractBatchFiles(filePaths, cfg ...Config) *BatchResult

// Links
links, err := html.ExtractAllLinks(htmlBytes, cfg ...Config) ([]LinkResource, error)
```

### BatchResult

Batch methods return a `*BatchResult` (not individual results):

```go
type BatchResult struct {
    Results    []*Result // Extraction results (nil for failed items)
    Errors     []error   // Per-item errors
    Success    int       // Count of successful extractions
    Failed     int       // Count of failed extractions
    Cancelled  int       // Count of items skipped due to context cancellation
}
```

Usage:

```go
br := processor.ExtractBatch(htmlContents)
if br.Failed > 0 {
    for i, err := range br.Errors {
        if err != nil {
            log.Printf("Item %d failed: %v", i, err)
        }
    }
}
for i, result := range br.Results {
    if result != nil {
        fmt.Printf("Item %d: %s\n", i, result.Title)
    }
}
```

## Result Types

### Result

```go
type Result struct {
    Text           string        // Extracted clean text
    Title          string        // Page/article title
    Images         []ImageInfo   // Image metadata
    Links          []LinkInfo    // Link metadata
    Videos         []VideoInfo   // Video metadata
    Audios         []AudioInfo   // Audio metadata
    ProcessingTime time.Duration // Processing duration
    WordCount      int           // Word count
    ReadingTime    time.Duration // Estimated reading time (200 WPM)
}
```

### ImageInfo

```go
type ImageInfo struct {
    URL          string // Image URL
    Alt          string // Alt text
    Title        string // Title attribute
    Width        string // Width attribute
    Height       string // Height attribute
    IsDecorative bool   // True if alt text is empty
    Position     int    // Position in text (for inline formatting)
}
```

### LinkInfo

```go
type LinkInfo struct {
    URL        string // Link URL
    Text       string // Anchor text
    Title      string // Title attribute
    IsExternal bool   // True if external domain
    IsNoFollow bool   // True if rel="nofollow"
    Position   int    // Position in text (for inline formatting)
}
```

### VideoInfo

```go
type VideoInfo struct {
    URL      string // Video URL (native, YouTube, Vimeo, direct)
    Type     string // MIME type or "embed"
    Poster   string // Poster image URL
    Width    string // Width attribute
    Height   string // Height attribute
    Duration string // Duration attribute
}
```

### AudioInfo

```go
type AudioInfo struct {
    URL      string // Audio URL
    Type     string // MIME type
    Duration string // Duration attribute
}
```

### LinkResource

```go
type LinkResource struct {
    URL   string // Resource URL
    Title string // Display title
    Type  string // Resource type: "link", "image", "video", "audio", "css", "js", "icon", "media"
}
```

### Statistics

```go
type Statistics struct {
    TotalProcessed     int64
    CacheHits          int64
    CacheMisses        int64
    ErrorCount         int64
    AverageProcessTime time.Duration
}
```

## Interfaces

The library provides interfaces for mocking and testing:

```go
// Full interface (all extraction, batch, link, and output methods)
var e html.Extractor = processor

// Statistics and cache management
var sp html.StatsProvider = processor
```

`Extractor` includes all content extraction, text extraction, formatted output (Markdown/JSON), batch processing, and link extraction methods. `StatsProvider` covers statistics retrieval, cache clearing, and statistics reset.

## Error Handling

The library uses sentinel errors compatible with `errors.Is()`:

```go
result, err := processor.Extract(htmlBytes)
if err != nil {
    if errors.Is(err, html.ErrInputTooLarge) {
        // Input exceeds MaxInputSize
    } else if errors.Is(err, html.ErrMaxDepthExceeded) {
        // HTML nesting exceeds MaxDepth
    } else if errors.Is(err, html.ErrProcessingTimeout) {
        // Processing exceeded ProcessingTimeout
    } else if errors.Is(err, html.ErrInvalidHTML) {
        // HTML parsing failed
    } else if errors.Is(err, html.ErrProcessorClosed) {
        // Processor was closed
    } else if errors.Is(err, html.ErrFileNotFound) {
        // File not found
    } else if errors.Is(err, html.ErrInvalidConfig) {
        // Configuration validation failed
    } else if errors.Is(err, html.ErrInvalidFilePath) {
        // File path validation failed
    } else if errors.Is(err, html.ErrInternalPanic) {
        // Internal panic recovered (indicates a bug)
    } else if errors.Is(err, html.ErrMultipleConfigs) {
        // More than one Config provided to package-level function
    }
}
```

## Custom Scorer

You can provide a custom content scorer to control article extraction:

```go
type MyScorer struct{}

func (s MyScorer) Score(node html.ContentNode) int {
    // Return a relevance score for the node
    return 0
}

func (s MyScorer) ShouldRemove(node html.ContentNode) bool {
    // Return true to remove the node from content
    return false
}

cfg := html.DefaultConfig()
cfg.Scorer = MyScorer{}
processor, err := html.New(cfg)
```

## Best Practices

### 1. Reuse Processor Instances

```go
// Good: Create once, reuse many times
processor, err := html.New()
if err != nil {
    log.Fatal(err)
}
defer processor.Close()

for _, content := range htmlDocs {
    result, err := processor.Extract(content)
    // ... handle result
}

// Bad: Creating new processor per request
for _, content := range htmlDocs {
    processor, _ := html.New()
    result, _ := processor.Extract(content)
    processor.Close()
}
```

### 2. Always Close Processor

```go
processor, err := html.New()
if err != nil {
    log.Fatal(err)
}
defer processor.Close() // Releases cache and background goroutines
```

### 3. Use Batch Processing for Multiple Documents

```go
// Good: Parallel processing with worker pool
br := processor.ExtractBatch(htmlDocs)

// Bad: Sequential processing
for _, content := range htmlDocs {
    result, _ := processor.Extract(content)
}
```

### 4. Configure Limits Appropriately

```go
// Good: Set limits based on your use case
cfg := html.DefaultConfig()
cfg.MaxInputSize = 10 * 1024 * 1024 // 10MB for blog posts
cfg.MaxCacheEntries = 500            // Cache 500 recent pages
cfg.WorkerPoolSize = 8               // 8 workers for batch processing

// Bad: Using unlimited or excessive values
cfg.MaxInputSize = 1024 * 1024 * 1024 // 1GB - too large
cfg.MaxCacheEntries = 1000000          // 1M entries - excessive memory
```

### 5. Use Package-Level Functions for One-Off Extractions

```go
// Good: Package-level function uses pooled processor
result, err := html.Extract(htmlBytes)

// Good: With custom config
cfg := html.TextOnlyConfig()
text, err := html.ExtractText(htmlBytes, cfg)
```

## Performance

### Single Extraction

- First extraction parses and analyzes content (~1-5ms for typical pages)
- Cached extractions retrieved near-instantly from cache (~0.1ms)
- Memory efficient with `sync.Pool` for builders and buffers

### Batch Processing

- Parallel workers maximize throughput (configurable `WorkerPoolSize`)
- Worker pool semaphore prevents resource exhaustion

### Caching

Content-addressable caching using xxHash-style hashing. Cache entries expire based on TTL and are evicted using LRU when full.

```go
processor, err := html.New()
if err != nil {
    log.Fatal(err)
}
defer processor.Close()

// First call: parse + analyze
result1, _ := processor.Extract(htmlBytes)

// Second call: cache hit
result2, _ := processor.Extract(htmlBytes)

stats := processor.GetStatistics()
fmt.Printf("Cache hit rate: %.1f%%\n",
    float64(stats.CacheHits)/float64(stats.TotalProcessed)*100)
```

## Dependencies

- `golang.org/x/net/html` - HTML5 parser (official Go supplementary library)
- `golang.org/x/text` - Character encoding detection and conversion

## Examples

See the `examples/` directory for complete examples:

- `01_quick_start.go` - Basic usage
- `02_content_extraction.go` - Content extraction features
- `03_links_media.go` - Link and media extraction
- `04_performance.go` - Performance optimization
- `05_http_integration.go` - HTTP server integration
- `06_advanced_usage.go` - Advanced features
- `07_error_handling.go` - Error handling patterns
- `08_real_world.go` - Real-world scenarios

## FAQ

### Q: Can I use this as a drop-in replacement for golang.org/x/net/html?
**A:** No. This library uses `golang.org/x/net/html` internally but does not re-export its types or functions. It is an independent content extraction library.

### Q: What input types do extraction methods accept?
**A:** `Processor.Extract()` and `ExtractBatch()` accept `[]byte` (not `string`). This allows for efficient encoding detection directly from raw bytes. Use `ExtractFromFile()` for file paths.

### Q: Do I need to pass config to each extraction call?
**A:** No. Configuration is set once at `New(cfg ...Config)` and applies to all subsequent calls on that processor. Package-level convenience functions accept optional `cfg ...Config` for one-off customization.

### Q: Is it thread-safe?
**A:** Yes. A single `Processor` can be safely used by multiple goroutines concurrently.

### Q: What about dependencies?
**A:** Two dependencies: `golang.org/x/net/html` (HTML parsing) and `golang.org/x/text` (encoding detection).

### Q: How does caching work?
**A:** Content-addressable caching using xxHash-style hashing. Cache entries expire based on TTL (default: 1 hour) and are evicted using LRU when the cache is full (default: 2000 entries).

## License

MIT License - See [LICENSE](../LICENSE) file for details.
