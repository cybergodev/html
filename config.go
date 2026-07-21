package html

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Default configuration values.
const (
	// DefaultMaxInputSize is the default maximum accepted HTML input size (50 MB).
	DefaultMaxInputSize = 50 * 1024 * 1024
	// DefaultMaxCacheEntries is the default maximum number of cached extraction results.
	DefaultMaxCacheEntries = 2000
	// DefaultWorkerPoolSize is the default number of workers used for batch extraction.
	DefaultWorkerPoolSize = 4
	// DefaultCacheTTL is the default time-to-live for cached extraction results.
	DefaultCacheTTL = time.Hour
	// DefaultCacheCleanup is the default interval between background sweeps of expired cache entries.
	DefaultCacheCleanup = 5 * time.Minute
	// DefaultMaxDepth is the default maximum HTML nesting depth, guarding against stack overflow.
	DefaultMaxDepth = 500
	// DefaultProcessingTimeout is the default per-document processing timeout.
	DefaultProcessingTimeout = 30 * time.Second
)

// Configuration limits - reference Default* constants for consistency
const (
	// maxConfigInputSize limits the maximum allowed MaxInputSize configuration.
	// This matches DefaultMaxInputSize (50MB) to prevent memory exhaustion.
	maxConfigInputSize = DefaultMaxInputSize

	// maxConfigWorkerSize limits the maximum worker pool size.
	// Value 256 prevents excessive goroutine creation while allowing
	// high-throughput batch processing on powerful machines.
	maxConfigWorkerSize = 256

	// maxConfigDepth limits the maximum HTML nesting depth.
	// This matches DefaultMaxDepth (500) to prevent stack overflow.
	maxConfigDepth = DefaultMaxDepth

	// maxConfigCacheEntries limits the maximum number of cache entries.
	// Value 100,000 entries ≈ 100MB assuming 1KB average entry size.
	maxConfigCacheEntries = 100000

	// Processing limits
	// maxHTMLForRegex limits HTML size for regex-based media URL detection.
	// Above 1MB, regex operations become slow and could cause ReDoS.
	maxHTMLForRegex = 1000000

	// maxRegexMatches limits the number of regex matches to prevent
	// excessive memory allocation on HTML with many media URLs.
	maxRegexMatches = 1000

	// maxCacheKeySize limits the content size used for cache key generation.
	// For content larger than 64KB, multi-point sampling is used instead.
	maxCacheKeySize = 64 * 1024

	// cacheKeySample is the sample size for cache key generation on large content.
	cacheKeySample = 4096

	// Buffer size estimates for pre-allocation
	initialTextSize = 4096 // Initial capacity for text builder
	initialSliceCap = 16   // Initial capacity for result slices
	// linksInitialCap is the initial capacity for the links result slice. Links are
	// the most numerous per-element extraction target on typical pages (nav bars,
	// in-content links, footers), and a link-dense page exceeds initialSliceCap
	// (16) many times over — each overflow reallocates and copies the whole slice.
	// 128 covers realistic pages in a single backing array with no growth while
	// staying a modest, strictly-bounded over-allocation for small pages.
	linksInitialCap   = 128
	initialMapCap     = 8  // Initial capacity for result maps
	imageHTMLBufExtra = 64 // Extra buffer for HTML image tag generation
	extractTagCap     = 16 // Initial capacity for tag attribute extraction
	linkMapCap        = 64 // Initial capacity for link deduplication map

	// Processing thresholds
	wordsPerMinute = 200 // Average reading speed for reading time estimation
)

// Pre-compiled regex patterns for media URL detection.
var (
	videoRegex = regexp.MustCompile(`(?i)https?://[^\s<>"',;)}\]]{1,500}\.(?:mp4|webm|ogg|mov|avi|wmv|flv|mkv|m4v|3gp)`)
	audioRegex = regexp.MustCompile(`(?i)https?://[^\s<>"',;)}\]]{1,500}\.(?:mp3|wav|ogg|m4a|aac|flac|wma|opus|oga)`)
)

// ============================================================================
// Flat Configuration Structure
// ============================================================================

