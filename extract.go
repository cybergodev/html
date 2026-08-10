package html

import (
	"context"
	"errors"
	"fmt"
	htmlstd "html"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cybergodev/html/internal"
	stdxhtml "golang.org/x/net/html"
)

// markdownEscapeReplacer escapes characters that could break Markdown link/image syntax.
var markdownEscapeReplacer = strings.NewReplacer(
	`\`, `\\`,
	`[`, `\[`,
	`]`, `\]`,
)

// containsASCIIFold reports whether substr is contained in s, case-insensitively
// (ASCII only). Both s and substr are folded to lowercase before comparison.
func containsASCIIFold(s, substr string) bool {
	substrLen := len(substr)
	sLen := len(s)
	if substrLen > sLen {
		return false
	}
	for i := 0; i <= sLen-substrLen; i++ {
		match := true
		for j := 0; j < substrLen; j++ {
			c := s[i+j]
			sc := substr[j]
			if c >= 'A' && c <= 'Z' {
				c += 32
			}
			if sc >= 'A' && sc <= 'Z' {
				sc += 32
			}
			if c != sc {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// escapeMarkdownText escapes characters that could break Markdown link/image syntax.
// Escapes ], [, and \ to prevent injection of arbitrary Markdown content.
func escapeMarkdownText(s string) string {
	// Fast path: strings.(*Replacer).Replace allocates a new string even when
	// nothing matches, so scan first and skip the replacer entirely when the
	// text contains none of the three escapable characters. Link and image
	// text almost never contains them, so this avoids an allocation per call.
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\', '[', ']':
			return markdownEscapeReplacer.Replace(s)
		}
	}
	return s
}

// Timeout goroutine management constants.
// These prevent goroutine leaks when many timeout operations occur.
const (
	// maxTimeoutGoroutines limits concurrent timeout goroutines to prevent
	// resource exhaustion. Value 1000 allows ~1GB of goroutine stack overhead
	// assuming 1MB stack per goroutine, which is a reasonable safety limit.
	// When exceeded, new operations return ErrProcessingTimeout immediately.
	maxTimeoutGoroutines = 1000
)

// activeTimeoutGoroutines tracks the number of currently running timeout goroutines.
var activeTimeoutGoroutines atomic.Int64

// recoverPanic wraps a function with defense-in-depth panic recovery.
func recoverPanic[T any](fn func() (T, error)) (result T, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", ErrInternalPanic, r)
		}
	}()
	return fn()
}

// Extract extracts content from HTML bytes with automatic encoding detection.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// For better performance with multiple extractions, create a Processor directly:
//
//	processor, _ := html.New()
//	defer processor.Close()
//	for _, html := range htmlDocs {
//	    result, _ := processor.Extract(html)  // Uses cache
//	}
//
// It automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML bytes and converts it to UTF-8 before processing.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
//
// Examples:
//
//	// With default config
//	result, err := html.Extract(htmlBytes)
//
//	// With custom config
//	cfg := html.DefaultConfig()
//	cfg.PreserveImages = false
//	result, err := html.Extract(htmlBytes, cfg)
//
// Returns the same errors as [Processor.Extract], plus ErrMultipleConfigs if
// more than one Config is provided and ErrInvalidConfig (wrapped in *ConfigError)
// if the supplied configuration is invalid.
func Extract(htmlBytes []byte, cfg ...Config) (*Result, error) {
	c, pooled, err := resolveConfig(cfg...)
	if err != nil {
		return nil, err
	}
	return withProcessor(pooled, c, func(p *Processor) (*Result, error) {
		return p.Extract(htmlBytes)
	})
}

// ExtractFromFile extracts content from an HTML file with automatic encoding detection.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// It automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML file and converts it to UTF-8 before processing.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
//
// In addition to the errors returned by [Extract], file access may fail with a
// *FileError wrapping ErrFileNotFound, ErrInvalidFilePath, or a path-traversal
// rejection (see Config.AllowedBaseDir).
func ExtractFromFile(filePath string, cfg ...Config) (*Result, error) {
	c, pooled, err := resolveConfig(cfg...)
	if err != nil {
		return nil, err
	}
	return withProcessor(pooled, c, func(p *Processor) (*Result, error) {
		return p.ExtractFromFile(filePath)
	})
}

// ExtractText extracts plain text from HTML bytes with automatic encoding detection.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// For better performance with multiple extractions, create a Processor directly.
//
// It automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML bytes and converts it to UTF-8 before processing.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
//
// Returns the same errors as [Extract].
func ExtractText(htmlBytes []byte, cfg ...Config) (string, error) {
	c, pooled, err := resolveConfig(cfg...)
	if err != nil {
		return "", err
	}
	return withProcessor(pooled, c, func(p *Processor) (string, error) {
		return p.ExtractText(htmlBytes)
	})
}

// ExtractTextFromFile extracts plain text from an HTML file with automatic encoding detection.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// It automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML file and converts it to UTF-8 before processing.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
//
// Returns the same errors as [ExtractFromFile].
func ExtractTextFromFile(filePath string, cfg ...Config) (string, error) {
	c, pooled, err := resolveConfig(cfg...)
	if err != nil {
		return "", err
	}
	return withProcessor(pooled, c, func(p *Processor) (string, error) {
		return p.ExtractTextFromFile(filePath)
	})
}

// ExtractTextWithContext extracts plain text from HTML bytes with context support for cancellation.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
//
// Returns the same errors as [ExtractWithContext].
func ExtractTextWithContext(ctx context.Context, htmlBytes []byte, cfg ...Config) (string, error) {
	c, pooled, err := resolveConfig(cfg...)
	if err != nil {
		return "", err
	}
	return withProcessor(pooled, c, func(p *Processor) (string, error) {
		return p.ExtractTextWithContext(ctx, htmlBytes)
	})
}

// ExtractTextFromFileWithContext extracts plain text from an HTML file with context support for cancellation.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
//
// Returns the same errors as [ExtractFromFileWithContext].
func ExtractTextFromFileWithContext(ctx context.Context, filePath string, cfg ...Config) (string, error) {
	c, pooled, err := resolveConfig(cfg...)
	if err != nil {
		return "", err
	}
	return withProcessor(pooled, c, func(p *Processor) (string, error) {
		return p.ExtractTextFromFileWithContext(ctx, filePath)
	})
}

// ExtractWithContext extracts content from HTML bytes with context support for cancellation.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
//
// Returns the same errors as [Extract], plus context.Canceled or
// context.DeadlineExceeded when ctx is cancelled.
func ExtractWithContext(ctx context.Context, htmlBytes []byte, cfg ...Config) (*Result, error) {
	c, pooled, err := resolveConfig(cfg...)
	if err != nil {
		return nil, err
	}
	return withProcessor(pooled, c, func(p *Processor) (*Result, error) {
		return p.ExtractWithContext(ctx, htmlBytes)
	})
}

// ExtractFromFileWithContext extracts content from an HTML file with context support for cancellation.
// This is a convenience function that uses a pooled Processor for efficiency.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
//
// Returns the same errors as [ExtractFromFile], plus context.Canceled or
// context.DeadlineExceeded when ctx is cancelled.
func ExtractFromFileWithContext(ctx context.Context, filePath string, cfg ...Config) (*Result, error) {
	c, pooled, err := resolveConfig(cfg...)
	if err != nil {
		return nil, err
	}
	return withProcessor(pooled, c, func(p *Processor) (*Result, error) {
		return p.ExtractFromFileWithContext(ctx, filePath)
	})
}

// Extract extracts content from HTML bytes with automatic encoding detection.
// The method automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML bytes and converts it to UTF-8 before processing.
//
// This is the primary method for HTML content extraction when the source encoding
// may not be UTF-8, such as content from HTTP responses, databases, or files.
//
// Returns:
//   - ErrProcessorClosed if the processor is nil or has been closed
//   - ErrInputTooLarge (wrapped in *InputError) if len(htmlBytes) exceeds MaxInputSize
//   - a wrapped error if character-encoding detection or conversion fails
//   - ErrInvalidHTML if the bytes cannot be parsed as HTML
//   - ErrMaxDepthExceeded if element nesting exceeds MaxDepth
//   - ErrProcessingTimeout if processing exceeds ProcessingTimeout
//   - ErrInternalPanic if an unexpected internal panic is recovered
func (p *Processor) Extract(htmlBytes []byte) (*Result, error) {
	return recoverPanic(func() (*Result, error) {
		return p.extractCoreWithContext(context.Background(), htmlBytes)
	})
}

// ExtractWithContext extracts content from HTML bytes with context support for cancellation.
// This method provides cooperative cancellation, allowing long-running extractions to be
// interrupted when the context is cancelled.
//
// The method automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML bytes and converts it to UTF-8 before processing.
//
// Example usage:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//
//	result, err := processor.ExtractWithContext(ctx, htmlBytes)
//	if errors.Is(err, context.Canceled) {
//	    // Extraction was cancelled
//	}
//
// In addition to the errors returned by [Processor.Extract], this method returns
// context.Canceled or context.DeadlineExceeded when ctx is cancelled.
func (p *Processor) ExtractWithContext(ctx context.Context, htmlBytes []byte) (*Result, error) {
	return recoverPanic(func() (*Result, error) {
		return p.extractCoreWithContext(ctx, htmlBytes)
	})
}

func (p *Processor) extractCoreWithContext(ctx context.Context, htmlBytes []byte) (*Result, error) {
	// Early cancellation check
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Validate processor state and input size
	if err := p.validateInput(htmlBytes); err != nil {
		return nil, err
	}

	startTime := time.Now()

	// Check the cache before encoding detection. The cache key is derived from
	// the raw input bytes (not the decoded string), so a cache hit skips the
	// O(n) encoding scan entirely — the single most expensive step on the hot
	// (repeated-extraction) path. This is safe because the same raw bytes + the
	// same config (including Encoding setting, which is mixed into the key)
	// deterministically produce the same extraction result.
	//
	// Note: Cache.Get/Set treat the zero key ([16]byte{}) as "no key" and reject
	// it, so in the astronomically unlikely event (2^-128) that this hash comes
	// out all zero, that input is simply a permanent cache miss — reprocessed on
	// every call. This is an accepted tradeoff.
	var cacheKey [16]byte
	hasCacheKey := false
	if p.config.MaxCacheEntries > 0 {
		cacheKey = p.generateCacheKey(htmlBytes)
		hasCacheKey = true
		if cached := p.cache.Get(cacheKey); cached != nil {
			// Count a hit only when the cached value is actually usable.
			if cachedResult, ok := cached.(*Result); ok {
				p.stats.cacheHits.Add(1)
				p.stats.totalProcessed.Add(1)
				return cloneResult(cachedResult), nil
			}
		}
		p.stats.cacheMisses.Add(1)
	}

	// Check cancellation before encoding detection
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Detect encoding and convert to UTF-8 (skipped entirely on cache hits above)
	utf8String, err := p.detectEncoding(htmlBytes)
	if err != nil {
		return nil, err
	}

	// Check cancellation before content processing
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Process content with optional timeout and context support. The timeout is
	// applied by deriving a deadline from ctx and threading it through
	// withTimeout into processContentWithContext, whose cooperative cancellation
	// checks then honor the deadline — so an expired timeout interrupts in-flight
	// work at the next check rather than merely racing the return value while
	// extraction runs to completion.
	var result *Result
	result, err = runWithTimeout(p, ctx, func(deadlineCtx context.Context) (*Result, error) {
		return p.processContentWithContext(deadlineCtx, utf8String)
	})

	if err != nil {
		p.stats.errorCount.Add(1)
		if p.audit != nil {
			if errors.Is(err, ErrProcessingTimeout) {
				p.audit.RecordTimeout(p.config.ProcessingTimeout)
			} else if errors.Is(err, ErrMaxDepthExceeded) {
				p.audit.RecordDepthViolation(p.config.MaxDepth+1, p.config.MaxDepth)
			}
		}
		return nil, err
	}

	processingTime := time.Since(startTime)
	result.ProcessingTime = processingTime
	p.stats.totalProcessTime.Add(int64(processingTime))
	p.stats.totalProcessed.Add(1)

	// Only cache if caching is enabled and we generated a key for this input.
	if hasCacheKey {
		p.cache.Set(cacheKey, result)
		// Return a clone so the caller owns an independent copy. The cached
		// entry and the returned value must not alias: a caller that mutates
		// its result would otherwise race with concurrent cache-hit reads
		// (cloneResult) and silently corrupt the cache. This mirrors the clone
		// taken on the cache-hit path, closing the same ownership contract.
		return cloneResult(result), nil
	}

	return result, nil
}

// processContentWithContext processes HTML content with context cancellation support.
// This method implements cooperative cancellation at key processing stages.
func (p *Processor) processContentWithContext(ctx context.Context, htmlContent string) (*Result, error) {
	// Check for cancellation at start
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if p.isBlankContent(htmlContent) {
		return &Result{}, nil
	}

	originalHTML := htmlContent

	// Check context before HTML parsing
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	doc, err := stdxhtml.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidHTML, err)
	}

	// Check context before depth validation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Validate DOM depth BEFORE any recursive traversal. SanitizeDOM (and other
	// downstream passes) recurse without an internal depth cap, so a
	// pathologically deep document could exhaust the call stack before this
	// iterative guard rejected it. Running the O(nodes) iterative depth check
	// first bounds every later recursive pass to MaxDepth. Depth is measured on
	// the raw parsed tree; sanitization only removes nodes, so this is at most
	// marginally stricter than the previous sanitize-then-validate order.
	if err := p.validateDepthTraversal(doc, 0); err != nil {
		return nil, err
	}

	// Sanitize DOM in-place (avoids render + re-parse overhead). Runs only on a
	// depth-validated tree, so its recursion is bounded by MaxDepth.
	if p.config.EnableSanitization {
		if p.audit != nil && p.config.Audit.Enabled {
			internal.SanitizeDOM(doc, p.auditAdapter)
		} else {
			internal.SanitizeDOM(doc, internal.NoOpAuditRecorder{})
		}
	}

	// Check context before document extraction
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return p.extractFromDocument(doc, originalHTML)
}

// ExtractFromFile extracts content from an HTML file with automatic encoding detection.
// The method automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML file and converts it to UTF-8 before processing.
// Use this when you have a file path instead of raw bytes.
//
// Returns the same errors as [Processor.Extract], plus a *FileError wrapping
// ErrFileNotFound, ErrInvalidFilePath, or a path-traversal rejection for
// file-access failures.
func (p *Processor) ExtractFromFile(filePath string) (*Result, error) {
	return recoverPanic(func() (*Result, error) {
		// Validate processor state (no input size check for file paths)
		if p == nil || p.closed.Load() {
			return nil, ErrProcessorClosed
		}

		data, err := p.validateAndReadFile(filePath)
		if err != nil {
			return nil, err
		}

		return p.Extract(data)
	})
}

// ExtractFromFileWithContext extracts content from an HTML file with context support.
// This method provides cooperative cancellation, allowing long-running extractions to be
// interrupted when the context is cancelled.
//
// Example usage:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//
//	result, err := processor.ExtractFromFileWithContext(ctx, "page.html")
//
// In addition to the errors returned by [Processor.ExtractFromFile], this method
// returns context.Canceled or context.DeadlineExceeded when ctx is cancelled.
func (p *Processor) ExtractFromFileWithContext(ctx context.Context, filePath string) (*Result, error) {
	return recoverPanic(func() (*Result, error) {
		// Early cancellation check
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Validate processor state (no input size check for file paths)
		if p == nil || p.closed.Load() {
			return nil, ErrProcessorClosed
		}

		data, err := p.validateAndReadFile(filePath)
		if err != nil {
			return nil, err
		}

		return p.ExtractWithContext(ctx, data)
	})
}

// ExtractText extracts plain text from HTML bytes with automatic encoding detection.
// The method automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML bytes and converts it to UTF-8 before processing.
// This is a convenience method that returns only the text content without other metadata.
// Returns the same errors as [Processor.Extract].
func (p *Processor) ExtractText(htmlBytes []byte) (string, error) {
	result, err := p.Extract(htmlBytes)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

// ExtractTextFromFile extracts plain text from an HTML file with automatic encoding detection.
// The method automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML file and converts it to UTF-8 before processing.
// Use this when you have a file path instead of raw bytes.
// This is a convenience method that returns only the text content without other metadata.
// Returns the same errors as [Processor.ExtractFromFile].
func (p *Processor) ExtractTextFromFile(filePath string) (string, error) {
	result, err := p.ExtractFromFile(filePath)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

// ExtractTextWithContext extracts plain text from HTML bytes with context support for cancellation.
// The method automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML bytes and converts it to UTF-8 before processing.
// This is a convenience method that returns only the text content without other metadata.
// Returns the same errors as [Processor.ExtractWithContext].
func (p *Processor) ExtractTextWithContext(ctx context.Context, htmlBytes []byte) (string, error) {
	result, err := p.ExtractWithContext(ctx, htmlBytes)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

// ExtractTextFromFileWithContext extracts plain text from an HTML file with context support for cancellation.
// The method automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML file and converts it to UTF-8 before processing.
// Use this when you have a file path instead of raw bytes.
// This is a convenience method that returns only the text content without other metadata.
// Returns the same errors as [Processor.ExtractFromFileWithContext].
func (p *Processor) ExtractTextFromFileWithContext(ctx context.Context, filePath string) (string, error) {
	result, err := p.ExtractFromFileWithContext(ctx, filePath)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

// withTimeout executes a function under a deadline derived from parent, returning
// its result or an error if the timeout expires. This is a generic helper that
// eliminates code duplication in timeout handling.
//
// The deadline context is passed into fn so that fn's own cooperative
// cancellation checks (e.g. the select-on-ctx.Done() stages in
// processContentWithContext) honor the timeout: when the deadline fires, fn
// returns at its next check rather than running to completion in the background.
// The caller still receives the timeout error promptly via the select below,
// independent of how long fn takes to wind down.
//
// On deadline, withTimeout returns the underlying context error
// (context.DeadlineExceeded); callers normalize it to ErrProcessingTimeout. A
// cancellation propagated through parent surfaces as context.Canceled.
//
// Goroutine Safety: To prevent goroutine leaks under heavy load, this function
// limits the maximum number of concurrent timeout goroutines. If the limit is
// exceeded, an error is returned.
func withTimeout[T any](parent context.Context, timeout time.Duration, fn func(context.Context) (T, error)) (T, error) {
	// Atomically check-and-add to prevent TOCTOU race under heavy concurrency.
	// This prevents resource exhaustion when processing many documents with timeouts.
	for {
		current := activeTimeoutGoroutines.Load()
		if current >= maxTimeoutGoroutines {
			var zero T
			return zero, fmt.Errorf("%w: too many concurrent operations (%d active)", ErrProcessingTimeout, current)
		}
		if activeTimeoutGoroutines.CompareAndSwap(current, current+1) {
			break
		}
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	type result struct {
		res T
		err error
	}

	resultChan := make(chan result, 1)

	// Launch the operation in a goroutine
	// (counter already incremented by CAS loop above)
	go func() {
		// Ensure we decrement the counter when done
		defer activeTimeoutGoroutines.Add(-1)

		// Recover from panics inside fn() to prevent crashing the entire process.
		// Without this, a panic in the spawned goroutine bypasses the caller's
		// recover() (which only catches panics in the same goroutine).
		res, err := func() (res T, err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("%w: %v", ErrInternalPanic, r)
				}
			}()
			return fn(ctx)
		}()
		// Try to send the result. If context is done, the result is discarded.
		// This is intentional - we don't want to block if nobody is listening.
		select {
		case resultChan <- result{res: res, err: err}:
			// Result sent successfully
		case <-ctx.Done():
			// Context cancelled/timed out, discard result
		}
	}()

	select {
	case res := <-resultChan:
		return res.res, res.err
	case <-ctx.Done():
		var zero T
		// Return the context error so callers can distinguish a deadline (which
		// they map to ErrProcessingTimeout) from a user cancellation. The
		// goroutine above receives the same ctx and winds down at its next
		// cooperative check; the buffered resultChan keeps it from blocking.
		return zero, ctx.Err()
	}
}

// runWithTimeout executes fn under the processor's configured ProcessingTimeout
// (if set), deriving the deadline from ctx so cooperative cancellation checks
// inside fn honor both the user's context and the timeout. A fired deadline
// surfaces as context.DeadlineExceeded and is normalized to ErrProcessingTimeout;
// a user-initiated cancellation surfaces as context.Canceled and is returned
// unchanged. It is a free function (not a method) because Go does not permit
// type parameters on methods.
func runWithTimeout[T any](p *Processor, ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	if p.config.ProcessingTimeout > 0 {
		result, err := withTimeout(ctx, p.config.ProcessingTimeout, fn)
		if errors.Is(err, context.DeadlineExceeded) {
			err = ErrProcessingTimeout
		}
		return result, err
	}
	return fn(ctx)
}

// isBlankContent checks if content is empty or whitespace-only without allocating.
func (p *Processor) isBlankContent(content string) bool {
	n := len(content)
	if n == 0 {
		return true
	}
	for i := 0; i < n; i++ {
		c := content[i]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return false
		}
	}
	return true
}

// depthStackEntry pairs a node with its depth for iterative depth validation.
type depthStackEntry struct {
	node  *stdxhtml.Node
	depth int
}

// depthStackPool reuses the traversal stack across Extract calls so each
// extraction does not allocate (and repeatedly grow) a fresh slice. The
// profiler attributed ~3.9% of allocations to this stack's growth.
var depthStackPool = sync.Pool{
	New: func() any {
		s := make([]depthStackEntry, 0, 64)
		return &s
	},
}

// validateDepthTraversal validates DOM tree depth using an iterative approach
// to avoid potential stack overflow on deeply nested documents.
// It folds depth validation into a single traversal rather than scanning the
// tree twice (once for depth, once for extraction).
func (p *Processor) validateDepthTraversal(root *stdxhtml.Node, initialDepth int) error {
	// Use iterative approach with explicit stack to avoid stack overflow
	// on deeply nested documents (MaxDepth can be up to 500). The stack is
	// borrowed from a pool and returned when traversal completes, so a
	// typical Extract allocates nothing here.
	stackPtr := depthStackPool.Get().(*[]depthStackEntry)
	stack := (*stackPtr)[:0]
	defer func() {
		*stackPtr = stack
		depthStackPool.Put(stackPtr)
	}()

	stack = append(stack, depthStackEntry{root, initialDepth})

	for len(stack) > 0 {
		entry := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if entry.depth > p.config.MaxDepth {
			return ErrMaxDepthExceeded
		}

		// Add children to stack in reverse order for correct traversal order
		for c := entry.node.FirstChild; c != nil; c = c.NextSibling {
			stack = append(stack, depthStackEntry{c, entry.depth + 1})
		}
	}
	return nil
}

func (p *Processor) extractFromDocument(doc *stdxhtml.Node, htmlContent string) (*Result, error) {
	result := &Result{}
	result.Title = p.extractTitle(doc)

	contentNode := doc
	if p.config.ExtractArticle {
		if article := p.extractArticleNode(doc); article != nil {
			contentNode = article
		}
	}
	contentNode = internal.CleanContentNode(contentNode)

	imageFormat := p.imageFormat
	linkFormat := p.linkFormat

	// Use placeholder path if either image or link format is not "none"
	if imageFormat != "none" || linkFormat != "none" {
		images, links := p.extractImagesAndLinks(contentNode, true, true)

		if p.config.PreserveImages {
			result.Images = images
		}
		if p.config.PreserveLinks {
			result.Links = links
		}

		tb := internal.GetTrackedBuilder()
		imageCounter := 0
		linkCounter := 0
		internal.ExtractTextWithStructureAndImages(contentNode, tb, &imageCounter, &linkCounter, p.config.TableFormat)
		textWithPlaceholders := internal.CleanText(tb.String())
		internal.PutTrackedBuilder(tb)

		// Apply formatters in order: images first, then links
		result.Text = p.formatInlineImages(textWithPlaceholders, images, imageFormat)
		result.Text = p.formatInlineLinks(result.Text, links, linkFormat)
	} else {
		result.Text = p.extractTextContent(contentNode, p.config.TableFormat)

		images, links := p.extractImagesAndLinks(contentNode, p.config.PreserveImages, p.config.PreserveLinks)
		if p.config.PreserveImages {
			result.Images = images
		}
		if p.config.PreserveLinks {
			result.Links = links
		}
	}

	result.WordCount = p.countWords(result.Text)
	result.ReadingTime = p.calculateReadingTime(result.WordCount)

	// Compute the media-reference gate once for both extractors. HasMediaReference
	// scans the whole document, and the video and audio gates evaluate the same
	// content-only condition, so computing it here (only when at least one media
	// type is preserved) halves the scan versus each extractor computing its own.
	if p.config.PreserveVideos || p.config.PreserveAudios {
		canContainMedia := len(htmlContent) > 0 &&
			len(htmlContent) <= maxHTMLForRegex &&
			internal.HasMediaReference(htmlContent)
		// When both media types are preserved (the default), extract them in a
		// single DOM walk instead of two separate WalkNodes traversals.
		if p.config.PreserveVideos && p.config.PreserveAudios {
			result.Videos, result.Audios = p.extractAllMedia(doc, htmlContent, canContainMedia)
		} else if p.config.PreserveVideos {
			result.Videos = p.extractVideos(doc, htmlContent, canContainMedia)
		} else {
			result.Audios = p.extractAudios(doc, htmlContent, canContainMedia)
		}
	}
	return result, nil
}

func (p *Processor) extractTitle(doc *stdxhtml.Node) string {
	if doc == nil {
		return ""
	}
	// Collect the first <title>, <h1>, and <h2> in a single document-order
	// traversal. The previous implementation issued up to three separate
	// FindElementByTag calls (one full tree walk each); documents lacking a
	// <title> or <h1> — common — paid for all three. Priority is unchanged:
	// title → h1 → h2, each preferring a non-empty value.
	var titleNode, h1Node, h2Node *stdxhtml.Node
	internal.WalkNodes(doc, func(n *stdxhtml.Node) bool {
		if n.Type == stdxhtml.ElementNode {
			switch n.Data {
			case "title":
				titleNode = n
			case "h1":
				h1Node = n
			case "h2":
				h2Node = n
			}
		}
		// Keep walking until all three candidates have been located.
		return titleNode == nil || h1Node == nil || h2Node == nil
	})
	if titleNode != nil {
		if title := internal.GetTextContent(titleNode); title != "" {
			return title
		}
	}
	if h1Node != nil {
		if title := internal.GetTextContent(h1Node); title != "" {
			return title
		}
	}
	if h2Node != nil {
		return internal.GetTextContent(h2Node)
	}
	return ""
}

func (p *Processor) extractArticleNode(doc *stdxhtml.Node) *stdxhtml.Node {
	if doc == nil {
		return nil
	}

	// Fast path: the built-in DefaultScorer can find the best article node from a
	// single bottom-up metrics pass (FindBestArticleNode), avoiding both the O(N²)
	// per-candidate subtree re-walk that Score+collectContentMetrics would repeat
	// for every non-inline element, and the map allocation that
	// ScoreArticleCandidates would require. Custom scorers lack that method and
	// fall back to the per-node Score() loop, whose behavior is unchanged.
	if ds, ok := p.scorer.(*internal.DefaultScorer); ok {
		if bestNode := ds.FindBestArticleNode(doc); bestNode != nil {
			return bestNode
		}
	} else {
		candidates := make(map[*stdxhtml.Node]int, initialMapCap)
		internal.WalkNodes(doc, func(n *stdxhtml.Node) bool {
			if n.Type == stdxhtml.ElementNode && !internal.IsInlineElement(n.Data) {
				if score := p.scorer.Score(n); score > 0 {
					candidates[n] = score
				}
			}
			return true
		})
		if bestNode := internal.SelectBestCandidate(candidates); bestNode != nil {
			return bestNode
		}
	}
	return internal.FindElementByTag(doc, "body")
}

func (p *Processor) extractTextContent(node *stdxhtml.Node, tableFormat string) string {
	// A pooled TrackedBuilder retains its capacity across calls (unlike a pooled
	// *strings.Builder, whose Reset nils the buffer), so the document-length text
	// is appended into a buffer that is already large enough after warmup — no
	// per-call growth allocations or the GC churn they cause.
	tb := internal.GetTrackedBuilder()
	internal.ExtractTextWithStructureAndImages(node, tb, nil, nil, tableFormat)
	result := internal.CleanText(tb.String())
	internal.PutTrackedBuilder(tb)
	return result
}

func (p *Processor) formatInlineImages(textWithPlaceholders string, images []ImageInfo, format string) string {
	if len(images) == 0 || format == "placeholder" || format == "none" {
		return textWithPlaceholders
	}

	switch format {
	case "markdown":
		// Build replacements and use direct string replacement
		replacements := make([]string, 0, len(images)*2)
		for i := range images {
			if images[i].Position == 0 {
				continue
			}
			placeholder := "[IMAGE:" + strconv.Itoa(images[i].Position) + "]"
			altText := images[i].Alt
			if altText == "" {
				altText = "Image " + strconv.Itoa(images[i].Position)
			}
			markdown := "![" + escapeMarkdownText(altText) + "](" + images[i].URL + ")"
			replacements = append(replacements, placeholder, markdown)
		}
		if len(replacements) > 0 {
			replacer := strings.NewReplacer(replacements...)
			return replacer.Replace(textWithPlaceholders)
		}

	case "html":
		// Build replacements with pooled builder for HTML tags
		htmlImg := internal.GetBuilder()
		replacements := make([]string, 0, len(images)*2)
		for i := range images {
			if images[i].Position == 0 {
				continue
			}
			placeholder := "[IMAGE:" + strconv.Itoa(images[i].Position) + "]"
			htmlImg.Reset()
			htmlImg.Grow(len(images[i].URL) + len(images[i].Alt) + len(images[i].Width) + len(images[i].Height) + imageHTMLBufExtra)
			htmlImg.WriteString(`<img src="`)
			htmlImg.WriteString(htmlstd.EscapeString(images[i].URL))
			htmlImg.WriteString(`" alt="`)
			htmlImg.WriteString(htmlstd.EscapeString(images[i].Alt))
			htmlImg.WriteString(`"`)
			if images[i].Width != "" {
				htmlImg.WriteString(` width="`)
				htmlImg.WriteString(htmlstd.EscapeString(images[i].Width))
				htmlImg.WriteString(`"`)
			}
			if images[i].Height != "" {
				htmlImg.WriteString(` height="`)
				htmlImg.WriteString(htmlstd.EscapeString(images[i].Height))
				htmlImg.WriteString(`"`)
			}
			htmlImg.WriteString(">")
			replacements = append(replacements, placeholder, htmlImg.String())
		}
		internal.PutBuilder(htmlImg)
		if len(replacements) > 0 {
			replacer := strings.NewReplacer(replacements...)
			return replacer.Replace(textWithPlaceholders)
		}
	}

	return textWithPlaceholders
}

