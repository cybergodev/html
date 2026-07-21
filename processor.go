package html

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cybergodev/html/internal"
	stdxhtml "golang.org/x/net/html"
)

// Scorer defines the interface for content scoring algorithms.
// Implementations can provide custom scoring logic for content extraction.
// If no custom scorer is provided, the DefaultScorer is used.
//
// Implementations MUST be safe for concurrent use: a single Processor invokes
// Score and ShouldRemove from multiple goroutines when it is shared across
// concurrent Extract calls. The built-in DefaultScorer/SharedDefaultScorer is
// read-only and satisfies this; a custom Scorer that holds mutable state must
// synchronize it itself.
//
// The ContentNode interface abstracts away the internal HTML parser types,
// allowing users to implement custom scorers without importing golang.org/x/net/html.
//
// # Architecture Notes
//
// This public interface uses ContentNode abstraction to hide the internal
// golang.org/x/net/html dependency. Internally, the scorerAdapter converts
// between this interface and internal.Scorer which uses *html.Node directly
// for performance. This dual-interface design provides:
//   - Clean public API (no external parser types exposed)
//   - High performance internally (direct node access)
//   - Flexibility for users to implement custom scoring
type Scorer interface {
	// Score calculates a relevance score for a content node.
	// Higher scores indicate more likely main content.
	Score(node ContentNode) int
	// ShouldRemove determines if a node should be removed from the content tree.
	ShouldRemove(node ContentNode) bool
}

// scorerAdapter adapts the public Scorer interface to the internal Scorer interface.
type scorerAdapter struct {
	external Scorer
}

func (a *scorerAdapter) Score(node *stdxhtml.Node) int {
	if a.external == nil || node == nil {
		return 0
	}
	return a.external.Score(contentNodeAdapter{node})
}

func (a *scorerAdapter) ShouldRemove(node *stdxhtml.Node) bool {
	if a.external == nil || node == nil {
		return false
	}
	return a.external.ShouldRemove(contentNodeAdapter{node})
}

// contentNodeAdapter adapts *stdxhtml.Node to ContentNode interface.
type contentNodeAdapter struct {
	*stdxhtml.Node
}

func (n contentNodeAdapter) Type() string {
	if n.Node == nil {
		return ""
	}
	switch n.Node.Type {
	case stdxhtml.ErrorNode:
		return "error"
	case stdxhtml.TextNode:
		return "text"
	case stdxhtml.DocumentNode:
		return "document"
	case stdxhtml.ElementNode:
		return "element"
	case stdxhtml.CommentNode:
		return "comment"
	case stdxhtml.DoctypeNode:
		return "doctype"
	case stdxhtml.RawNode:
		return "raw"
	default:
		return "unknown"
	}
}

func (n contentNodeAdapter) Data() string {
	if n.Node == nil {
		return ""
	}
	return n.Node.Data
}