// Config is the unified configuration for the HTML processor.
// All configuration options are flat for ease of use.
// Zero-value is not usable; start from DefaultConfig().
//
// Example:
//
//	cfg := html.DefaultConfig()
//	cfg.MaxInputSize = 10 * 1024 * 1024
//	cfg.InlineImageFormat = "markdown"
//	cfg.IncludeJS = false
//	processor, err := html.New(cfg)
type Config struct {
	// === Resource Management ===
	MaxInputSize      int           // Maximum HTML input size in bytes. Default: 50MB. Must be positive and <= 50MB.
	MaxCacheEntries   int           // Maximum number of cache entries. Set to 0 to disable caching. Default: 2000.
	CacheTTL          time.Duration // Time-to-live for cache entries. Default: 1 hour.
	CacheCleanup      time.Duration // Interval for background cleanup of expired cache entries. Set to 0 to disable. Default: 5 minutes.
	WorkerPoolSize    int           // Number of concurrent workers for batch processing. Default: 4. Must be positive and <= 256.
	ProcessingTimeout time.Duration // Maximum time allowed for processing a single document. Default: 30 seconds. Set to 0 for no timeout.

	// === Security ===
	EnableSanitization bool        // Controls whether HTML sanitization is applied. Default: true. Should only be disabled for trusted input.
	MaxDepth           int         // Maximum allowed nesting depth of HTML elements. Prevents stack overflow. Default: 500.
	AllowedBaseDir     string      // Restricts file operations to this directory. Empty (default) means no restriction. Use when accepting file paths from untrusted input.
	Audit              AuditConfig // Security audit logging configuration.

	// === Content Extraction ===
	ExtractArticle bool // Enables article extraction mode. When true, identifies and extracts main content. Default: true.
	PreserveImages bool // Controls whether images are preserved in output. Default: true.
	PreserveLinks  bool // Controls whether links are preserved in output. Default: true.
	PreserveVideos bool // Controls whether video elements are extracted. Default: true.
	PreserveAudios bool // Controls whether audio elements are extracted. Default: true.

	// === Output Formats ===
	InlineImageFormat string // How images are formatted in text output. Options: "none", "markdown", "html", "placeholder". Default: "none".
	InlineLinkFormat  string // How links are formatted in text output. Options: "none", "markdown", "html". Default: "none".
	TableFormat       string // How tables are formatted in output. Options: "markdown", "html". Default: "markdown".
	Encoding          string // Character encoding of input HTML. Leave empty for auto-detection. Common: "utf-8", "windows-1252", "gbk".

	// === Link Extraction ===
	ResolveRelativeURLs  bool   // Controls whether relative URLs are resolved to absolute URLs. Requires BaseURL. Default: true.
	BaseURL              string // Base URL for resolving relative URLs. Example: "https://example.com"
	IncludeImages        bool   // Controls whether image URLs are included in link extraction. Default: true.
	IncludeVideos        bool   // Controls whether video URLs are included in link extraction. Default: true.
	IncludeAudios        bool   // Controls whether audio URLs are included in link extraction. Default: true.
	IncludeCSS           bool   // Controls whether CSS stylesheet URLs are included in link extraction. Default: true.
	IncludeJS            bool   // Controls whether JavaScript URLs are included in link extraction. Default: true.
	IncludeContentLinks  bool   // Controls whether content links (a[href]) are included. Default: true.
	IncludeExternalLinks bool   // Controls whether external links are included. Default: true.
	IncludeIcons         bool   // Controls whether favicon/icon URLs are included in link extraction. Default: true.

	// === Extension ===
	Scorer Scorer `json:"-"` // Optional custom scorer for content extraction. If nil, the default scorer is used.
}

// DefaultConfig returns a Config with all default values.
func DefaultConfig() Config {
	return Config{
		// Resource Management
		MaxInputSize:      DefaultMaxInputSize,
		MaxCacheEntries:   DefaultMaxCacheEntries,
		CacheTTL:          DefaultCacheTTL,
		CacheCleanup:      DefaultCacheCleanup,
		WorkerPoolSize:    DefaultWorkerPoolSize,
		ProcessingTimeout: DefaultProcessingTimeout,

		// Security
		EnableSanitization: true,
		MaxDepth:           DefaultMaxDepth,
		Audit:              DefaultAuditConfig(),

		// Content Extraction
		ExtractArticle: true,
		PreserveImages: true,
		PreserveLinks:  true,
		PreserveVideos: true,
		PreserveAudios: true,

		// Output Formats
		InlineImageFormat: "none",
		InlineLinkFormat:  "none",
		TableFormat:       "markdown",

		// Link Extraction
		ResolveRelativeURLs:  true,
		IncludeImages:        true,
		IncludeVideos:        true,
		IncludeAudios:        true,
		IncludeCSS:           true,
		IncludeJS:            true,
		IncludeContentLinks:  true,
		IncludeExternalLinks: true,
		IncludeIcons:         true,
	}
}

