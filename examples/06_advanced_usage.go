//go:build examples

package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/cybergodev/html"
)

// This example demonstrates advanced features: custom scorers, audit logging, and security configuration.
func main() {
	fmt.Println("=== Advanced Features ===")
	fmt.Println()

	// ============================================================
	// 1. Custom Scorer Implementation
	// ============================================================
	fmt.Println("1. Custom Scorer")
	fmt.Println("-----------------")

	scorer := &ArticleScorer{minParagraphLength: 50}
	scorerCfg := html.DefaultConfig()
	scorerCfg.Scorer = scorer
	scorerProcessor, err := html.New(scorerCfg)
	if err != nil {
		log.Fatal(err)
	}
	defer scorerProcessor.Close()

	sampleHTML := "<html><body><nav>Navigation links</nav><article><h1>Article Title</h1><p>This is a substantial paragraph with meaningful content that meets the minimum length requirement.</p></article><aside>Sidebar content</aside></body></html>"

	result, err := scorerProcessor.Extract([]byte(sampleHTML))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Extracted title: %s\n", result.Title)
	fmt.Printf("Content length: %d chars\n\n", len(result.Text))

	// ============================================================
	// 2. Audit System (Security Logging)
	// ============================================================
	fmt.Println("2. Audit System (Security Logging)")
	fmt.Println("-----------------------------------")

	// Configure processor with audit (entries stored internally).
	// Sink is set to io.Discard so audit stays enabled (GetAuditLog() works)
	// without the default LoggerAuditSink dumping JSON to stderr.
	auditCfg := html.DefaultConfig()
	auditCfg.Audit = html.HighSecurityAuditConfig()
	auditCfg.Audit.Enabled = true
	auditCfg.Audit.Sink = html.NewWriterAuditSink(io.Discard)
	auditProcessor, err := html.New(auditCfg)
	if err != nil {
		log.Fatal(err)
	}
	defer auditProcessor.Close()

	// Process potentially dangerous HTML
	dangerousHTML := `
		<html>
			<body>
				<script>alert('xss')</script>
				<a href="javascript:void(0)">Click</a>
				<img src="x.png" onerror="alert(1)">
			</body>
		</html>
	`

	auditProcessor.Extract([]byte(dangerousHTML))

	// Read audit entries (GetAuditLog reads from internal storage, no wait needed)
	auditLog := auditProcessor.GetAuditLog()
	fmt.Printf("Audit entries recorded: %d\n", len(auditLog))

	for _, entry := range auditLog {
		fmt.Printf("  [%s] %s: %s\n", entry.Level, entry.EventType, entry.Message)
	}

	// ============================================================
	// 3. Audit Sinks (Output Destinations)
	// ============================================================
	fmt.Println("\n3. Audit Sinks (Custom Output)")
	fmt.Println("------------------------------")

	// Demo: WriterAuditSink writing to a buffer
	var auditBuf bytes.Buffer
	sinkCfg := html.DefaultConfig()
	sinkCfg.Audit = html.DefaultAuditConfig()
	sinkCfg.Audit.Enabled = true
	sinkCfg.Audit.LogBlockedTags = true
	sinkCfg.Audit.LogBlockedAttrs = true
	sinkCfg.Audit.Sink = html.NewWriterAuditSink(&auditBuf)

	sinkProcessor, err := html.New(sinkCfg)
	if err != nil {
		log.Fatal(err)
	}

	sinkProcessor.Extract([]byte(dangerousHTML))
	sinkProcessor.Close() // flush pending audit writes to sink
	fmt.Printf("WriterAuditSink captured %d bytes of JSON audit logs\n", auditBuf.Len())

	// ChannelAuditSink routes entries to a Go channel for async consumption
	// (e.g. forwarding to an external logging service). Audit writes are
	// synchronous, so once Extract returns the entries are already queued —
	// drain the channel to inspect them, then close the sink.
	channelSink := html.NewChannelAuditSink(64)
	chCfg := html.DefaultConfig()
	chCfg.Audit = html.DefaultAuditConfig()
	chCfg.Audit.Enabled = true
	chCfg.Audit.Sink = channelSink
	chProcessor, err := html.New(chCfg)
	if err != nil {
		log.Fatal(err)
	}
	chProcessor.Extract([]byte(dangerousHTML))
	chProcessor.Close()

	var critical, warning, info int
drain:
	for {
		select {
		case e, ok := <-channelSink.Channel():
			if !ok {
				break drain
			}
			switch e.Level {
			case html.AuditLevelCritical:
				critical++
			case html.AuditLevelWarning:
				warning++
			case html.AuditLevelInfo:
				info++
			}
		default:
			break drain
		}
	}
	channelSink.Close()
	fmt.Printf("ChannelAuditSink drained: %d critical, %d warning, %d info\n", critical, warning, info)

	fmt.Println("\nOther built-in sinks (set via AuditConfig.Sink):")
	fmt.Println("  - LoggerAuditSink   - Writes to standard logger")
	fmt.Println("  - ChannelAuditSink  - Sends to Go channel (for async processing)")
	fmt.Println("  - MultiSink         - Combines multiple sinks")
	fmt.Println("  - FilteredSink      - Filters by custom criteria")
	fmt.Println("  - LevelFilteredSink - Filters by severity level")
	fmt.Println()
	fmt.Println("Or use GetAuditLog() to read entries from internal storage.")

	// ============================================================
	// 4. Security Configuration
	// ============================================================
	fmt.Println("\n4. Security Configuration")
	fmt.Println("-------------------------")

	// High security config
	secureConfig := html.HighSecurityConfig()
	// HighSecurityConfig enables audit by default; point its sink at a buffer
	// (instead of the default stderr logger) so this section stays quiet —
	// audit output is already demonstrated in sections 2 and 3 above.
	var secureAuditBuf bytes.Buffer
	secureConfig.Audit.Sink = html.NewWriterAuditSink(&secureAuditBuf)
	fmt.Println("High Security Config:")
	fmt.Printf("  MaxInputSize: %d MB\n", secureConfig.MaxInputSize/(1024*1024))
	fmt.Printf("  EnableSanitization: %v\n", secureConfig.EnableSanitization)
	fmt.Printf("  MaxDepth: %d\n", secureConfig.MaxDepth)
	fmt.Printf("  ProcessingTimeout: %v\n", secureConfig.ProcessingTimeout)
	fmt.Printf("  Audit.Enabled: %v\n", secureConfig.Audit.Enabled)

	secureProcessor, err := html.New(secureConfig)
	if err != nil {
		log.Fatal(err)
	}
	defer secureProcessor.Close()

	// This will have stricter limits
	secureProcessor.Extract([]byte(dangerousHTML))
	fmt.Printf("  Audit events captured (audit enabled by default): %d\n", len(secureProcessor.GetAuditLog()))

	// ============================================================
	// 5. File Processing Patterns
	// ============================================================
	fmt.Println("\n5. File Processing Patterns")
	fmt.Println("----------------------------")

	fmt.Println("Single file:")
	fmt.Println("  result, err := processor.ExtractFromFile(\"article.html\")")

	fmt.Println("\nBatch files:")
	fmt.Println("  result := processor.ExtractBatchFiles([]string{\"a.html\", \"b.html\"})")

	fmt.Println("\nWith context and timeout:")
	fmt.Println("  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)")
	fmt.Println("  defer cancel()")
	fmt.Println("  result := processor.ExtractBatchFilesWithContext(ctx, paths)")

	// ============================================================
	// 6. Type Aliases for HTML Processing
	// ============================================================
	fmt.Println("\n6. Types for Custom Scorers")
	fmt.Println("---------------------------")
	fmt.Println("The package provides types for implementing custom Scorers:")
	fmt.Println("  • html.ContentNode - Interface for node access (Type, Data, AttrValue, etc.)")
	fmt.Println("  • html.NodeAttr    - Attribute key-value pair")
	fmt.Println()
	fmt.Println("No need to import golang.org/x/net/html directly.")

	// ============================================================
	// Summary
	// ============================================================
	fmt.Println("\n=== Advanced Features Summary ===")
	fmt.Println("1. Custom Scorers: Implement html.Scorer interface for domain-specific extraction")
	fmt.Println("2. Audit System: Monitor security events and blocked content")
	fmt.Println("3. Audit Sinks: Multiple output destinations for audit logs")
	fmt.Println("4. Security Configs: Use HighSecurityConfig() for sensitive data processing")
	fmt.Println("5. File Operations: Single file, batch, and context-aware processing")
	fmt.Println("6. Types for Scorers: ContentNode and NodeAttr for custom implementations")
}

