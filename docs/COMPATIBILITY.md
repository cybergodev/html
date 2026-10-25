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

#### Reader Mode

Reader mode extracts a full-page structured reading view: every region
(header, navigation, main, aside, footer) is kept as a labeled section with
its text and inline hyperlinks, instead of selecting one article and
discarding the rest. Its primary use is feeding clean pages to LLMs.

```go
// Structured sections (from bytes / file, with optional context)
page, err := processor.ReadPage(htmlBytes []byte) (*ReadResult, error)
page, err := processor.ReadPageFromFile(filePath string) (*ReadResult, error)
page, err := processor.ReadPageWithContext(ctx context.Context, htmlBytes []byte) (*ReadResult, error)
page, err := processor.ReadPageFromFileWithContext(ctx context.Context, filePath string) (*ReadResult, error)

// One-document Markdown form, WITH inline links ([text](URL), ![alt](URL))
markdown, err := processor.ReadPageToMarkdown(htmlBytes []byte) (string, error)
markdown, err := processor.ReadPageFromFileToMarkdown(filePath string) (string, error)
markdown, err := processor.ReadPageToMarkdownWithContext(ctx, htmlBytes) (string, error)
markdown, err := processor.ReadPageFromFileToMarkdownWithContext(ctx, filePath) (string, error)

// One-document Markdown form, WITHOUT inline links or images (labels as words)
plain, err := processor.ReadPageToMarkdownText(htmlBytes []byte) (string, error)
plain, err := processor.ReadPageFromFileToMarkdownText(filePath string) (string, error)
plain, err := processor.ReadPageToMarkdownTextWithContext(ctx, htmlBytes) (string, error)
plain, err := processor.ReadPageFromFileToMarkdownTextWithContext(ctx, filePath) (string, error)

// Flatten a structured result either way
linked := page.Render()      // with links
plainDoc := page.RenderText() // without links
```

Hyperlinks are always preserved in section `Text`; `Config.InlineLinkFormat`
selects the style (`"markdown"` → `[text](URL)`, the default; `"html"` →
`<a href="...">text</a>`), and `"none"` — the DefaultConfig value, right for
the Extract family — is treated as `"markdown"` in reader mode, so links
never silently vanish from the default output. Each section also carries
`PlainText` (the link-free variant), and the `*Text` entry points and
`RenderText` expose it — use those when the consumer wants prose only.
Inside link labels, images are label-aware: decorative images (empty alt,
menu icons) are dropped, an images-only label (a logo link) renders the alt
texts as the label, and a mixed label nests the image markdown —
`[text ![alt](img)](url)`. (With `InlineImageFormat "none"` images emit no
placeholders, so images-only labels keep the `Link N` fallback.)
Relative link and image URLs are resolved against `Config.BaseURL` or a base
detected from the document. Section cleaning is kind-aware: main regions get
article-grade cleaning (ads, hidden nodes, nested navigation removed) while
chrome regions keep their menus and are stripped of hidden nodes only. A
later chrome section whose whitespace-normalized text exactly duplicates an
earlier section of the same kind (the same menu visibly rendered twice) is
dropped; main sections are never deduplicated.
ReadPage does not use the extraction cache (its result type differs from
Result's).

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

All `Processor` methods have package-level convenience functions. Without a Config they reuse a pooled default-config processor (caching disabled — pooled processors are reset between uses); with a Config, a fresh one-shot processor is created and closed for that call. They accept an optional `cfg ...Config` parameter:

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

// Reader mode
page, err := html.ReadPage(htmlBytes, cfg ...Config) (*ReadResult, error)
markdown, err := html.ReadPageToMarkdown(htmlBytes, cfg ...Config) (string, error)        // with links
plain, err := html.ReadPageToMarkdownText(htmlBytes, cfg ...Config) (string, error)       // without links
markdown, err := html.ReadPageFromFileToMarkdown(filePath, cfg ...Config) (string, error)
plain, err := html.ReadPageFromFileToMarkdownText(filePath, cfg ...Config) (string, error)
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
    Text             string        // Extracted clean text
    Title            string        // Page/article title (JSON-LD headline > og:title > <title>)
    Images           []ImageInfo   // Image metadata
    Links            []LinkInfo    // Link metadata
    Videos           []VideoInfo   // Video metadata
    Audios           []AudioInfo   // Audio metadata
    ProcessingTime   time.Duration // Processing duration
    WordCount        int           // Word count (each CJK ideograph/kana/Hangul counts as one word)
    ReadingTime      time.Duration // Estimated reading time (200 words/min; 350 chars/min for CJK)
    Author           string        // JSON-LD author or <meta name="author"> (ExtractMetadata)
    PublishedTime    time.Time     // article:published_time or JSON-LD datePublished (zero = unknown)
    Description      string        // og:description > JSON-LD description > <meta name="description">
    SiteName         string        // og:site_name or JSON-LD publisher.name
    CanonicalURL     string        // <link rel="canonical"> or og:url
    Confidence       float64       // Extraction-quality signal in [0, 1] (1.0 = clean article)
    ExtractionMethod string        // "article" | "listing" (card feed) | "body" (fallback)
}
```

The metadata fields (`Author` … `CanonicalURL`) and the metadata-based title priority are
populated when `Config.ExtractMetadata` is true (the default); with it false the fields
stay empty and `Title` falls back to the `<title>` tag. See
`examples/12_metadata_quality/` for the priority ladders and quality signals.

### ReadResult

Returned by the reader-mode family (`ReadPage*`). Unlike `Result`, nothing is
discarded by kind — the page's regions are labeled, and callers decide which
sections to keep. `Render()` flattens the sections into one document WITH
links (title as a top-level heading, then each section under a `# <Kind>`
heading); `RenderText()` flattens them without links.

