package main

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/cybergodev/html"
)

// This example demonstrates error handling patterns.
func main() {
	fmt.Println("=== Error Handling ===")
	fmt.Println()

	// ============================================================
	// 1. Basic Error Checking
	// ============================================================
	fmt.Println("1. Basic Error Checking")
	fmt.Println("-----------------------")

	htmlContent := `<html><body><h1>Hello</h1></body></html>`
	text, err := html.ExtractText([]byte(htmlContent))
	if err != nil {
		log.Fatalf("Failed: %v", err)
	}
	fmt.Printf("Extracted: %s\n\n", text)
	fmt.Println("✓ Always check errors from extraction functions")

	// ============================================================
	// 2. Sentinel Errors
	// ============================================================
	fmt.Println("\n2. Sentinel Errors")
	fmt.Println("-----------------")

	processor, err := html.New()
	if err != nil {
		log.Fatalf("Failed to create processor: %v", err)
	}
	defer func() { _ = processor.Close() }()

	_, err = processor.Extract([]byte(""))
	if err != nil {
		fmt.Printf("Empty input error: %v\n", err)
	} else {
		fmt.Printf("Empty input: result.Text is empty (no error)\n")
	}

	// Demonstrate errors.Is() for sentinel errors
	fmt.Println("\nAvailable sentinel errors:")
	fmt.Println("  • html.ErrInputTooLarge        - Input exceeds MaxInputSize")
	fmt.Println("  • html.ErrInvalidHTML          - HTML parsing failed")
	fmt.Println("  • html.ErrProcessorClosed      - Operation on closed processor")
	fmt.Println("  • html.ErrMaxDepthExceeded     - Nesting exceeds MaxDepth")
	fmt.Println("  • html.ErrInvalidConfig        - Configuration validation failed")
	fmt.Println("  • html.ErrProcessingTimeout    - Processing exceeded timeout")
	fmt.Println("  • html.ErrFileNotFound         - File does not exist")
	fmt.Println("  • html.ErrInvalidFilePath      - Path validation failed")
	fmt.Println("  • html.ErrInternalPanic        - Internal panic was recovered")
	fmt.Println("  • html.ErrMultipleConfigs      - More than one Config argument given")
	fmt.Println("  • html.ErrBatchTooLarge        - Batch exceeds 10,000 items")
	fmt.Println("  • html.ErrTooManyConcurrentOps - Too many concurrent one-shot calls")

	// Example: Create processor with small limit to trigger ErrInputTooLarge
	smallCfg := html.DefaultConfig()
	smallCfg.MaxInputSize = 1024 // 1 KB limit
	smallProcessor, err := html.New(smallCfg)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = smallProcessor.Close() }()

	largeInput := strings.Repeat("<div>", 500) // ~2.5 KB, exceeds 1 KB limit
	_, err = smallProcessor.Extract([]byte(largeInput))
	if errors.Is(err, html.ErrInputTooLarge) {
		fmt.Println("\n✓ Use errors.Is() to check for specific error types")
	}

	// Nesting deeper than Config.MaxDepth (default 500) is rejected outright,
	// guarding against stack overflow on hostile input.
	deepInput := []byte(strings.Repeat("<div>", 501))
	if _, depthErr := processor.Extract(deepInput); errors.Is(depthErr, html.ErrMaxDepthExceeded) {
		fmt.Println("✓ 501-level nesting → ErrMaxDepthExceeded")
	}

	// Variadic-config entry points accept at most one Config.
	if _, cfgErr := html.Extract([]byte("<p>ok</p>"), html.DefaultConfig(), html.DefaultConfig()); errors.Is(cfgErr, html.ErrMultipleConfigs) {
		fmt.Println("✓ Two Config arguments → ErrMultipleConfigs")
	}

	// ============================================================
	// 3. Custom Error Types
	// ============================================================
	fmt.Println("\n3. Custom Error Types")
	fmt.Println("--------------------")

	fmt.Println("Custom error types with additional context:")
	fmt.Println("  • html.InputError  - Op, Size, MaxSize fields")
	fmt.Println("  • html.ConfigError - Field, Value, Message fields")
	fmt.Println("  • html.FileError   - Op, Path, FileErr fields")

	// Example: Type assertion for more details (Go 1.26 errors.AsType)
	if inputErr, ok := errors.AsType[*html.InputError](err); ok {
		fmt.Printf("\nInputError details:\n")
		fmt.Printf("  Operation: %s\n", inputErr.Op)
		fmt.Printf("  Size: %d\n", inputErr.Size)
		fmt.Printf("  MaxSize: %d\n", inputErr.MaxSize)
	}

	// Config errors: New validates the config and returns *ConfigError
	// (wrapping ErrInvalidConfig). A zero-value Config fails because
	// MaxInputSize must be positive — always start from DefaultConfig().
	_, err = html.New(html.Config{})
	if cfgErr, ok := errors.AsType[*html.ConfigError](err); ok {
		fmt.Printf("\nConfigError: field=%s, message=%q\n", cfgErr.Field, cfgErr.Message)
	}
	if errors.Is(err, html.ErrInvalidConfig) {
		fmt.Println("  ✓ errors.Is(err, html.ErrInvalidConfig)")
	}

	fmt.Println("\n✓ Use errors.AsType[T]() to access error details (Go 1.26+)")

	// ============================================================
	// 4. Processor Lifecycle
	// ============================================================
	fmt.Println("\n4. Processor Lifecycle")
	fmt.Println("----------------------")

	p, err := html.New()
	if err != nil {
		log.Fatalf("Failed to create processor: %v", err)
	}

	// Normal pattern: defer Close() for cleanup
	fmt.Println("✓ Always use defer p.Close() for cleanup")

	// Demonstrate: using after close returns error
	_ = p.Close() // Close is idempotent - safe to call multiple times
	_, err = p.Extract([]byte("<html></html>"))
	if errors.Is(err, html.ErrProcessorClosed) {
		fmt.Println("✓ ErrProcessorClosed returned after Close()")
	}

	// ============================================================
	// 5. Batch Processing Errors
	// ============================================================
	fmt.Println("\n5. Batch Processing Errors")
	fmt.Println("---------------------------")

	// Batch items can fail for different reasons: a malformed document, an
	// oversize one, or a file that cannot be read. Set a per-document size
	// limit so an oversize item fails with ErrInputTooLarge instead of being
	// accepted (and instead of the parser rejecting deeply-nested input).
	batchCfg := html.DefaultConfig()
	batchCfg.MaxInputSize = 1024
	processor2, err := html.New(batchCfg)
	if err != nil {
		log.Fatalf("Failed to create processor: %v", err)
	}
	defer func() { _ = processor2.Close() }()

	docs := [][]byte{
		[]byte("<html><body><p>Doc 1 - Valid</p></body></html>"),
		[]byte(strings.Repeat("<div>", 600)), // Oversize → ErrInputTooLarge (exceeds MaxInputSize)
		[]byte("<html><body><p>Doc 3 - Valid</p></body></html>"),
	}

	batchResult := processor2.ExtractBatch(docs)

	fmt.Printf("Batch results: %d total, %d success, %d failed\n",
		len(batchResult.Results), batchResult.Success, batchResult.Failed)
	for i, r := range batchResult.Results {
		if r != nil {
			fmt.Printf("  Doc %d: %s (%d words)\n", i+1, r.Title, r.WordCount)
		} else {
			// Results[i] is nil exactly when the item failed; the matching
			// error lives at Errors[i] (same index), and errors.Is still
			// identifies the cause through the batch wrapper.
			fmt.Printf("  Doc %d: nil result → %v", i+1, batchResult.Errors[i])
			if errors.Is(batchResult.Errors[i], html.ErrInputTooLarge) {
				fmt.Printf("  (errors.Is → ErrInputTooLarge)")
			}
			fmt.Println()
		}
	}

	fmt.Println("\n✓ Continue processing on individual failures")
	fmt.Println("✓ Track and report aggregate results")
	fmt.Println("✓ Results[i] is nil ⇔ Errors[i] is non-nil (same index)")

	// ============================================================
	// 6. File Error Handling
	// ============================================================
	fmt.Println("\n6. File Error Handling")
	fmt.Println("----------------------")

	// Non-existent file
	_, err = processor2.ExtractFromFile("nonexistent.html")
	if err != nil {
		if fileErr, ok := errors.AsType[*html.FileError](err); ok {
			fmt.Printf("FileError:\n")
			fmt.Printf("  Path: %s\n", fileErr.Path)
			fmt.Printf("  Error: %v\n", fileErr.FileErr)
		}

		if errors.Is(err, html.ErrFileNotFound) {
			fmt.Println("\n✓ Use errors.Is() to check for file not found")
		}
	}

	// ============================================================
	// Summary
	// ============================================================
	fmt.Println("\n=== Error Handling Summary ===")
	fmt.Println()
	fmt.Println("1. Always check errors from extraction functions")
	fmt.Println("2. Use errors.Is() for sentinel errors")
	fmt.Println("   - ErrInputTooLarge, ErrInvalidHTML, ErrProcessorClosed, etc.")
	fmt.Println()
	fmt.Println("3. Use errors.AsType[T]() for custom error types")
	fmt.Println("   - InputError, ConfigError, FileError")
	fmt.Println()
	fmt.Println("4. Handle processor lifecycle with defer")
	fmt.Println("   - defer processor.Close()")
	fmt.Println()
	fmt.Println("5. Check batch results for individual failures")
	fmt.Println("   - Results slice may contain nil entries")
}
