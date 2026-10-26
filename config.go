package html

import (
	"fmt"
	"slices"
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
	// DefaultMaxAuditEntries is the default cap on audit entries retained in
	// memory by GetAuditLog (see AuditConfig.MaxEntries).
	DefaultMaxAuditEntries = 10000
	// DefaultMaxRawValueLength is the default cap on audit RawValue strings
	// (see AuditConfig.MaxRawValueLength).
	DefaultMaxRawValueLength = 200
	// DefaultMaxDepth is the default maximum HTML nesting depth, guarding against stack overflow.
	DefaultMaxDepth = 500
	// DefaultProcessingTimeout is the default per-document processing timeout.
	DefaultProcessingTimeout = 30 * time.Second
)

// Format values accepted by Config.InlineImageFormat, Config.InlineLinkFormat,
// and Config.TableFormat. Validation is case-insensitive and trims surrounding
// whitespace; the empty string falls back to the field default.
const (
	// FormatNone omits the element from text output. Not valid for TableFormat.
	// Reader mode (ReadPage*) always preserves links and treats FormatNone for
	// Config.InlineLinkFormat as FormatMarkdown.
	FormatNone = "none"
	// FormatMarkdown formats the element as Markdown. Valid for all three fields.
	FormatMarkdown = "markdown"
	// FormatHTML keeps the element as inline HTML. Valid for all three fields.
	FormatHTML = "html"
	// FormatPlaceholder emits a positional placeholder such as [IMAGE:1].
	// Valid for Config.InlineImageFormat only.
	FormatPlaceholder = "placeholder"
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

	// maxConfigAuditEntries limits the number of audit entries retained in
	// memory by GetAuditLog. Audit entries carry URLs and raw values (up to a
	// few hundred bytes each once truncated), so 100,000 bounds retention to
	// tens of MB while leaving ample forensic headroom over the 10,000 default.
	maxConfigAuditEntries = 100000

	// Processing limits
	// maxHTMLForRegex limits HTML size for regex-based media URL detection.
	// Above 1MB, regex operations become slow and could cause ReDoS.
	maxHTMLForRegex = 1000000

	// maxRegexMatches limits the number of regex matches to prevent
	// excessive memory allocation on HTML with many media URLs.
	maxRegexMatches = 1000

	// Buffer size estimates for pre-allocation
	initialSliceCap = 16 // Initial capacity for result slices
	initialMapCap   = 8  // Initial capacity for result maps
	extractTagCap   = 16 // Initial capacity for tag attribute extraction
	linkMapCap      = 64 // Initial capacity for link deduplication map

	// Processing thresholds
	wordsPerMinute    = 200 // Average reading speed for reading time estimation
	cjkCharsPerMinute = 350 // Reading speed for CJK text, in characters per minute

	// Extraction-confidence thresholds (see computeExtractionConfidence in
	// extract.go). Internal tuning constants, not exported.
	confidentTextChars  = 200  // extracted chars that make an article extraction "substantial"
	confidentBodyRatio  = 0.05 // minimum share of body text an article extraction should cover
	paywallScanChars    = 800  // leading chars scanned for consent/paywall boilerplate
	paywallTextMaxChars = 600  // extractions longer than this are not treated as paywalled
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
	MaxCacheEntries   int           // Maximum number of cache entries. Set to 0 to disable caching. Default: 2000. Ignored by the package-level convenience functions, which run on one-shot processors with caching disabled — construct a Processor via New to use the cache.
	CacheTTL          time.Duration // Time-to-live for cache entries. Default: 1 hour. Ignored by the package-level convenience functions (see MaxCacheEntries).
	CacheCleanup      time.Duration // Interval for background cleanup of expired cache entries. Set to 0 to disable. Default: 5 minutes. Ignored by the package-level convenience functions (see MaxCacheEntries).
	WorkerPoolSize    int           // Number of concurrent workers for batch processing. Default: 4. Must be positive and <= 256.
	ProcessingTimeout time.Duration // Maximum wall-clock time allowed for processing a single document. Default: 30 seconds. Set to 0 for no timeout. The deadline is wall-clock, not CPU time: batch pipelines or CPU-throttled containers intentionally processing very large documents should raise it (or set 0, with input size still bounded by MaxInputSize).

	// === Security ===
	EnableSanitization bool        // Controls whether HTML sanitization is applied. Default: true. Should only be disabled for trusted input.
	MaxDepth           int         // Maximum allowed nesting depth of HTML elements. Prevents stack overflow. Default: 500.
	AllowedBaseDir     string      // Restricts file operations to this directory. Empty (default) means no restriction. Use when accepting file paths from untrusted input.
	Audit              AuditConfig // Security audit logging configuration.

	// === Content Extraction ===
	ExtractArticle bool // Enables article extraction mode. When true, identifies and extracts main content. Default: true.
	// ExtractMetadata controls whether structured metadata (JSON-LD, OpenGraph
	// <meta> tags, rel=canonical) is collected to populate Result.Author,
	// Result.PublishedTime, Result.Description, Result.SiteName,
	// Result.CanonicalURL, and the metadata-based title priority (JSON-LD
	// headline / og:title ahead of the <title> tag). Default: true.
	ExtractMetadata bool
	PreserveImages  bool // Controls whether images are preserved in output. Default: true.
	PreserveLinks   bool // Controls whether links are preserved in output. Default: true.
	// PreserveVideos controls whether video elements are extracted. Default: true.
	// Note: URLs that appear only in raw HTML outside media elements (inside
	// script JSON, comments, etc.) are additionally scanned only for documents
	// up to 1MB; larger documents extract media from the parsed DOM only.
	PreserveVideos bool
	// PreserveAudios controls whether audio elements are extracted, with the
	// same 1MB raw-HTML scan limit as PreserveVideos. Default: true.
	PreserveAudios bool

	// === Output Formats ===
	InlineImageFormat string // How images are formatted in text output. Options: "none", "markdown", "html", "placeholder". Default: "none".
	InlineLinkFormat  string // How links are formatted in text output. Options: "none", "markdown", "html". Default: "none". Reader mode (ReadPage*) always preserves links and treats "none" as "markdown"; "none" applies to the Extract family only.
	TableFormat       string // How tables are formatted in output. Options: "markdown", "html". Default: "markdown".
	Encoding          string // Character encoding of input HTML. Leave empty for auto-detection. Common: "utf-8", "windows-1252", "gbk".

	// === Link Extraction ===
	// ResolveRelativeURLs controls whether relative URLs are resolved to
	// absolute URLs. Default: true. Resolution is against BaseURL when set;
	// when BaseURL is empty, ExtractAllLinks infers a base from the document
	// (<base>, og:url, rel=canonical, or the first absolute href/src as a
	// last resort). Protocol-relative references ("//host/path") are passed
	// through unchanged rather than inheriting the base's scheme.
	ResolveRelativeURLs  bool
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
		ExtractArticle:  true,
		ExtractMetadata: true,
		PreserveImages:  true,
		PreserveLinks:   true,
		PreserveVideos:  true,
		PreserveAudios:  true,

		// Output Formats
		InlineImageFormat: FormatNone,
		InlineLinkFormat:  FormatNone,
		TableFormat:       FormatMarkdown,

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

	if err := c.Audit.Validate(); err != nil {
		return err
	}

	// Validate format strings
	if err := validateFormat("InlineImageFormat", c.InlineImageFormat, []string{FormatNone, FormatMarkdown, FormatHTML, FormatPlaceholder}); err != nil {
		return err
	}
	if err := validateFormat("InlineLinkFormat", c.InlineLinkFormat, []string{FormatNone, FormatMarkdown, FormatHTML}); err != nil {
		return err
	}
	if err := validateFormat("TableFormat", c.TableFormat, []string{FormatMarkdown, FormatHTML}); err != nil {
		return err
	}

	return nil
}