func (p *Processor) formatInlineLinks(textWithPlaceholders string, links []LinkInfo, format string) string {
	if len(links) == 0 || format == "none" {
		return textWithPlaceholders
	}

	// Fast check: any [LINK: present?
	if !strings.Contains(textWithPlaceholders, "[LINK:") {
		return textWithPlaceholders
	}

	// Build lookup map
	linkMap := make(map[int]LinkInfo, len(links))
	for _, link := range links {
		if link.Position > 0 {
			linkMap[link.Position] = link
		}
	}

	// Direct string scanning to avoid regex overhead
	sb := internal.GetBuilder()
	defer internal.PutBuilder(sb)
	sb.Grow(len(textWithPlaceholders))

	i := 0
	n := len(textWithPlaceholders)
	for i < n {
		// Look for [LINK:
		if textWithPlaceholders[i] == '[' && i+6 <= n && textWithPlaceholders[i:i+6] == "[LINK:" {
			// Parse position number
			j := i + 6
			numStart := j
			for j < n && textWithPlaceholders[j] >= '0' && textWithPlaceholders[j] <= '9' {
				j++
			}
			if j > numStart && j < n && textWithPlaceholders[j] == ']' {
				position, err := strconv.Atoi(textWithPlaceholders[numStart:j])
				if err == nil {
					j++ // skip ']'
					// Find [/LINK]
					textStart := j
					endTag := "[/LINK]"
					found := false
					for j <= n-len(endTag) {
						if textWithPlaceholders[j] == '[' && textWithPlaceholders[j:j+7] == endTag {
							// Found the closing tag
							linkText := textWithPlaceholders[textStart:j]

							link, ok := linkMap[position]
							if !ok {
								// Unknown position, output just the text
								sb.WriteString(linkText)
							} else {
								if linkText == "" {
									linkText = "Link " + strconv.Itoa(position)
								}
								switch format {
								case "markdown":
									sb.WriteByte('[')
									sb.WriteString(escapeMarkdownText(linkText))
									sb.WriteString("](")
									sb.WriteString(link.URL)
									sb.WriteByte(')')
								case "html":
									sb.WriteString(`<a href="`)
									sb.WriteString(htmlstd.EscapeString(link.URL))
									sb.WriteString(`"`)
									if link.Title != "" {
										sb.WriteString(` title="`)
										sb.WriteString(htmlstd.EscapeString(link.Title))
										sb.WriteString(`"`)
									}
									sb.WriteString(`>`)
									sb.WriteString(htmlstd.EscapeString(linkText))
									sb.WriteString("</a>")
								default:
									sb.WriteString(linkText)
								}
							}
							j += len(endTag)
							found = true
							break
						}
						j++
					}
					if !found {
						// No closing [/LINK] tag present: write the opening tag
						// literally and resume scanning from textStart so that
						// trailing content is preserved rather than dropped.
						sb.WriteString(textWithPlaceholders[i:textStart])
						j = textStart
					}
					i = j
					continue
				}
			}
			// Failed to parse, write as-is
			sb.WriteByte(textWithPlaceholders[i])
			i++
		} else {
			sb.WriteByte(textWithPlaceholders[i])
			i++
		}
	}

	return sb.String()
}

