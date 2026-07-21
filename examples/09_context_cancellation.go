//go:build examples

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/cybergodev/html"
)

// This example demonstrates cooperative cancellation and timeouts.
//
// Every extraction method has a ...WithContext variant that honors a
// context.Context, and Config.ProcessingTimeout bounds each document
// independently of the caller's context.
//
// The demonstrations use deterministic patterns (a pre-cancelled or
// already-expired context) rather than racing to cancel a live extraction,
// since real extractions finish in microseconds.
func main() {
	fmt.Println("=== Context Cancellation & Timeouts ===")
	fmt.Println()

	processor, err := html.New()
	if err != nil {
		log.Fatal(err)
	}
	defer processor.Close()

	doc := []byte(`<html><body><article><h1>Contexts</h1><p>Cooperative cancellation lets callers stop work early.</p></article></body></html>`)

	// ============================================================
	// 1. Cancellation via context.WithCancel
	// ============================================================
	fmt.Println("1. Cancellation (context.WithCancel)")
	fmt.Println("------------------------------------")

	// A context cancelled before the call returns context.Canceled: the
	// extractor checks ctx.Done() at the start of processing.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	_, err = processor.ExtractWithContext(ctx, doc)
	if errors.Is(err, context.Canceled) {
		fmt.Println("  ✓ pre-cancelled context → context.Canceled")
	} else if err != nil {
		fmt.Printf("  got %v\n", err)
	}

	// An active context extracts normally.
	result, err := processor.ExtractWithContext(context.Background(), doc)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  active context → %q (%d words)\n\n", result.Title, result.WordCount)

	// ============================================================
	// 2. Deadlines via context.WithTimeout
	// ============================================================
	fmt.Println("2. Deadline (context.WithTimeout)")
	fmt.Println("----------------------------------")

	// An already-expired deadline surfaces as context.DeadlineExceeded. The
	// caller's context deadline is returned as-is (not remapped).
	expiredCtx, cancel2 := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel2()
	time.Sleep(time.Millisecond) // ensure the deadline has passed

	_, err = processor.ExtractWithContext(expiredCtx, doc)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		fmt.Println("  ✓ expired deadline → context.DeadlineExceeded")
	case err != nil:
		fmt.Printf("  got %v\n", err)
	}

	// A generous deadline allows normal completion.
	generousCtx, cancel3 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel3()
	if _, err := processor.ExtractWithContext(generousCtx, doc); err != nil {
		log.Fatal(err)
	}
	fmt.Println("  generous timeout → completed normally")
	fmt.Println()

	// ============================================================
	// 3. Per-document ProcessingTimeout (Config)
	// ============================================================
	fmt.Println("3. Per-document ProcessingTimeout")
	fmt.Println("---------------------------------")

	// ProcessingTimeout bounds each extraction independently of the caller's
	// context. Unlike a user-supplied deadline (section 2), a fired
	// ProcessingTimeout is normalized to html.ErrProcessingTimeout, so callers
	// can distinguish a per-document budget exhaustion from their own
	// cancellation. It is configured, not passed per call:
	timeoutCfg := html.DefaultConfig()
	timeoutCfg.ProcessingTimeout = 2 * time.Second
	timeoutProc, err := html.New(timeoutCfg)
	if err != nil {
		log.Fatal(err)
	}
	defer timeoutProc.Close()
	fmt.Printf("  Configured ProcessingTimeout: %v\n", timeoutCfg.ProcessingTimeout)
	fmt.Println("  On a large/slow document a fired budget → html.ErrProcessingTimeout")
	fmt.Println("  (not triggered live: small inputs finish well under budget.)")
	fmt.Println()

	// ============================================================
	// 4. Context-aware Variants
	// ============================================================
	fmt.Println("4. Context-aware Variants")
	fmt.Println("-------------------------")
	fmt.Println("Every extractor has a ...WithContext variant:")
	fmt.Println("  ExtractWithContext(ctx, htmlBytes)")
	fmt.Println("  ExtractTextWithContext(ctx, htmlBytes)")
	fmt.Println("  ExtractToMarkdownWithContext(ctx, htmlBytes)")
	fmt.Println("  ExtractToJSONWithContext(ctx, htmlBytes)")
	fmt.Println("  ExtractAllLinksWithContext(ctx, htmlBytes)")
	fmt.Println("  ExtractFromFileWithContext(ctx, path)")
	fmt.Println("  ExtractBatchWithContext(ctx, docs)  — see 04_performance / 05_http_integration")
	fmt.Println()

	// ============================================================
	// Summary
	// ============================================================
	fmt.Println("=== Summary ===")
	fmt.Println("• Pass a context to enable cooperative cancellation")
	fmt.Println("• User context errors surface as context.Canceled / context.DeadlineExceeded")
	fmt.Println("• Config.ProcessingTimeout bounds each doc → html.ErrProcessingTimeout")
}
