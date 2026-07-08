package html

import (
	"context"
	"fmt"
	"sync"
)

// BatchResult holds the results of a batch extraction operation.
type BatchResult struct {
	// Results contains the extraction results for each input item.
	// Nil results indicate that the extraction failed or was cancelled.
	Results []*Result
	// Errors contains the error for each input item, if any.
	// The index corresponds to the index in the input slice.
	Errors []error
	// Success is the count of successful extractions.
	Success int
	// Failed is the count of failed extractions.
	Failed int
	// Cancelled is the count of items that were not processed due to context cancellation.
	Cancelled int
}

// extractFunc is a function type for extracting content from a single input.
type extractFunc func() (*Result, error)

// maxBatchSize limits the number of items in a single batch operation to prevent OOM.
// Callers passing more items than this limit receive a BatchResult with an error.
const maxBatchSize = 10000

// uniformErrorBatch creates a BatchResult where every item has the same error.
// The Errors slice length matches n, preserving index correspondence with Results.
func uniformErrorBatch(n int, err error) *BatchResult {
	errs := make([]error, n)
	for i := range errs {
		errs[i] = err
	}
	return &BatchResult{
		Results: make([]*Result, n),
		Errors:  errs,
		Failed:  n,
	}
}

// ============================================================================
// Package-level Batch Convenience Functions
// ============================================================================

// ExtractBatch extracts content from multiple HTML byte slices concurrently.
// This is a convenience function that uses a pooled Processor for efficiency.
// The concurrency level is controlled by the WorkerPoolSize configuration (default: 4).
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
func ExtractBatch(htmlContents [][]byte, cfg ...Config) *BatchResult {
	c, pooled, err := resolveConfig(cfg...)
	if err != nil {
		return uniformErrorBatch(len(htmlContents), err)
	}
	return withProcessorBatch(pooled, c, len(htmlContents), func(p *Processor) *BatchResult {
		return p.ExtractBatch(htmlContents)
	})
}

// ExtractBatchWithContext extracts content from multiple HTML byte slices concurrently with context support.
// This is a convenience function that uses a pooled Processor for efficiency.
// If the context is cancelled, pending extractions are skipped and the BatchResult.Cancelled count is incremented.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
func ExtractBatchWithContext(ctx context.Context, htmlContents [][]byte, cfg ...Config) *BatchResult {
	c, pooled, err := resolveConfig(cfg...)
	if err != nil {
		return uniformErrorBatch(len(htmlContents), err)
	}
	return withProcessorBatch(pooled, c, len(htmlContents), func(p *Processor) *BatchResult {
		return p.ExtractBatchWithContext(ctx, htmlContents)
	})
}

// ExtractBatchFiles extracts content from multiple HTML files concurrently.
// This is a convenience function that uses a pooled Processor for efficiency.
// The concurrency level is controlled by the WorkerPoolSize configuration (default: 4).
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
func ExtractBatchFiles(filePaths []string, cfg ...Config) *BatchResult {
	c, pooled, err := resolveConfig(cfg...)
	if err != nil {
		return uniformErrorBatch(len(filePaths), err)
	}
	return withProcessorBatch(pooled, c, len(filePaths), func(p *Processor) *BatchResult {
		return p.ExtractBatchFiles(filePaths)
	})
}

// ExtractBatchFilesWithContext extracts content from multiple HTML files concurrently with context support.
// This is a convenience function that uses a pooled Processor for efficiency.
// If the context is cancelled, pending extractions are skipped and the BatchResult.Cancelled count is incremented.
//
// An optional Config can be provided to customize extraction behavior.
// If no config is provided, DefaultConfig() is used.
func ExtractBatchFilesWithContext(ctx context.Context, filePaths []string, cfg ...Config) *BatchResult {
	c, pooled, err := resolveConfig(cfg...)
	if err != nil {
		return uniformErrorBatch(len(filePaths), err)
	}
	return withProcessorBatch(pooled, c, len(filePaths), func(p *Processor) *BatchResult {
		return p.ExtractBatchFilesWithContext(ctx, filePaths)
	})
}

// ExtractBatch extracts content from multiple HTML byte slices concurrently.
// The concurrency level is controlled by the WorkerPoolSize configuration (default: 4).
// Each extraction is performed independently with automatic encoding detection.
func (p *Processor) ExtractBatch(htmlContents [][]byte) *BatchResult {
	if br := p.prepareBatch(len(htmlContents)); br != nil {
		return br
	}

	extractors := make([]extractFunc, len(htmlContents))
	for i, content := range htmlContents {
		extractors[i] = p.extractorForBytes(content)
	}

	return p.runBatch(extractors)
}

// ExtractBatchWithContext extracts content from multiple HTML byte slices concurrently with context support.
// The concurrency level is controlled by the WorkerPoolSize configuration (default: 4).
// Each extraction is performed independently with automatic encoding detection.
// If the context is cancelled, pending extractions are skipped and the BatchResult.Cancelled count is incremented.
func (p *Processor) ExtractBatchWithContext(ctx context.Context, htmlContents [][]byte) *BatchResult {
	if br := p.prepareBatch(len(htmlContents)); br != nil {
		return br
	}

	extractors := make([]extractFunc, len(htmlContents))
	for i, content := range htmlContents {
		extractors[i] = p.extractorForBytes(content)
	}

	return p.runBatchWithContext(ctx, extractors)
}

