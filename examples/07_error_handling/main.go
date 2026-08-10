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
	defer processor.Close()

	_, err = processor.Extract([]byte(""))
	if err != nil {
		fmt.Printf("Empty input error: %v\n", err)
	} else {
		fmt.Printf("Empty input: result.Text is empty (no error)\n")
	}

	// Demonstrate errors.Is() for sentinel errors
	fmt.Println("\nAvailable sentinel errors:")
	fmt.Println("  • html.ErrInputTooLarge    - Input exceeds MaxInputSize")
	fmt.Println("  • html.ErrInvalidHTML      - HTML parsing failed")
	fmt.Println("  • html.ErrProcessorClosed  - Operation on closed processor")
	fmt.Println("  • html.ErrMaxDepthExceeded - Nesting exceeds MaxDepth")
	fmt.Println("  • html.ErrInvalidConfig    - Configuration validation failed")
	fmt.Println("  • html.ErrProcessingTimeout - Processing exceeded timeout")
	fmt.Println("  • html.ErrFileNotFound     - File does not exist")
	fmt.Println("  • html.ErrInvalidFilePath  - Path validation failed")

	// Example: Create processor with small limit to trigger ErrInputTooLarge
	smallCfg := html.DefaultConfig()
	smallCfg.MaxInputSize = 1024 // 1 KB limit
	smallProcessor, err := html.New(smallCfg)
	if err != nil {
		log.Fatal(err)
	}
	defer smallProcessor.Close()

	largeInput := strings.Repeat("<div>", 500) // ~2.5 KB, exceeds 1 KB limit
	_, err = smallProcessor.Extract([]byte(largeInput))
	if errors.Is(err, html.ErrInputTooLarge) {
		fmt.Println("\n✓ Use errors.Is() to check for specific error types")
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

	// Example: Type assertion for more details
	var inputErr *html.InputError
	if errors.As(err, &inputErr) {
		fmt.Printf("\nInputError details:\n")
		fmt.Printf("  Operation: %s\n", inputErr.Op)
		fmt.Printf("  Size: %d\n", inputErr.Size)
		fmt.Printf("  MaxSize: %d\n", inputErr.MaxSize)
	}

	fmt.Println("\n✓ Use errors.As() to access error details")

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
	p.Close() // Close is idempotent - safe to call multiple times
	_, err = p.Extract([]byte("<html></html>"))
	if errors.Is(err, html.ErrProcessorClosed) {
		fmt.Println("✓ ErrProcessorClosed returned after Close()")
	}

	// ============================================================
	// 5. Batch Processing Errors
	// ============================================================
	fmt.Println("\n5. Batch Processing Errors")
	fmt.Println("---------------------------")

	processor2, err := html.New()
	if err != nil {
		log.Fatalf("Failed to create processor: %v", err)
	}
	defer processor2.Close()

	docs := [][]byte{
		[]byte("<html><body><p>Doc 1 - Valid</p></body></html>"),
		[]byte(""), // Empty - not an error, just empty result
		[]byte("<html><body><p>Doc 3 - Valid</p></body></html>"),
	}

	batchResult := processor2.ExtractBatch(docs)

	fmt.Printf("Batch results: %d total, %d success, %d failed\n",
		len(batchResult.Results), batchResult.Success, batchResult.Failed)
	for i, r := range batchResult.Results {
		if r != nil {
			fmt.Printf("  Doc %d: %s (%d words)\n", i+1, r.Title, r.WordCount)
		} else {
			fmt.Printf("  Doc %d: nil result\n", i+1)
		}
	}

	fmt.Println("\n✓ Continue processing on individual failures")
	fmt.Println("✓ Track and report aggregate results")

	// ============================================================
	// 6. File Error Handling
	// ============================================================
	fmt.Println("\n6. File Error Handling")
	fmt.Println("----------------------")

	// Non-existent file
	_, err = processor2.ExtractFromFile("nonexistent.html")
	if err != nil {
		var fileErr *html.FileError
		if errors.As(err, &fileErr) {
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
	fmt.Println("3. Use errors.As() for custom error types")
	fmt.Println("   - InputError, ConfigError, FileError")
	fmt.Println()
	fmt.Println("4. Handle processor lifecycle with defer")
	fmt.Println("   - defer processor.Close()")
	fmt.Println()
	fmt.Println("5. Check batch results for individual failures")
	fmt.Println("   - Results slice may contain nil entries")
}
