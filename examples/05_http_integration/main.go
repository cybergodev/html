package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/cybergodev/html"
)

// This example demonstrates HTTP integration patterns.
// Learn how to fetch web pages and extract content efficiently.
func main() {
	fmt.Println("=== HTTP Integration ===")
	fmt.Println()

	// Create mock server
	server := createMockServer()
	defer server.Close()

	processor, err := html.New()
	if err != nil {
		log.Fatal(err)
	}
	defer processor.Close()

	// ============================================================
	// 1. Fetch and Extract
	// ============================================================
	fmt.Println("1. Fetch and Extract")
	fmt.Println("-------------------")

	url := server.URL + "/article"
	content, err := fetchURL(url)
	if err != nil {
		log.Fatalf("Fetch error: %v", err)
	}

	result, err := html.Extract(content)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("URL: %s\n", url)
	fmt.Printf("Title: %s\n", result.Title)
	fmt.Printf("Words: %d\n\n", result.WordCount)

	// ============================================================
	// 2. Process Multiple URLs Concurrently
	// ============================================================
	fmt.Println("2. Concurrent URL Processing")
	fmt.Println("-----------------------------")

	urls := []string{
		server.URL + "/article",
		server.URL + "/blog",
		server.URL + "/docs",
	}

	start := time.Now()
	results := processURLsConcurrently(processor, urls)
	duration := time.Since(start)

	fmt.Printf("Processed %d URLs in %v\n", len(results), duration)
	for _, r := range results {
		if r.Error != nil {
			fmt.Printf("  • %s: Error - %v\n", r.URL, r.Error)
		} else {
			fmt.Printf("  • %s: %s (%d words)\n", r.URL, r.Result.Title, r.Result.WordCount)
		}
	}

	// ============================================================
	// 3. Configure HTTP Client
	// ============================================================
	fmt.Println("\n3. Configure HTTP Client")
	fmt.Println("-------------------------")

	httpClient := createHTTPClient(10 * time.Second)
	fmt.Println("HTTP Client Configuration:")
	fmt.Println("  Timeout: 10 seconds")
	fmt.Println("  Max Idle Conns: 10")

	resp, err := httpClient.Get(server.URL + "/article")
	if err != nil {
		log.Printf("Request failed: %v", err)
	} else {
		defer resp.Body.Close()
		fmt.Printf("\nResponse Status: %s\n", resp.Status)
		body, _ := io.ReadAll(resp.Body) // best-effort: mock server always returns valid body
		result, _ := html.Extract(body)  // best-effort: demo context
		fmt.Printf("Extracted: %s (%d words)\n", result.Title, result.WordCount)
	}

	// ============================================================
	// 4. Batch with Context (Timeout/Cancellation)
	// ============================================================
	fmt.Println("\n4. Batch Processing with Context")
	fmt.Println("----------------------------------")

	pages := make([][]byte, 3)
	for i, path := range []string{"/article", "/blog", "/docs"} {
		content, _ := fetchURL(server.URL + path) // best-effort: mock server always returns valid bodies
		pages[i] = content
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	batchResult := processor.ExtractBatchWithContext(ctx, pages)
	fmt.Printf("Success: %d, Failed: %d, Cancelled: %d\n",
		batchResult.Success, batchResult.Failed, batchResult.Cancelled)

	// ============================================================
	// Summary
	// ============================================================
	fmt.Println("\n=== Best Practices ===")
	fmt.Println("• Set HTTP timeouts (10-30 seconds)")
	fmt.Println("• Reuse processor across requests")
	fmt.Println("• Use context for cancellation")
	fmt.Println("• Implement retry logic for transient failures")
	fmt.Println("• Validate responses before processing")
}

// fetchURL fetches content from a URL
func fetchURL(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	return io.ReadAll(resp.Body)
}

// processURLsConcurrently processes multiple URLs concurrently
func processURLsConcurrently(processor *html.Processor, urls []string) []URLResult {
	var wg sync.WaitGroup
	results := make([]URLResult, len(urls))

	for i, url := range urls {
		wg.Add(1)
		go func(idx int, u string) {
			defer wg.Done()

			content, err := fetchURL(u)
			if err != nil {
				results[idx] = URLResult{URL: u, Error: err}
				return
			}

			result, err := processor.Extract(content)
			if err != nil {
				results[idx] = URLResult{URL: u, Error: err}
				return
			}

			results[idx] = URLResult{URL: u, Result: result}
		}(i, url)
	}

	wg.Wait()
	return results
}

// createHTTPClient creates an optimized HTTP client
func createHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  false,
		},
	}
}

// URLResult represents the result of processing a URL
type URLResult struct {
	URL    string
	Result *html.Result
	Error  error
}

// createMockServer creates a test server with sample HTML content
func createMockServer() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/article", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`
			<html>
				<head><title>Go Article</title></head>
				<body>
					<article>
						<h1>Understanding Go Concurrency</h1>
						<p>Go makes concurrent programming easy with goroutines and channels.</p>
					</article>
				</body>
			</html>
		`))
	})

	mux.HandleFunc("/blog", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`
			<html>
				<head><title>Blog Post</title></head>
				<body>
					<article>
						<h1>Web Scraping Best Practices</h1>
						<p>Learn how to scrape web content responsibly.</p>
					</article>
				</body>
			</html>
		`))
	})

	mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`
			<html>
				<head><title>Documentation</title></head>
				<body>
					<article>
						<h1>API Reference</h1>
						<p>This document describes the API endpoints.</p>
					</article>
				</body>
			</html>
		`))
	})

	return httptest.NewServer(mux)
}