// ArticleScorer is a custom scorer that prioritizes article content.
type ArticleScorer struct {
	minParagraphLength int
}

// Score calculates a relevance score for a content node.
func (s *ArticleScorer) Score(node html.ContentNode) int {
	if node.Type() != "element" {
		return 0
	}

	score := 0

	switch node.Data() {
	case "article":
		score += 100
	case "main":
		score += 90
	case "section":
		score += 50
	case "div":
		score += 10
	case "p":
		textLen := len(getTextContent(node))
		if textLen >= s.minParagraphLength {
			score += 30
		}
	}

	// Check class attributes
	classVal := strings.ToLower(node.AttrValue("class"))
	if classVal != "" {
		if strings.Contains(classVal, "content") ||
			strings.Contains(classVal, "article") ||
			strings.Contains(classVal, "post") {
			score += 20
		}
		if strings.Contains(classVal, "sidebar") ||
			strings.Contains(classVal, "nav") ||
			strings.Contains(classVal, "footer") ||
			strings.Contains(classVal, "ad") {
			score -= 50
		}
	}

	return score
}

// ShouldRemove determines if a node should be removed.
func (s *ArticleScorer) ShouldRemove(node html.ContentNode) bool {
	if node.Type() != "element" {
		return false
	}

	switch node.Data() {
	case "nav", "aside", "footer", "header":
		return true
	}

	classVal := strings.ToLower(node.AttrValue("class"))
	if classVal != "" {
		if strings.Contains(classVal, "ad-") ||
			strings.Contains(classVal, "sponsor") ||
			strings.Contains(classVal, "promo") ||
			strings.Contains(classVal, "sidebar") {
			return true
		}
	}

	return false
}

// getTextContent extracts text content from a node.
func getTextContent(n html.ContentNode) string {
	if n.Type() == "text" {
		return n.Data()
	}
	var result string
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		result += getTextContent(c)
	}
	return result
}
