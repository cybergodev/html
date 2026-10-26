package html

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cybergodev/html/internal"
	stdxhtml "golang.org/x/net/html"
)

// Timeout goroutine management constants.
// These prevent goroutine leaks when many timeout operations occur.
const (
	// maxTimeoutGoroutines limits concurrent timeout goroutines to prevent
	// resource exhaustion. Goroutine stacks start at a few KB and grow on
	// demand, so 1000 concurrent watchdog goroutines is a modest, bounded cost;
	// the cap exists to keep a burst of timed operations from spawning them
	// without limit. When exceeded, new operations return an error wrapping
	// ErrProcessingTimeout immediately rather than queuing another goroutine.
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
	return withConfig(cfg, func(p *Processor) (*Result, error) {
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
	return withConfig(cfg, func(p *Processor) (*Result, error) {
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
	return withConfig(cfg, func(p *Processor) (string, error) {
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
	return withConfig(cfg, func(p *Processor) (string, error) {
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
	return withConfig(cfg, func(p *Processor) (string, error) {
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
	return withConfig(cfg, func(p *Processor) (string, error) {
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
	return withConfig(cfg, func(p *Processor) (*Result, error) {
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
	return withConfig(cfg, func(p *Processor) (*Result, error) {
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
	if err := p.validateInput(htmlBytes, "Extract"); err != nil {
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
				// A cache hit is an extraction too: record its (near-zero)
				// elapsed time so AverageProcessTime is a true mean over all
				// extractions rather than a hybrid whose numerator excluded
				// hits while its divisor counted them, trending toward zero
				// as the hit ratio rose.
				p.stats.totalProcessTime.Add(int64(time.Since(startTime)))
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
		// Depth violations are recorded at the violation site in
		// validateDepthTraversal, which knows the actual offending depth.
		if p.audit != nil && errors.Is(err, ErrProcessingTimeout) {
			p.audit.RecordTimeout(p.config.ProcessingTimeout)
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

	// Validate DOM depth BEFORE any recursive traversal. The extraction passes
	// below (article-node folding, text extraction, media walks) recurse without
	// an internal depth cap, so a pathologically deep document could exhaust the
	// call stack before those passes rejected it; SanitizeDOM itself is iterative
	// but runs here too, on the same depth-validated tree. Running the O(nodes)
	// iterative depth check first bounds every later recursive pass to MaxDepth.
	// Depth is measured on the raw parsed tree; sanitization only removes nodes,
	// so this is at most marginally stricter than the previous
	// sanitize-then-validate order.
	if err := p.validateDepthTraversal(doc, 0); err != nil {
		return nil, err
	}

	// Collect structured metadata BEFORE sanitization: the sanitizer strips
	// <script> elements, which carry the JSON-LD payloads this pass reads.
	// Disabled by config, in which case every field stays empty and title
	// resolution uses the DOM-only priority.
	var meta internal.Metadata
	if p.config.ExtractMetadata {
		meta = internal.CollectMetadata(doc)
	}

	// Sanitize DOM in-place (avoids render + re-parse overhead). Runs only on a
	// depth-validated tree, so its recursion is bounded by MaxDepth.
	if p.config.EnableSanitization {
		if p.audit != nil && p.config.Audit.Enabled {
			internal.SanitizeDOM(doc, p.audit)
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

	return p.extractFromDocument(doc, originalHTML, meta)
}

// ExtractFromFile extracts content from an HTML file with automatic encoding detection.
// The method automatically detects the character encoding (Windows-1252, UTF-8, GBK, Shift_JIS, etc.)
// from the HTML file and converts it to UTF-8 before processing.
// Use this when you have a file path instead of raw bytes.
//
// Returns the same errors as [Processor.Extract], plus a *FileError wrapping
// ErrFileNotFound or ErrInvalidFilePath, or a path-traversal rejection (which wraps ErrInvalidFilePath) for
// file-access failures.
func (p *Processor) ExtractFromFile(filePath string) (*Result, error) {
	return recoverPanic(func() (*Result, error) {
		// Validate processor state (no input size check for file paths)
		if p == nil || p.closed.Load() {
			return nil, ErrProcessorClosed
		}

		data, err := p.validateAndReadFile(filePath, "ExtractFromFile")
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

		data, err := p.validateAndReadFile(filePath, "ExtractFromFile")
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
			return zero, fmt.Errorf("%w: %d watchdogs active", ErrTooManyConcurrentOps, current)
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
		// Deliver the result unconditionally: resultChan is buffered with
		// capacity 1 and this goroutine is its only sender, so the send never
		// blocks — and racing it against ctx.Done() could discard a fully
		// computed result that the caller's prefer-result select below was
		// about to accept.
		resultChan <- result{res: res, err: err}
	}()

	// Prefer a completed result when both cases are ready: fn finishing in
	// the same instant the deadline fires made the blocking select below pick
	// either case pseudo-randomly, so a successfully computed extraction
	// could be reported as a timeout, run to run. Try the non-blocking
	// receive first.
	select {
	case res := <-resultChan:
		return res.res, res.err
	default:
	}
	select {
	case res := <-resultChan:
		return res.res, res.err
	case <-ctx.Done():
		var zero T
		// Return the context error so callers can distinguish a deadline from
		// a user cancellation. runWithTimeout normalizes it to
		// ErrProcessingTimeout only when the parent context is still live
		// (the processing timeout fired); a user deadline or cancellation
		// passes through unchanged. The goroutine above receives the same ctx
		// and winds down at its next cooperative check; the buffered
		// resultChan keeps it from blocking.
		return zero, ctx.Err()
	}
}

// runWithTimeout executes fn under the processor's configured ProcessingTimeout
// (if set), deriving the deadline from ctx so cooperative cancellation checks
// inside fn honor both the user's context and the timeout. The derived context
// expires with context.DeadlineExceeded both when the processing timeout
// elapses and when the caller's own (earlier) deadline does; only the former is
// normalized to ErrProcessingTimeout — a user deadline or cancellation is
// returned unchanged, matching the documented contract of the *WithContext
// entry points and keeping the audit log from misattributing a user
// cancellation to the library's timeout. It is a free function (not a method)
// because Go does not permit type parameters on methods.
func runWithTimeout[T any](p *Processor, ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	if p.config.ProcessingTimeout > 0 {
		result, err := withTimeout(ctx, p.config.ProcessingTimeout, fn)
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			// The parent context is still live, so the deadline that fired
			// is the processing timeout itself.
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
	for i := range n {
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
			// Record the actual offending depth (not a MaxDepth+1 stand-in)
			// so audit consumers can gauge how hostile the document was.
			if p.audit != nil {
				p.audit.RecordDepthViolation(entry.depth, p.config.MaxDepth)
			}
			return ErrMaxDepthExceeded
		}

		// Add children to stack in reverse order for correct traversal order
		for c := entry.node.FirstChild; c != nil; c = c.NextSibling {
			stack = append(stack, depthStackEntry{c, entry.depth + 1})
		}
	}
	return nil
}

// Extraction-method values reported by Result.ExtractionMethod.
const (
	// extractionMethodArticle marks an extraction whose root is the
	// main-content node the scorer selected.
	extractionMethodArticle = "article"
	// extractionMethodBody marks a full-<body> extraction: either no candidate
	// scored above zero or ExtractArticle is disabled.
	extractionMethodBody = "body"
	// extractionMethodListing marks a recognized listing/index page (a card
	// container of several link-dense sibling <article> elements): the
	// extracted Text is the whole card list, not a single article.
	extractionMethodListing = "listing"
)

func (p *Processor) extractFromDocument(doc *stdxhtml.Node, htmlContent string, meta internal.Metadata) (*Result, error) {
	result := &Result{}

	// Identify the content node first: the title resolution below prefers an
	// h1 inside it over a page-level h1. Listing pages (card feeds) are
	// recognized before article selection — every card is its own <article>
	// and would otherwise let the scorer pick a single card — and only when
	// the card list dominates the page's visible text, so an article page
	// with a stray article-card section stays on the article path.
	//
	// bodyTextLen feeds the listing dominance guard and the confidence body
	// ratio, and both read it only on the listing/article paths — so it is
	// computed lazily (bodyTextLength is a full body-subtree walk) rather than
	// unconditionally: a body-fallback extraction skips the pass entirely.
	var bodyTextLen int
	contentNode := doc
	method := extractionMethodBody
	if p.config.ExtractArticle {
		if listing := internal.FindListingContainer(doc); listing != nil {
			bodyTextLen = p.bodyTextLength(doc)
			if internal.TextContentLength(listing)*2 >= bodyTextLen {
				contentNode = listing
				method = extractionMethodListing
			} else if article, m := p.extractArticleNode(doc); article != nil {
				contentNode = article
				method = m
			}
		} else if article, m := p.extractArticleNode(doc); article != nil {
			contentNode = article
			method = m
			bodyTextLen = p.bodyTextLength(doc)
		}
	}
	result.ExtractionMethod = method

	result.Title = p.resolveTitle(doc, contentNode, method, meta)
	if p.config.ExtractMetadata {
		result.Author = meta.Author
		result.PublishedTime = parsePublishedTime(meta.PublishedTime)
		result.Description = meta.Description
		result.SiteName = meta.SiteName
		result.CanonicalURL = meta.CanonicalURL
	}

	contentNode = internal.CleanContentNode(contentNode)

	imageFormat := p.imageFormat
	linkFormat := p.linkFormat

	// A "none" format has no formatter, so it needs neither the placeholder
	// tokens nor the resource list behind them.
	needsImageTokens := imageFormat != "none"
	needsLinkTokens := linkFormat != "none"

	// Collect images and links once, in a single traversal. A list is needed
	// when the caller asked for it (PreserveImages/PreserveLinks) or when a
	// formatter must substitute placeholders with it; requesting a list solely
	// because a formatter consumes it would otherwise drop the entries the
	// caller asked for when that format is "none".
	images, links := p.extractImagesAndLinks(contentNode,
		p.config.PreserveImages || needsImageTokens,
		p.config.PreserveLinks || needsLinkTokens,
		false)
	if p.config.PreserveImages {
		result.Images = images
	}
	if p.config.PreserveLinks {
		result.Links = links
	}

	if needsImageTokens || needsLinkTokens {
		tb := internal.GetTrackedBuilder()
		var imageCounter, linkCounter int
		var imgCounter, linkCounterPtr *int
		// Only emit the placeholder kinds a formatter will consume: a "none"
		// format has no formatter, so its tokens would survive both formatting
		// passes verbatim and leak into Text as literal [IMAGE:n]/[LINK:n] text
		// when just one of the two formats is enabled. ("placeholder" keeps
		// [IMAGE:n] visible by design and still needs the numbering.) Positions
		// in result.Images/result.Links come from the independent counters in
		// extractImagesAndLinks and are unaffected by this.
		if needsImageTokens {
			imgCounter = &imageCounter
		}
		if needsLinkTokens {
			linkCounterPtr = &linkCounter
		}
		internal.ExtractTextWithStructureAndImages(contentNode, tb, imgCounter, linkCounterPtr, p.config.TableFormat)
		// CleanTextBytes reads the builder's buffer directly, skipping the
		// tb.String() intermediate copy the string entry point would make.
		text := internal.CleanTextBytes(tb.Bytes())
		internal.PutTrackedBuilder(tb)

		// Apply formatters in order: links first, then images. Link labels
		// may carry [IMAGE:n] placeholders through (see formatInlineLinks),
		// and the image pass nests their markdown inside the constructs; a
		// "none" formatter returns its input unchanged. The substitution can
		// collapse a placeholder to nothing (an empty-label anchor with no
		// usable href), leaving the two spaces that flanked the token
		// adjacent — re-normalize so Text carries the same single-space form
		// the plain path produces (safe now that the table renderer emits no
		// column-width padding for it to fold).
		result.Text = internal.CleanTextBytes([]byte(
			p.formatInlineImages(p.formatInlineLinks(text, links, images, linkFormat), images, imageFormat)))
	} else {
		result.Text = p.extractTextContent(contentNode, p.config.TableFormat)
	}

	var cjkChars int
	result.WordCount, cjkChars = p.countWords(result.Text)
	result.ReadingTime = p.calculateReadingTime(result.WordCount, cjkChars)
	result.Confidence = computeExtractionConfidence(method, result.Text, bodyTextLen)

	// HasMediaReference is a necessary condition for the raw-HTML media scans,
	// and it is the same content-only test for both media kinds, so it is
	// computed once here rather than per extractor.
	if p.config.PreserveVideos || p.config.PreserveAudios {
		canContainMedia := len(htmlContent) > 0 &&
			len(htmlContent) <= maxHTMLForRegex &&
			internal.HasMediaReference(htmlContent)
		// One call handles all three preserve combinations; the want flags keep
		// a disabled kind from costing a walk branch or a raw-HTML scan.
		videos, audios := p.extractMedia(doc, htmlContent, canContainMedia,
			p.config.PreserveVideos, p.config.PreserveAudios)
		result.Videos = videos
		result.Audios = audios
	}
	return result, nil
}

// bodyTextLength returns the normalized visible-text length of doc's <body>
// subtree, computed on the sanitized tree before cleaning so the confidence
// ratio measures the article's share of all visible page text. One pooled-buffer
// walk (TextContentLength); extractFromDocument calls it lazily because only
// the listing-dominance guard and article-method confidence read the value.
func (p *Processor) bodyTextLength(doc *stdxhtml.Node) int {
	if body := internal.FindElementByTag(doc, "body"); body != nil {
		return internal.TextContentLength(body)
	}
	return 0
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