// Validate validates the configuration and returns an error if invalid.
func (c Config) Validate() error {
	switch {
	case c.MaxInputSize <= 0:
		return newConfigError("MaxInputSize", c.MaxInputSize, "must be positive")
	case c.MaxInputSize > maxConfigInputSize:
		return newConfigError("MaxInputSize", c.MaxInputSize, fmt.Sprintf("exceeds maximum %d", maxConfigInputSize))
	case c.MaxCacheEntries < 0:
		return newConfigError("MaxCacheEntries", c.MaxCacheEntries, "cannot be negative")
	case c.MaxCacheEntries > maxConfigCacheEntries:
		return newConfigError("MaxCacheEntries", c.MaxCacheEntries, fmt.Sprintf("exceeds maximum %d", maxConfigCacheEntries))
	case c.CacheTTL < 0:
		return newConfigError("CacheTTL", c.CacheTTL, "cannot be negative")
	case c.CacheCleanup < 0:
		return newConfigError("CacheCleanup", c.CacheCleanup, "cannot be negative")
	case c.WorkerPoolSize <= 0:
		return newConfigError("WorkerPoolSize", c.WorkerPoolSize, "must be positive")
	case c.WorkerPoolSize > maxConfigWorkerSize:
		return newConfigError("WorkerPoolSize", c.WorkerPoolSize, fmt.Sprintf("exceeds maximum %d", maxConfigWorkerSize))
	case c.MaxDepth <= 0:
		return newConfigError("MaxDepth", c.MaxDepth, "must be positive")
	case c.MaxDepth > maxConfigDepth:
		return newConfigError("MaxDepth", c.MaxDepth, fmt.Sprintf("exceeds maximum %d", maxConfigDepth))
	case c.ProcessingTimeout < 0:
		return newConfigError("ProcessingTimeout", c.ProcessingTimeout, "cannot be negative")
	}

	// Validate format strings
	if err := validateFormat("InlineImageFormat", c.InlineImageFormat, []string{"none", "markdown", "html", "placeholder"}); err != nil {
		return err
	}
	if err := validateFormat("InlineLinkFormat", c.InlineLinkFormat, []string{"none", "markdown", "html"}); err != nil {
		return err
	}
	if err := validateFormat("TableFormat", c.TableFormat, []string{"markdown", "html"}); err != nil {
		return err
	}

	return nil
}

// validateFormat validates that a format string is one of the allowed values.
// Empty string is allowed (will use default).
func validateFormat(field, value string, allowed []string) error {
	if value == "" {
		return nil // Empty means use default
	}
	lowerValue := strings.ToLower(value)
	for _, a := range allowed {
		if lowerValue == a {
			return nil
		}
	}
	return newConfigError(field, value, fmt.Sprintf("valid values: %s", strings.Join(allowed, ", ")))
}

// HighSecurityConfig returns a configuration optimized for high-security environments.
// This includes reduced limits, shorter timeouts, and comprehensive audit logging.
func HighSecurityConfig() Config {
	cfg := DefaultConfig()

	// Resource Management - reduced limits
	cfg.MaxInputSize = 10 * 1024 * 1024 // 10MB
	cfg.MaxCacheEntries = 500
	cfg.CacheTTL = 30 * time.Minute
	cfg.CacheCleanup = time.Minute
	cfg.WorkerPoolSize = 2
	cfg.ProcessingTimeout = 10 * time.Second

	// Security - strict settings
	cfg.MaxDepth = 100
	cfg.Audit = HighSecurityAuditConfig()

	return cfg
}

// TextOnlyConfig returns a configuration for extracting plain text only.
// This disables all media preservation for maximum performance.
func TextOnlyConfig() Config {
	cfg := DefaultConfig()

	// Content Extraction - disable all media
	cfg.PreserveImages = false
	cfg.PreserveLinks = false
	cfg.PreserveVideos = false
	cfg.PreserveAudios = false

	return cfg
}

// MarkdownConfig returns a configuration optimized for Markdown output.
// This enables markdown format for inline images and links.
func MarkdownConfig() Config {
	cfg := DefaultConfig()

	// Output Formats - enable markdown
	cfg.InlineImageFormat = "markdown"
	cfg.InlineLinkFormat = "markdown"

	return cfg
}

// ============================================================================
// Result Types
// ============================================================================