```go
type SectionKind string // "header", "navigation", "main", "aside", "footer"

const (
    SectionHeader     SectionKind = "header"
    SectionNavigation SectionKind = "navigation"
    SectionMain       SectionKind = "main"
    SectionAside      SectionKind = "aside"
    SectionFooter     SectionKind = "footer"
)

type PageSection struct {
    Kind      SectionKind // Region label
    Text      string      // Region text WITH inline links (style per Config.InlineLinkFormat)
    PlainText string      // Same words WITHOUT links/images (backs RenderText and the *Text entry points)
    Links     []LinkInfo  // Region links in document order (positions local to the source region)
    Images    []ImageInfo // Region images in document order, same locality rule (when PreserveImages)
    WordCount int         // Words in PlainText (CJK-aware)
}

type ReadResult struct {
    Title          string        // Same priority ladder as Result.Title
    Author         string
    PublishedTime  time.Time     // Zero time = unknown
    Description    string
    SiteName       string
    CanonicalURL   string
    Sections       []PageSection // Document order; adjacent same-kind merged; empty regions dropped
    ProcessingTime time.Duration
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
    IsExternal bool   // True if the URL is absolute (http/https or protocol-relative), regardless of domain — same-domain absolute links also report true; relative URLs report false
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

Two method families are `Processor` methods outside these interfaces: the reader-mode
family (`ReadPage*`) and the audit accessors (`GetAuditLog`/`ClearAuditLog`). Code that
needs to mock those should define its own narrow interface over the concrete
`*Processor` usage it relies on.

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
    } else if errors.Is(err, html.ErrBatchTooLarge) {
        // Batch exceeds 10,000 items
    } else if errors.Is(err, html.ErrTooManyConcurrentOps) {
        // Too many concurrent one-shot (package-level) operations
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
    // Part of the Scorer interface; see the note below
    return false
}

cfg := html.DefaultConfig()
cfg.Scorer = MyScorer{}
processor, err := html.New(cfg)
```

> **Note:** Two methods are consulted on custom scorers. `Score` drives article-node
> candidate selection; a scorer that additionally implements the optional `ArticleFinder`
> interface (`FindArticleNode(root ContentNode) ContentNode`) takes over article-node
> selection in a single pass — a non-nil return skips the `Score` enumeration entirely,
> and a nil return means "no article" (the whole `<body>` is extracted).
> `ShouldRemove` is part of the `Scorer` interface but is not invoked
> on custom scorers by the extraction pipeline; node removal always follows the built-in
> scorer's rules regardless of what a custom scorer returns here.

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

// Bad: Values that exceed validation limits — New() rejects these with
// ErrInvalidConfig (MaxInputSize is hard-capped at 50MB, MaxCacheEntries at 100,000)
cfg.MaxInputSize = 1024 * 1024 * 1024 // 1GB - exceeds the 50MB hard cap
cfg.MaxCacheEntries = 1000000          // 1M entries - exceeds the 100,000 hard cap
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

Content-addressable caching keyed by a 128-bit `hash/maphash` digest (AES-NI accelerated, seeded randomly per process) — the 64-bit maphash sum plus a splitmix64-derived second half. Cache entries expire based on TTL and are evicted using LRU when full.

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

### Unicode Normalization Data by Go Version

`golang.org/x/text` v0.43.0 ships its `unicode/norm` tables in two
version-gated files: `tables15.0.0.go` (`//go:build !go1.27`) and
`tables17.0.0.go` (`//go:build go1.27`). As a result, the NFC normalization
the sanitizer applies before its URL-scheme checks uses Unicode 15.0 data
when built with Go 1.26 and Unicode 17.0 data when built with Go 1.27 or
later.

This is not a security gap — both files are complete, self-consistent
normalization tables, and NFC has been essentially stable across these
Unicode releases. The only observable difference is how characters newly
introduced in Unicode 16/17 normalize, which is irrelevant to the
`javascript:`/`vbscript:`/`file:` scheme policy. To keep normalization
byte-identical across a fleet, build the library with a single pinned Go
version.

## Examples

See the `examples/` directory for complete examples (each is a self-contained `main` package in its own subdirectory):

- `01_quick_start/` - Basic usage
- `02_content_extraction/` - Content extraction features
- `03_links_media/` - Link and media extraction
- `04_performance/` - Performance optimization
- `05_http_integration/` - HTTP integration patterns (fetching via a shared client)
- `06_advanced_usage/` - Advanced features
- `07_error_handling/` - Error handling patterns
- `08_real_world/` - Real-world scenarios
- `09_context_cancellation/` - Context cancellation and timeouts
- `10_secure_file_processing/` - AllowedBaseDir sandbox and secure file reads
- `11_encoding/` - Character encoding detection and override
- `12_metadata_quality/` - Structured metadata and extraction confidence
- `13_page_reader/` - Reader mode: full-page structured reading view for LLMs

Run any one directly, e.g. `go run ./examples/01_quick_start/`, or build all at once with `go build ./examples/...`.

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
**A:** Content-addressable caching keyed by a 128-bit `hash/maphash` digest (AES-NI accelerated, seeded randomly per process). Cache entries expire based on TTL (default: 1 hour) and are evicted using LRU when the cache is full (default: 2000 entries).

## License

MIT License - See [LICENSE](../LICENSE) file for details.