// extractImagesAndLinks collects images and/or links from node in a single
// document-order traversal, assigning positions with the same semantics the
// previous separate collectors used:
//   - image positions count only <img> with a valid src (parseImageNode accepts),
//     keeping them contiguous and in lockstep with the [IMAGE:n] placeholders the
//     text pass emits (which guards on imgHasValidSrc).
//   - link positions count every <a> (invalid-href anchors still consume a
//     number but are not appended), matching the [LINK:n] placeholders the text
//     pass emits.
//
// The two counters are independent, so one walk yields the same results as the
// previous two. Pass wantImages/wantLinks false to skip (and avoid allocating)
// a kind; the traversal runs at most once.
func (p *Processor) extractImagesAndLinks(node *stdxhtml.Node, wantImages, wantLinks bool) (images []ImageInfo, links []LinkInfo) {
	if wantImages {
		images = make([]ImageInfo, 0, initialSliceCap)
	}
	if wantLinks {
		links = make([]LinkInfo, 0, linksInitialCap)
	}
	if !wantImages && !wantLinks {
		return images, links
	}

	imgPos, linkPos := 0, 0
	internal.WalkNodes(node, func(n *stdxhtml.Node) bool {
		if n.Type != stdxhtml.ElementNode {
			return true
		}
		switch n.Data {
		case "img":
			if !wantImages {
				return true
			}
			img := p.parseImageNode(n, imgPos+1)
			if img.URL == "" {
				return true
			}
			imgPos++
			images = append(images, img)
		case "a":
			if !wantLinks {
				return true
			}
			linkPos++
			link := p.parseLinkNode(n)
			link.Position = linkPos
			if link.URL != "" {
				links = append(links, link)
			}
		}
		return true
	})
	return images, links
}

