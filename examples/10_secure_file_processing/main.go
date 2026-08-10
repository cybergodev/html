package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/cybergodev/html"
)

// This example demonstrates secure file processing with Config.AllowedBaseDir.
//
// When AllowedBaseDir is set, every file read is confined to that directory.
// Containment is verified against the real on-disk path resolved through the
// OS file handle, which closes the gaps a path-only check leaves open
// (symlinks on all platforms; Windows junctions/reparse points that need no
// privilege to create).
func main() {
	fmt.Println("=== Secure File Processing (AllowedBaseDir) ===")
	fmt.Println()

	// Build a temp tree:
	//   <tmp>/allowed/article.html       <- inside the sandbox
	//   <tmp>/allowed/nested/page.html   <- inside (nested subdir)
	//   <tmp>/outside/secret.html        <- outside the sandbox
	base, cleanup, err := makeSandbox()
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	insideA := filepath.Join(base, "article.html")
	insideB := filepath.Join(base, "nested", "page.html")
	outside := filepath.Join(filepath.Dir(base), "outside", "secret.html")

	cfg := html.DefaultConfig()
	cfg.AllowedBaseDir = base
	processor, err := html.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer processor.Close()

	// ============================================================
	// 1. Files inside the sandbox
	// ============================================================
	fmt.Println("1. Files inside the sandbox")
	fmt.Println("---------------------------")
	for _, path := range []string{insideA, insideB} {
		r, err := processor.ExtractFromFile(path)
		if err != nil {
			fmt.Printf("  %s: ERROR %v\n", filepath.Base(path), err)
			continue
		}
		fmt.Printf("  %s: %q (%d words)\n", filepath.Base(path), r.Title, r.WordCount)
	}
	fmt.Println()

	// ============================================================
	// 2. A file outside the sandbox is refused
	// ============================================================
	fmt.Println("2. File outside the sandbox")
	fmt.Println("---------------------------")
	_, err = processor.ExtractFromFile(outside)
	var fileErr *html.FileError
	if errors.As(err, &fileErr) {
		fmt.Printf("  ✓ refused: %v\n", fileErr.FileErr)
		fmt.Printf("    op=%s, reported path=%q (full path is redacted)\n",
			fileErr.Op, fileErr.SafePath())
	}
	fmt.Println()

	// ============================================================
	// 3. Missing file → ErrFileNotFound
	// ============================================================
	fmt.Println("3. Missing file")
	fmt.Println("---------------")
	_, err = processor.ExtractFromFile(filepath.Join(base, "does-not-exist.html"))
	if errors.Is(err, html.ErrFileNotFound) {
		fmt.Println("  ✓ errors.Is(err, html.ErrFileNotFound)")
	}
	fmt.Println()

	// ============================================================
	// 4. Batch files (same sandbox applies to each)
	// ============================================================
	fmt.Println("4. Batch files")
	fmt.Println("--------------")
	batch := processor.ExtractBatchFiles([]string{
		insideA, insideB, // ok
		outside,                             // refused
		filepath.Join(base, "missing.html"), // not found
	})
	fmt.Printf("  %d ok, %d failed of %d\n", batch.Success, batch.Failed, len(batch.Results))
	fmt.Println()

	// ============================================================
	// Summary
	// ============================================================
	fmt.Println("=== Summary ===")
	fmt.Println("• Config.AllowedBaseDir sandboxes all file reads")
	fmt.Println("• Containment resolves through the OS handle — symlinks/junctions can't escape")
	fmt.Println("• Refusals surface as *html.FileError; missing files → html.ErrFileNotFound")
	fmt.Println("• Relative paths containing '..' are rejected as path-traversal attempts")
}

// makeSandbox creates the example directory tree and returns the sandbox base
// path, a cleanup function, and any error. On error the partial tree is removed.
func makeSandbox() (base string, cleanup func(), err error) {
	tmp, err := os.MkdirTemp("", "html-sandbox-*")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { _ = os.RemoveAll(tmp) }

	base = filepath.Join(tmp, "allowed")
	for _, dir := range []string{
		filepath.Join(base, "nested"),
		filepath.Join(tmp, "outside"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			cleanup()
			return "", nil, err
		}
	}

	files := map[string]string{
		filepath.Join(base, "article.html"): "<html><head><title>Sandboxed Article</title></head>" +
			"<body><article><h1>Sandboxed Article</h1><p>Content inside the allowed directory.</p></article></body></html>",
		filepath.Join(base, "nested", "page.html"): "<html><head><title>Nested Page</title></head>" +
			"<body><article><h1>Nested Page</h1><p>Content in a nested subdirectory, still inside the sandbox.</p></article></body></html>",
		filepath.Join(tmp, "outside", "secret.html"): "<html><body><p>This file lives OUTSIDE the sandbox.</p></body></html>",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			cleanup()
			return "", nil, err
		}
	}
	return base, cleanup, nil
}
