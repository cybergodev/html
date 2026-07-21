//go:build examples

package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/cybergodev/html"
)

// This example demonstrates link extraction, URL resolution, and media extraction.
// Learn how to extract links, images, videos, and audio from HTML content.
//
// NOTE: For brevity, extraction errors are elided in this example;
// see 07_error_handling.go for proper error-handling patterns.
func main() {
	fmt.Println("=== Links & Media Extraction ===")
	fmt.Println()

	htmlContent := `
		<html>
			<head>
				<base href="https://example.com/blog/">
				<link rel="stylesheet" href="/assets/style.css">
			</head>
			<body>
				<nav>
					<a href="/">Home</a>
					<a href="/about">About</a>
					<a href="https://golang.org">Official Site</a>
				</nav>
				<article>
					<h1>Go Programming</h1>
					<img src="hero.jpg" alt="Hero Image" width="800" height="400">
					<p>Read more at <a href="tutorial.html">Tutorial</a></p>
					<video src="demo.mp4" poster="thumb.jpg" width="640">
						<source src="demo.webm" type="video/webm">
					</video>
					<audio src="podcast.mp3" controls></audio>
				</article>
			</body>
		</html>
	`

	processor, err := html.New()
	if err != nil {
		log.Fatal(err)
	}
	defer processor.Close()

	// ============================================================
	// PART 1: Link extraction with URL resolution
	// ============================================================
	fmt.Println("Part 1: Link Extraction")
	fmt.Println("-----------------------")

	links, err := processor.ExtractAllLinks([]byte(htmlContent))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Found %d links:\n", len(links))

	// Categorize links
	for _, link := range links {
		icon := "•"
		switch link.Type {
		case "css":
			icon = "🎨"
		case "link":
			// Distinguish external links (https and not our base host)
			// from internal ones; otherwise the 🔗 case below never fires.
			if strings.HasPrefix(link.URL, "https://") && !strings.Contains(link.URL, "example.com") {
				icon = "🔗"
			} else {
				icon = "📄"
			}
		}
		fmt.Printf("  %s %s [%s]\n", icon, link.URL, link.Type)
	}

	// ============================================================
	// PART 2: Filter links by type using custom config
	// ============================================================
	fmt.Println("\nPart 2: Filter Links by Type")
	fmt.Println("----------------------------")

	filterCfg := html.DefaultConfig()
	filterCfg.IncludeCSS = false
	filterCfg.IncludeJS = false
	filterProcessor, err := html.New(filterCfg)
	if err != nil {
		log.Fatal(err)
	}
	defer filterProcessor.Close()

	filteredLinks, _ := filterProcessor.ExtractAllLinks([]byte(htmlContent))
	fmt.Printf("Content links only (CSS/JS excluded): %d\n", len(filteredLinks))

	// Internal links only: IncludeExternalLinks=false drops every link whose
	// host differs from the (detected) BaseURL. The fixture's one external
	// link — https://golang.org — disappears from the result.
	internalCfg := html.DefaultConfig()
	internalCfg.IncludeExternalLinks = false
	internalProcessor, err := html.New(internalCfg)
	if err != nil {
		log.Fatal(err)
	}
	defer internalProcessor.Close()

	internalLinks, _ := internalProcessor.ExtractAllLinks([]byte(htmlContent))
	hasExternal := false
	for _, l := range internalLinks {
		if strings.Contains(l.URL, "golang.org") {
			hasExternal = true
		}
	}
	fmt.Printf("Internal links only (external excluded): %d (golang.org present: %v)\n",
		len(internalLinks), hasExternal)

	// ============================================================
	// PART 3: Image extraction
	// ============================================================
	fmt.Println("\nPart 3: Image Extraction")
	fmt.Println("------------------------")

	result, _ := processor.Extract([]byte(htmlContent))

	fmt.Printf("Found %d images:\n", len(result.Images))
	for i, img := range result.Images {
		fmt.Printf("  %d. %s\n", i+1, img.URL)
		fmt.Printf("     Alt: %q, Size: %sx%s\n", img.Alt, img.Width, img.Height)
		if img.Alt == "" {
			fmt.Printf("     ⚠ Missing alt text (bad for accessibility)\n")
		}
	}

	// ============================================================
	// PART 4: Video extraction
	// ============================================================
	fmt.Println("\nPart 4: Video Extraction")
	fmt.Println("------------------------")

	fmt.Printf("Found %d videos:\n", len(result.Videos))
	for i, vid := range result.Videos {
		fmt.Printf("  %d. %s\n", i+1, vid.URL)
		vidType := vid.Type
		if vidType == "" {
			vidType = "video"
		}
		fmt.Printf("     Type: %s, Poster: %s\n", vidType, vid.Poster)
		if vid.Width != "" || vid.Height != "" {
			fmt.Printf("     Size: %sx%s\n", vid.Width, vid.Height)
		}
	}

	// ============================================================
	// PART 5: Audio extraction
	// ============================================================
	fmt.Println("\nPart 5: Audio Extraction")
	fmt.Println("------------------------")

	fmt.Printf("Found %d audio files:\n", len(result.Audios))
	for i, aud := range result.Audios {
		fmt.Printf("  %d. %s\n", i+1, aud.URL)
		audType := aud.Type
		if audType == "" {
			audType = "audio"
		}
		fmt.Printf("     Type: %s\n", audType)
	}

	// ============================================================
	// PART 6: Selective media extraction
	// ============================================================
	fmt.Println("\nPart 6: Selective Media Extraction")
	fmt.Println("----------------------------------")

	// Images only (disable video and audio)
	imageOnlyCfg := html.DefaultConfig()
	imageOnlyCfg.PreserveVideos = false
	imageOnlyCfg.PreserveAudios = false
	imageProcessor, err := html.New(imageOnlyCfg)
	if err != nil {
		log.Fatal(err)
	}
	defer imageProcessor.Close()

	imgResult, _ := imageProcessor.Extract([]byte(htmlContent))
	fmt.Printf("Images only: %d images, %d videos, %d audio\n",
		len(imgResult.Images), len(imgResult.Videos), len(imgResult.Audios))

	// Video and audio only (disable images)
	mediaOnlyCfg := html.DefaultConfig()
	mediaOnlyCfg.PreserveImages = false
	mediaProcessor, err := html.New(mediaOnlyCfg)
	if err != nil {
		log.Fatal(err)
	}
	defer mediaProcessor.Close()

	mediaResult, _ := mediaProcessor.Extract([]byte(htmlContent))
	fmt.Printf("Media only: %d images, %d videos, %d audio\n",
		len(mediaResult.Images), len(mediaResult.Videos), len(mediaResult.Audios))

	// ============================================================
	// Summary
	// ============================================================
	fmt.Println("\n=== Quick Reference ===")
	fmt.Println("Link filtering (in Config):")
	fmt.Println("  IncludeCSS, IncludeJS, IncludeImages, IncludeVideos, IncludeAudios")
	fmt.Println("  IncludeContentLinks, IncludeExternalLinks, IncludeIcons")
	fmt.Println()
	fmt.Println("URL resolution:")
	fmt.Println("  ResolveRelativeURLs=true, BaseURL=\"https://example.com/\"")
	fmt.Println()
	fmt.Println("Media extraction:")
	fmt.Println("  PreserveImages, PreserveVideos, PreserveAudios")
}