func (p *Processor) parseImageNode(n *stdxhtml.Node, position int) ImageInfo {
	img := ImageInfo{Position: position}

	for _, attr := range n.Attr {
		switch attr.Key {
		case "src":
			if !internal.IsValidURL(attr.Val) {
				return ImageInfo{}
			}
			img.URL = attr.Val
		case "alt":
			img.Alt = attr.Val
		case "title":
			img.Title = attr.Val
		case "width":
			img.Width = attr.Val
		case "height":
			img.Height = attr.Val
		}
	}

	if img.URL == "" {
		return ImageInfo{}
	}

	img.IsDecorative = img.Alt == ""
	return img
}

func (p *Processor) parseLinkNode(n *stdxhtml.Node) LinkInfo {
	link := LinkInfo{}

	for _, attr := range n.Attr {
		switch attr.Key {
		case "href":
			if !internal.IsValidURL(attr.Val) {
				return LinkInfo{}
			}
			link.URL = attr.Val
		case "title":
			link.Title = attr.Val
		case "rel":
			// Check for nofollow in rel attribute (case-insensitive, no allocation)
			if containsASCIIFold(attr.Val, "nofollow") {
				link.IsNoFollow = true
			}
		}
	}

	if link.URL == "" {
		return LinkInfo{}
	}

	link.Text = internal.GetTextContent(n)
	link.IsExternal = internal.IsExternalURL(link.URL)
	return link
}

