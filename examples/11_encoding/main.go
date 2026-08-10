package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/cybergodev/html"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// This example demonstrates automatic character-encoding detection and the
// explicit Config.Encoding override. The library detects the declared charset
// and converts to UTF-8 before extraction, so titles and body text come back
// correctly decoded regardless of the source encoding.
//
// The non-UTF-8 fixtures below are built with golang.org/x/text — already a
// dependency of this library — so the example needs no extra modules.
func main() {
	fmt.Println("=== Character Encoding ===")
	fmt.Println()

	// A GBK-encoded page that declares its charset via <meta charset>.
	gbkUTF8 := `<html><head><meta charset="gbk"><title>测试标题</title></head>` +
		`<body><article><h1>测试标题</h1><p>你好世界，这是一段中文内容。</p></article></body></html>`
	gbkBytes, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(gbkUTF8))
	if err != nil {
		log.Fatal(err)
	}

	// A Windows-1252-encoded page (Western European) with é and €.
	winUTF8 := `<html><head><meta charset="windows-1252"><title>Café Menu</title></head>` +
		`<body><article><h1>Café Menu</h1><p>Price: 100 €. Résumé available.</p></article></body></html>`
	winBytes, err := charmap.Windows1252.NewEncoder().Bytes([]byte(winUTF8))
	if err != nil {
		log.Fatal(err)
	}

	// ============================================================
	// 1. Auto-detection (charset declared in the document)
	// ============================================================
	fmt.Println("1. Auto-detection (declared charset)")
	fmt.Println("-------------------------------------")

	r, err := html.Extract(gbkBytes)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  GBK          → title=%q, text=%q\n", r.Title, compact(r.Text))

	r, err = html.Extract(winBytes)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  Windows-1252 → title=%q, text=%q\n", r.Title, compact(r.Text))
	fmt.Println()

	// ============================================================
	// 2. Explicit override (Config.Encoding)
	// ============================================================
	fmt.Println("2. Explicit override (Config.Encoding)")
	fmt.Println("---------------------------------------")

	// When the document omits (or has a wrong) charset declaration, force the
	// encoding via Config.Encoding. This takes precedence over auto-detection.
	noMetaUTF8 := `<html><head><title>测试</title></head><body><article><h1>测试</h1><p>你好世界</p></article></body></html>`
	noMetaGBK, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(noMetaUTF8))
	if err != nil {
		log.Fatal(err)
	}

	cfg := html.DefaultConfig()
	cfg.Encoding = "gbk"
	r, err = html.Extract(noMetaGBK, cfg)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  Encoding=\"gbk\" (no meta) → title=%q, text=%q\n", r.Title, compact(r.Text))
	fmt.Println()

	// ============================================================
	// 3. Supported encodings
	// ============================================================
	fmt.Println("3. Supported encodings")
	fmt.Println("----------------------")
	fmt.Println("  Auto-detected from <meta charset>, or forced via Config.Encoding:")
	fmt.Println("  UTF-8 / UTF-16, Windows-1250/1251/1252, ISO-8859-1..16,")
	fmt.Println("  GBK, Big5, Shift_JIS, EUC-JP, ISO-2022-JP, EUC-KR")
	fmt.Println()

	// ============================================================
	// Summary
	// ============================================================
	fmt.Println("=== Summary ===")
	fmt.Println("• Declare charset via <meta charset> for reliable auto-detection")
	fmt.Println("• Force an encoding with Config.Encoding when the declaration is missing or wrong")
}

// compact collapses runs of whitespace into single spaces for tidy display.
func compact(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