// ExtractBatchFiles extracts content from multiple HTML files concurrently.
// The concurrency level is controlled by the WorkerPoolSize configuration (default: 4).
// Each extraction is performed independently with automatic encoding detection.
func (p *Processor) ExtractBatchFiles(filePaths []string) *BatchResult {
	if br := p.prepareBatch(len(filePaths)); br != nil {
		return br
	}

	extractors := make([]extractFunc, len(filePaths))
	for i, path := range filePaths {
		extractors[i] = p.extractorForFile(path)
	}

	return p.runBatch(extractors)
}

// ExtractBatchFilesWithContext extracts content from multiple HTML files concurrently with context support.
// The concurrency level is controlled by the WorkerPoolSize configuration (default: 4).
// Each extraction is performed independently with automatic encoding detection.
// If the context is cancelled, pending extractions are skipped and the BatchResult.Cancelled count is incremented.
func (p *Processor) ExtractBatchFilesWithContext(ctx context.Context, filePaths []string) *BatchResult {
	if br := p.prepareBatch(len(filePaths)); br != nil {
		return br
	}

	extractors := make([]extractFunc, len(filePaths))
	for i, path := range filePaths {
		extractors[i] = p.extractorForFile(path)
	}

	return p.runBatchWithContext(ctx, extractors)
}

// extractorForBytes creates an extractFunc for byte slice input.
func (p *Processor) extractorForBytes(htmlBytes []byte) extractFunc {
	return func() (*Result, error) {
		return p.Extract(htmlBytes)
	}
}

// extractorForFile creates an extractFunc for file path input.
func (p *Processor) extractorForFile(filePath string) extractFunc {
	return func() (*Result, error) {
		return p.ExtractFromFile(filePath)
	}
}

// runBatch executes a batch of extractions concurrently.
// It is a thin wrapper around runBatchWithContext using context.Background(),
// which never cancels: with a non-cancellable context the <-ctx.Done() cases in
// runBatchWithContext are never selected (the channel is nil and blocks
// forever), yielding identical semantics to a dedicated implementation while
// avoiding duplicated goroutine/recover/result-collection logic.
func (p *Processor) runBatch(extractors []extractFunc) *BatchResult {
	return p.runBatchWithContext(context.Background(), extractors)
}

// runBatchWithContext executes a batch of extractions concurrently with context support.
func (p *Processor) runBatchWithContext(ctx context.Context, extractors []extractFunc) *BatchResult {
	br := &BatchResult{
		Results: make([]*Result, len(extractors)),
		Errors:  make([]error, len(extractors)),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, p.config.WorkerPoolSize)

	for i, extractor := range extractors {
		// Check context before starting each job
		select {
		case <-ctx.Done():
			mu.Lock()
			br.Cancelled++
			br.Errors[i] = ctx.Err()
			mu.Unlock()
			continue
		default:
		}

		// Acquire semaphore before spawning goroutine to bound total goroutines
		// to WorkerPoolSize rather than spawning len(extractors) at once.
		select {
		case <-ctx.Done():
			mu.Lock()
			br.Cancelled++
			br.Errors[i] = ctx.Err()
			mu.Unlock()
			continue
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(idx int, extract extractFunc) {
			defer func() {
				<-sem
				if r := recover(); r != nil {
					mu.Lock()
					br.Errors[idx] = fmt.Errorf("%w: %v", ErrInternalPanic, r)
					br.Failed++
					mu.Unlock()
				}
				wg.Done()
			}()

			// Check context before processing
			select {
			case <-ctx.Done():
				mu.Lock()
				br.Cancelled++
				br.Errors[idx] = ctx.Err()
				mu.Unlock()
				return
			default:
			}

			result, err := extract()
			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				br.Errors[idx] = err
				br.Failed++
			} else {
				br.Results[idx] = result
				br.Success++
			}
		}(i, extractor)
	}

	wg.Wait()
	return br
}

// closedBatchResult creates a BatchResult for a closed processor.
// It is a free function (not a method) because it does not need the receiver —
// the processor is nil/closed and must not be dereferenced.
func closedBatchResult(count int) *BatchResult {
	br := &BatchResult{
		Results: make([]*Result, count),
		Errors:  make([]error, count),
		Failed:  count,
	}
	for i := 0; i < count; i++ {
		br.Errors[i] = ErrProcessorClosed
	}
	return br
}

// prepareBatch runs the input guards shared by all four ExtractBatch* methods
// before any work is dispatched. It returns a non-nil *BatchResult for the
// short-circuit cases — a nil/closed processor (closedBatchResult), a batch
// exceeding maxBatchSize (uniformErrorBatch), or an empty batch — and nil to
// signal "validation passed, build the extractors and proceed". Centralizing
// these guards removes a 13-line preamble that was duplicated verbatim across
// the four methods. Safe to call on a nil *Processor: closedBatchResult is a
// free function and never dereferences the receiver.
func (p *Processor) prepareBatch(count int) *BatchResult {
	if p == nil || p.closed.Load() {
		return closedBatchResult(count)
	}
	if count > maxBatchSize {
		return uniformErrorBatch(count,
			fmt.Errorf("html: batch size %d exceeds maximum %d", count, maxBatchSize))
	}
	if count == 0 {
		return &BatchResult{Results: []*Result{}, Errors: []error{}}
	}
	return nil
}