func (n contentNodeAdapter) AttrValue(key string) string {
	if n.Node == nil {
		return ""
	}
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func (n contentNodeAdapter) Attrs() []NodeAttr {
	if n.Node == nil {
		return nil
	}
	attrs := make([]NodeAttr, len(n.Attr))
	for i, attr := range n.Attr {
		attrs[i] = NodeAttr{Key: attr.Key, Value: attr.Val}
	}
	return attrs
}

func (n contentNodeAdapter) FirstChild() ContentNode {
	if n.Node == nil || n.Node.FirstChild == nil {
		return nil
	}
	return contentNodeAdapter{n.Node.FirstChild}
}

func (n contentNodeAdapter) NextSibling() ContentNode {
	if n.Node == nil || n.Node.NextSibling == nil {
		return nil
	}
	return contentNodeAdapter{n.Node.NextSibling}
}

func (n contentNodeAdapter) Parent() ContentNode {
	if n.Node == nil || n.Node.Parent == nil {
		return nil
	}
	return contentNodeAdapter{n.Node.Parent}
}

// Processor is the main HTML processing engine.
// It provides methods for extracting content, links, and media from HTML documents
// with automatic encoding detection and caching support.
type Processor struct {
	config   *Config
	configMu sync.Mutex // Protects config snapshot copy during extractWithFormats
	cache    *internal.Cache[[16]byte]
	scorer   internal.Scorer
	audit    *auditCollector
	closed   atomic.Bool
	stats    *processorStats

	// Pre-computed format strings to avoid repeated strings.ToLower in hot path
	imageFormat string
	linkFormat  string
	// Cached audit adapter to avoid per-call allocation
	auditAdapter *auditRecorderAdapter
}

// processorStats holds thread-safe statistics counters shared between processors.
type processorStats struct {
	totalProcessed   atomic.Int64
	cacheHits        atomic.Int64
	cacheMisses      atomic.Int64
	errorCount       atomic.Int64
	totalProcessTime atomic.Int64
}

// New creates a new HTML processor with optional configuration.
// If no configuration is provided, DefaultConfig() is used.
//
// Example usage:
//
//	// Simple usage with default configuration
//	processor, err := html.New()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer processor.Close()
//
//	// With custom configuration
//	cfg := html.DefaultConfig()
//	cfg.MaxInputSize = 10 * 1024 * 1024
//	cfg.InlineImageFormat = "markdown"
//	processor, err := html.New(cfg)
//
//	// Or use preset configurations
//	processor, err := html.New(html.HighSecurityConfig())
//
// To use a custom Scorer, set the Scorer field:
//
//	cfg := html.DefaultConfig()
//	cfg.Scorer = myScorer
//	processor, err := html.New(cfg)
//
// Returns:
//   - ErrMultipleConfigs if more than one Config is provided
//   - ErrInvalidConfig (wrapped in *ConfigError) if the configuration fails validation
func New(cfg ...Config) (*Processor, error) {
	c, _, err := resolveConfig(cfg...)
	if err != nil {
		return nil, err
	}

	if err := c.Validate(); err != nil {
		return nil, err
	}

	p := &Processor{
		config: &c,
		cache:  internal.NewCache[[16]byte](c.MaxCacheEntries, c.CacheTTL),
		audit:  newAuditCollector(c.Audit),
		stats:  &processorStats{},
	}

	// Pre-compute normalized format strings to avoid repeated strings.ToLower in hot path
	p.imageFormat = normalizeInlineFormat(c.InlineImageFormat)
	p.linkFormat = normalizeInlineFormat(c.InlineLinkFormat)

	// Cache audit adapter to avoid per-call allocation
	p.auditAdapter = &auditRecorderAdapter{collector: p.audit}

	// Set up scorer from config
	// Note: Scorer interface uses ContentNode abstraction; adapter converts to internal.Scorer
	if c.Scorer != nil {
		p.scorer = &scorerAdapter{external: c.Scorer}
	} else {
		p.scorer = internal.SharedDefaultScorer()
	}

	// Start background cache cleanup if TTL and cleanup interval are configured
	if c.CacheTTL > 0 && c.CacheCleanup > 0 {
		p.cache.StartCleanup(c.CacheCleanup)
	}

	return p, nil
}

// normalizeInlineFormat lowercases, trims, and defaults an inline format string.
// An empty value maps to "none", matching the default used by DefaultConfig and New.
func normalizeInlineFormat(format string) string {
	f := strings.ToLower(strings.TrimSpace(format))
	if f == "" {
		f = "none"
	}
	return f
}

// GetStatistics returns current processing statistics.
func (p *Processor) GetStatistics() Statistics {
	if p == nil || p.stats == nil {
		return Statistics{}
	}
	totalProcessed := p.stats.totalProcessed.Load()
	totalTime := time.Duration(p.stats.totalProcessTime.Load())
	var avgTime time.Duration
	if totalProcessed > 0 {
		avgTime = totalTime / time.Duration(totalProcessed)
	}
	return Statistics{
		TotalProcessed:     totalProcessed,
		CacheHits:          p.stats.cacheHits.Load(),
		CacheMisses:        p.stats.cacheMisses.Load(),
		ErrorCount:         p.stats.errorCount.Load(),
		AverageProcessTime: avgTime,
	}
}

// GetAuditLog returns the audit log entries collected during processing.
// Returns nil if audit logging is not enabled.
func (p *Processor) GetAuditLog() []AuditEntry {
	if p == nil || p.audit == nil {
		return nil
	}
	return p.audit.GetEntries()
}

// ClearAuditLog clears all collected audit log entries.
func (p *Processor) ClearAuditLog() {
	if p == nil || p.audit == nil {
		return
	}
	p.audit.Clear()
}

// ClearCache clears the cache contents but preserves cumulative statistics.
// Use ResetStatistics to reset statistics counters.
func (p *Processor) ClearCache() {
	if p == nil {
		return
	}
	p.cache.Clear()
}

// ResetStatistics resets all statistics counters to zero.
// This preserves cache entries while clearing the accumulated metrics.
func (p *Processor) ResetStatistics() {
	if p == nil || p.stats == nil {
		return
	}
	p.stats.cacheHits.Store(0)
	p.stats.cacheMisses.Store(0)
	p.stats.errorCount.Store(0)
	p.stats.totalProcessed.Store(0)
	p.stats.totalProcessTime.Store(0)
}

// Close releases resources used by the processor.
// After calling Close, the processor should not be used.
func (p *Processor) Close() error {
	if p == nil {
		return nil
	}
	if !p.closed.CompareAndSwap(false, true) {
		return nil
	}
	// Stop background cleanup goroutine if running
	p.cache.StopCleanup()
	p.cache.Clear()
	if p.audit != nil {
		_ = p.audit.Close() // best-effort cleanup
	}
	return nil
}

// validateInput performs common validation for HTML input.
// It checks for nil/closed processor and input size limits.
// Returns an error if validation fails, nil otherwise.
func (p *Processor) validateInput(htmlBytes []byte) error {
	if p == nil || p.closed.Load() {
		return ErrProcessorClosed
	}
	if len(htmlBytes) > p.config.MaxInputSize {
		return p.inputTooLargeError("Extract", len(htmlBytes))
	}
	return nil
}

// inputTooLargeError builds the canonical *InputError for an oversize input,
// records the violation to the audit log (when enabled), and bumps the error
// counter. Centralizing this keeps the byte-input path (validateInput) and the
// file path (errFileTooLarge) producing the same ErrInputTooLarge error shape.
func (p *Processor) inputTooLargeError(op string, size int) error {
	if p.audit != nil {
		p.audit.RecordInputViolation(size, p.config.MaxInputSize, "input_too_large")
	}
	p.stats.errorCount.Add(1)
	return newInputError(op, size, p.config.MaxInputSize, nil)
}

// errFileTooLarge returns an *InputError when a regular file's size exceeds
// MaxInputSize, so the file is rejected before its contents are read into
// memory. Non-regular files (pipes, devices, sockets) report an implausible
// Stat size (often 0), so they are never rejected here — they are bounded
// instead by the byte-level MaxInputSize check that runs after the read in
// validateInput. Returns nil when the file is within the limit.
func (p *Processor) errFileTooLarge(regular bool, size int64) error {
	if !regular || size <= int64(p.config.MaxInputSize) {
		return nil
	}
	return p.inputTooLargeError("ExtractFromFile", int(size))
}

// detectEncoding detects the character encoding and converts HTML bytes to UTF-8.
// This is a helper method used by multiple extraction methods to avoid code duplication.
// It records encoding issues to the audit log if enabled.
func (p *Processor) detectEncoding(htmlBytes []byte) (string, error) {
	utf8String, _, convErr := internal.DetectAndConvertToUTF8String(htmlBytes, p.config.Encoding)
	if convErr != nil {
		if p.audit != nil {
			p.audit.RecordEncodingIssue(p.config.Encoding, convErr.Error())
		}
		p.stats.errorCount.Add(1)
		return "", fmt.Errorf("encoding detection failed: %w", convErr)
	}
	return utf8String, nil
}

// validateAndReadFile validates the file path and reads the file contents.
// It performs security checks including path traversal detection and optional directory restriction.
// Returns the file contents or an appropriate error.
func (p *Processor) validateAndReadFile(filePath string) ([]byte, error) {
	// Validate file path
	if filePath == "" {
		return nil, newFileError("ReadFile", filePath, ErrInvalidFilePath)
	}

	// Clean the file path to resolve any "." or ".." components
	cleanPath := filepath.Clean(filePath)

	// After cleaning, check if the path contains parent directory references
	// This catches path traversal attempts like "../file", "subdir/../../file", etc.
	if strings.Contains(cleanPath, "..") {
		if p.audit != nil {
			p.audit.RecordPathTraversal(filePath)
		}
		return nil, newFileError("ReadFile", filePath, fmt.Errorf("path traversal detected: %s", cleanPath))
	}

	// Enforce AllowedBaseDir restriction when configured. Containment is verified
	// against the real on-disk path resolved through the OS file handle, which
	// catches symlinks (all platforms) and Windows junctions/reparse points
	// (which need no privilege and are NOT resolved by filepath.EvalSymlinks).
	// See readContained for details.
	if p.config.AllowedBaseDir != "" {
		return p.readContained(cleanPath)
	}

	// Pre-check the file size against MaxInputSize so an oversized file is
	// rejected before os.ReadFile materializes it in memory. The byte-level
	// check in validateInput still guards downstream processing; this guard
	// closes the read-time memory-exhaustion window for untrusted paths.
	// AllowedBaseDir confines WHICH file may be read, not how large it is.
	info, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, newFileError("ReadFile", cleanPath, ErrFileNotFound)
		}
		return nil, newFileError("ReadFile", cleanPath, err)
	}
	if err := p.errFileTooLarge(info.Mode().IsRegular(), info.Size()); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, newFileError("ReadFile", cleanPath, ErrFileNotFound)
		}
		return nil, newFileError("ReadFile", cleanPath, err)
	}

	return data, nil
}