// Result holds the extraction result.
type Result struct {
	// Text is the extracted plain-text content of the document.
	Text string `json:"text"`
	// Title is the document title from <title>, or the first <h1>/<h2> when absent.
	Title string `json:"title"`
	// Images lists extracted <img> elements in document order; empty when PreserveImages is false.
	Images []ImageInfo `json:"images,omitempty"`
	// Links lists extracted <a> elements in document order; empty when PreserveLinks is false.
	Links []LinkInfo `json:"links,omitempty"`
	// Videos lists extracted video sources; empty when PreserveVideos is false.
	Videos []VideoInfo `json:"videos,omitempty"`
	// Audios lists extracted audio sources; empty when PreserveAudios is false.
	Audios []AudioInfo `json:"audios,omitempty"`
	// ProcessingTime is the wall-clock time spent on this extraction. It is omitted from
	// JSON and serialized as processing_time_ms by MarshalJSON.
	ProcessingTime time.Duration `json:"-"`
	// WordCount is the number of whitespace-separated words in Text.
	WordCount int `json:"word_count"`
	// ReadingTime is the estimated reading time based on WordCount. It is omitted from
	// JSON and serialized as reading_time_ms by MarshalJSON.
	ReadingTime time.Duration `json:"-"`
}

// ImageInfo holds information about an extracted image.
type ImageInfo struct {
	// URL is the image source (the src attribute).
	URL string `json:"url"`
	// Alt is the alternative text (the alt attribute).
	Alt string `json:"alt"`
	// Title is the advisory title (the title attribute).
	Title string `json:"title"`
	// Width is the intrinsic width attribute, as an unparsed string.
	Width string `json:"width"`
	// Height is the intrinsic height attribute, as an unparsed string.
	Height string `json:"height"`
	// IsDecorative is true when Alt is empty, indicating a decorative image.
	IsDecorative bool `json:"is_decorative"`
	// Position is the 1-based ordinal of the image within the extracted content (0 if unplaced).
	Position int `json:"position"`
}

// LinkInfo holds information about an extracted link.
type LinkInfo struct {
	// URL is the link destination (the href attribute).
	URL string `json:"url"`
	// Text is the link's visible text content.
	Text string `json:"text"`
	// Title is the advisory title (the title attribute).
	Title string `json:"title"`
	// IsExternal is true when the URL targets a different host than BaseURL.
	IsExternal bool `json:"is_external"`
	// IsNoFollow is true when the link's rel attribute contains "nofollow".
	IsNoFollow bool `json:"is_nofollow"`
	// Position is the 1-based ordinal of the link within the extracted content (0 if unplaced).
	Position int `json:"position"`
}

// VideoInfo holds information about an extracted video.
type VideoInfo struct {
	// URL is the video source URL.
	URL string `json:"url"`
	// Type is the detected video type (the file container, or "embed" for iframe embeds).
	Type string `json:"type"`
	// Poster is the poster-frame URL for <video> elements.
	Poster string `json:"poster"`
	// Width is the width attribute, as an unparsed string.
	Width string `json:"width"`
	// Height is the height attribute, as an unparsed string.
	Height string `json:"height"`
	// Duration is the duration attribute, as an unparsed string.
	Duration string `json:"duration"`
}

// AudioInfo holds information about an extracted audio.
type AudioInfo struct {
	// URL is the audio source URL.
	URL string `json:"url"`
	// Type is the detected audio type (the file container).
	Type string `json:"type"`
	// Duration is the duration attribute, as an unparsed string.
	Duration string `json:"duration"`
}

// LinkResource represents a link resource extracted from HTML.
type LinkResource struct {
	// URL is the resource URL, resolved against BaseURL when ResolveRelativeURLs is enabled.
	URL string
	// Title is a human-readable label for the resource.
	Title string
	// Type categorizes the resource: "link", "image", "video", "audio", "css", "js", "icon", or "media".
	Type string
}

// Statistics holds processor statistics.
type Statistics struct {
	// TotalProcessed is the number of extractions that completed without error, including cache hits.
	TotalProcessed int64
	// CacheHits is the number of extractions served from the cache.
	CacheHits int64
	// CacheMisses is the number of extractions that missed the cache and required full processing.
	CacheMisses int64
	// ErrorCount is the number of extractions that returned an error.
	ErrorCount int64
	// AverageProcessTime is the mean wall-clock time per extraction.
	AverageProcessTime time.Duration
}