// validateFormat validates that a format string is one of the allowed values.
// Empty string is allowed (will use default). The value is trimmed before the
// case-insensitive membership check, matching the TrimSpace the same value
// receives later in normalizeInlineFormat — validation was previously
// stricter than the normalization applied to the accepted value, rejecting
// " markdown" as invalid.
func validateFormat(field, value string, allowed []string) error {
	if value == "" {
		return nil // Empty means use default
	}
	lowerValue := strings.ToLower(strings.TrimSpace(value))
	if slices.Contains(allowed, lowerValue) {
		return nil
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
	cfg.InlineImageFormat = FormatMarkdown
	cfg.InlineLinkFormat = FormatMarkdown

	return cfg
}

// ============================================================================
// Result Types
// ============================================================================

// Result holds the extraction result.
type Result struct {
	// Text is the extracted plain-text content of the document.
	Text string `json:"text"`
	// Title is the document title. With metadata extraction enabled (the
	// default), a declared metadata title wins — the article-typed JSON-LD
	// headline over og:title, with a WebPage/WebSite JSON-LD name as the last
	// metadata resort — and is stripped of a site-name suffix only when the
	// trailing segment exactly matches the declared site name. Otherwise the
	// DOM ladder applies: <title>, then the h1 inside the selected article,
	// the first document h1, and the first h2.
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
	// WordCount is the number of whitespace-separated words in Text; each CJK
	// ideograph, kana, or Hangul syllable additionally counts as one word.
	WordCount int `json:"word_count"`
	// ReadingTime is the estimated reading time based on WordCount, using
	// separate per-script rates (200 words/min for space-separated words,
	// 350 characters/min for CJK). It is omitted from JSON and serialized as
	// reading_time_ms by MarshalJSON.
	ReadingTime time.Duration `json:"-"`

	// Author is the article author from JSON-LD author or <meta name="author">,
	// joined with ", " when several are declared. Empty when absent. It is
	// omitted from JSON when empty.
	Author string `json:"author,omitempty"`
	// PublishedTime is the publication timestamp from article:published_time or
	// JSON-LD datePublished, parsed to time.Time. The zero time means unknown.
	// It is omitted from JSON when zero (omitzero).
	PublishedTime time.Time `json:"published_time,omitzero"`
	// Description is the page summary from og:description, JSON-LD description,
	// or <meta name="description">, in that priority. It is omitted from JSON
	// when empty.
	Description string `json:"description,omitempty"`
	// SiteName is the publishing site's name from og:site_name or JSON-LD
	// publisher.name. It is omitted from JSON when empty.
	SiteName string `json:"site_name,omitempty"`
	// CanonicalURL is the document's canonical location from <link
	// rel="canonical"> or og:url. It is omitted from JSON when empty.
	CanonicalURL string `json:"canonical_url,omitempty"`
	// Confidence is a coarse extraction-quality signal in [0, 1]: 1.0 means an
	// article node was identified with substantial text covering a sane share
	// of the page; lower tiers flag body fallback, very short output, or
	// consent/paywall boilerplate (see computeExtractionConfidence for the
	// exact tiers). It lets pipeline callers decide when to fall back or flag
	// a document for review.
	Confidence float64 `json:"confidence"`
	// ExtractionMethod reports how Text was produced: "article" when the
	// scorer identified a main-content node, "listing" when the page was
	// recognized as a card listing/index (Text is the whole card list), and
	// "body" when extraction fell back to the full <body> (no candidate scored
	// above zero, or ExtractArticle is false). It is omitted from JSON when
	// empty.
	ExtractionMethod string `json:"extraction_method,omitempty"`
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
//
// A fragment-only ("#top") or query-only ("?page=2") href is page-internal
// navigation rather than a link to a document, so it produces no LinkInfo.
// Links with a scheme the validator rejects (javascript:, vbscript:, file:,
// unsafe data: URLs) are excluded for the same reason, and their position
// number is still consumed so the [LINK:n] placeholders stay aligned with
// Result.Text.
type LinkInfo struct {
	// URL is the link destination (the href attribute).
	URL string `json:"url"`
	// Text is the link's visible text content.
	Text string `json:"text"`
	// Title is the advisory title (the title attribute).
	Title string `json:"title"`
	// IsExternal is true when the URL is absolute (http/https or
	// protocol-relative), regardless of domain: same-domain absolute links
	// also report true, and relative URLs report false. It does not compare
	// the host against BaseURL.
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
	// Type is the video's container: the MIME type derived from the URL's
	// extension ("video/mp4", "video/webm", …), an explicit <source type> or
	// <embed type> value when the markup declares one (which wins, since a
	// server may serve a different container than the extension suggests), or
	// "embed" for iframe/embed-host videos. Empty when the URL has no
	// recognizable media extension and the markup declares no type.
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
	// Type is the audio's container: the MIME type derived from the URL's
	// extension ("audio/mpeg", "audio/ogg", …) or an explicit <source type>
	// value when the markup declares one. Empty when the extension is not a
	// recognized audio extension and no type is declared.
	Type string `json:"type"`
	// Duration is the duration attribute, as an unparsed string.
	Duration string `json:"duration"`
}

// LinkResource represents a link resource extracted from HTML.
//
// Only resolvable references are reported. A fragment-only ("#top") or
// query-only ("?page=2") href is page-internal navigation, not a resource
// reference, so it produces no LinkResource — parsing it as a path would
// silently resolve it against the document base and invent a URL that was
// never in the markup. The same rule applies to Result.Links.
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
	// ErrorCount counts extractions that failed once processing had begun:
	// oversize input, encoding-conversion failure, and every error raised
	// inside the processing stage (including the processing timeout and
	// cancellation that lands there). It is not incremented when context
	// cancellation is observed before processing starts, on calls rejected
	// by a closed processor, or for file-read errors (missing files,
	// traversal rejections, I/O failures).
	ErrorCount int64
	// AverageProcessTime is the mean wall-clock time per extraction.
	AverageProcessTime time.Duration
}