func (p *Processor) countWords(text string) int {
	if text == "" {
		return 0
	}
	n := len(text)
	count := 0
	inWord := false
	for i := 0; i < n; i++ {
		c := text[i]
		isSpace := c == ' ' || c == '\t' || c == '\n' || c == '\r'
		if isSpace {
			inWord = false
		} else if !inWord {
			inWord = true
			count++
		}
	}
	return count
}

func (p *Processor) calculateReadingTime(wordCount int) time.Duration {
	if wordCount == 0 {
		return 0
	}
	minutes := float64(wordCount) / wordsPerMinute
	return time.Duration(minutes * float64(time.Minute))
}

// cloneResult returns a deep copy of a Result to prevent data races
// when the same cached entry is returned to multiple callers.
func cloneResult(r *Result) *Result {
	clone := *r
	if r.Images != nil {
		clone.Images = make([]ImageInfo, len(r.Images))
		copy(clone.Images, r.Images)
	}
	if r.Links != nil {
		clone.Links = make([]LinkInfo, len(r.Links))
		copy(clone.Links, r.Links)
	}
	if r.Videos != nil {
		clone.Videos = make([]VideoInfo, len(r.Videos))
		copy(clone.Videos, r.Videos)
	}
	if r.Audios != nil {
		clone.Audios = make([]AudioInfo, len(r.Audios))
		copy(clone.Audios, r.Audios)
	}
	return &clone
}