// readContained enforces the AllowedBaseDir restriction for a file read.
//
// It opens the target, resolves its true on-disk path through the OS file
// handle (realPath), confirms that path is within the configured base, and only
// then reads from the already-open handle. Resolving through the handle (rather
// than the path) is what closes the containment gaps that a path-only check
// leaves open:
//
//   - Symlinks inside the allowed tree pointing outside it (all platforms).
//   - Windows directory junctions / reparse points, which need no privilege to
//     create and are NOT resolved by filepath.EvalSymlinks.
//
// Reading the same handle that was verified also removes the TOCTOU window
// between the check and the read. The resolved real path is used only for the
// containment decision and never reaches the returned error (which carries the
// caller-supplied path, sanitized by FileError), so no filesystem layout is
// disclosed on rejection.
func (p *Processor) readContained(cleanPath string) ([]byte, error) {
	absBase, err := filepath.Abs(p.config.AllowedBaseDir)
	if err != nil {
		return nil, newFileError("ReadFile", cleanPath, fmt.Errorf("invalid AllowedBaseDir: %w", err))
	}

	f, err := os.Open(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, newFileError("ReadFile", cleanPath, ErrFileNotFound)
		}
		return nil, newFileError("ReadFile", cleanPath, err)
	}
	defer f.Close()

	realTarget, err := realPath(f)
	if err != nil {
		return nil, newFileError("ReadFile", cleanPath, fmt.Errorf("failed to resolve path: %w", err))
	}

	realBase, err := resolveRealPath(absBase)
	if err != nil {
		return nil, newFileError("ReadFile", cleanPath, fmt.Errorf("invalid AllowedBaseDir: %w", err))
	}

	if !pathWithin(realBase, realTarget) {
		if p.audit != nil {
			p.audit.RecordPathTraversal(cleanPath)
		}
		return nil, newFileError("ReadFile", cleanPath, fmt.Errorf("path outside allowed directory"))
	}

	// Pre-check size against MaxInputSize on the same verified handle (no
	// second path resolution, no TOCTOU window) so an oversized file inside
	// the allowed tree is rejected before io.ReadAll loads it into memory.
	if info, err := f.Stat(); err == nil {
		if err := p.errFileTooLarge(info.Mode().IsRegular(), info.Size()); err != nil {
			return nil, err
		}
	}

	return io.ReadAll(f)
}

// resolveRealPath opens path, resolves its true on-disk location, and closes it.
// It is the path-based counterpart to realPath, used to put the base directory
// into the same canonical form as the target so the containment comparison is
// exact (including when the base itself crosses a symlink or junction).
func resolveRealPath(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return realPath(f)
}

// pathWithin reports whether target is realBase or located beneath it. Both
// inputs must be cleaned, absolute paths in the same canonical form.
func pathWithin(realBase, target string) bool {
	base := filepath.Clean(realBase)
	if !strings.HasSuffix(base, string(filepath.Separator)) {
		base += string(filepath.Separator)
	}
	return strings.HasPrefix(filepath.Clean(target)+string(filepath.Separator), base)
}
