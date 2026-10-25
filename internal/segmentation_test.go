package internal

import (
	"slices"
	"testing"

	"golang.org/x/net/html"
)

// kindsOf extracts the ordered kind strings from specs for table comparison.
func kindsOf(specs []PageSectionSpec) []string {
	kinds := make([]string, len(specs))
	for i := range specs {
		kinds[i] = specs[i].Kind
	}
	return kinds
}

// chromeFlagsOf extracts the ordered chrome flags from specs.
func chromeFlagsOf(specs []PageSectionSpec) []bool {
	flags := make([]bool, len(specs))
	for i := range specs {
		flags[i] = specs[i].Chrome
	}
	return flags
}

// countKind counts specs of the given kind.
func countKind(specs []PageSectionSpec, kind string) int {
	n := 0
	for i := range specs {
		if specs[i].Kind == kind {
			n++
		}
	}
	return n
}

// TestSegmentPage table-drives the classification scenarios that all shared
// the parse → SegmentPage → kindsOf skeleton (the former
// TestSegmentPageSemanticLandmarks / TestSegmentPageClassTokens /
// TestSegmentPageChromeTokenOutranksMainToken /
// TestSegmentPageWrapperGuardDescendsLayoutShell /
// TestSegmentPageResidualClassification / TestSegmentPageFallbackLargestResidual /
// TestSegmentPageDropsRemovalClassedResiduals / TestSegmentPageARIALandmarks /
// TestSegmentPageFormWrappingWholePage). Scenarios whose findArticle callback
// must locate a fixture node stay as dedicated tests below.
func TestSegmentPage(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name       string
		doc        string
		wantKinds  []string
		wantChrome []bool
		check      func(t *testing.T, specs []PageSectionSpec)
	}{
		{
			name: "semantic landmarks",
			doc: `<html><body>
				<header><nav><a href="/">Home</a><a href="/news">News</a></nav></header>
				<main><article><h1>Headline</h1><p>Body prose, long enough to read as content.</p></article></main>
				<aside><p>Sidebar note.</p></aside>
				<footer><p>© 2026 Example</p></footer>
			</body></html>`,
			wantKinds:  []string{SectionHeader, SectionMain, SectionAside, SectionFooter},
			wantChrome: []bool{true, false, true, true},
			check: func(t *testing.T, specs []PageSectionSpec) {
				// The nested <nav> must not appear as its own section: it is
				// part of the header region (outermost landmark wins).
				if n := countKind(specs, SectionNavigation); n != 0 {
					t.Errorf("nested nav produced %d separate navigation sections, want 0", n)
				}
			},
		},
		{
			name: "class tokens",
			doc: `<html><body>
				<div class="top-nav"><a href="/">Home</a><a href="/a">A</a><a href="/b">B</a></div>
				<div id="site-main"><p>The article body lives here, with enough words to dominate the page text.</p></div>
				<div class="site-footer"><p>© 2026</p></div>
			</body></html>`,
			wantKinds: []string{SectionNavigation, SectionMain, SectionFooter},
		},
		{
			// "main-nav" carries word-bounded "main" AND "nav"; the
			// navigation reading must win.
			name: "chrome token outranks main token",
			doc: `<html><body>
				<div class="main-nav"><a href="/">Home</a><a href="/x">X</a><a href="/y">Y</a></div>
				<main><p>Article prose.</p></main>
			</body></html>`,
			wantKinds: []string{SectionNavigation, SectionMain},
		},
		{
			// A wrapper carrying the "main" token but enclosing the whole
			// layout (header + main + footer) is a CSS shell, not the main
			// region: the containsLandmark guard must demote it to a wrapper
			// and descend.
			name: "wrapper guard descends layout shell",
			doc: `<html><body>
				<div class="page-main-content">
					<div class="site-header"><a href="/">Home</a></div>
					<main><p>Real article prose, reasonably long.</p></main>
					<div class="site-footer"><p>© 2026</p></div>
				</div>
			</body></html>`,
			wantKinds: []string{SectionHeader, SectionMain, SectionFooter},
			check: func(t *testing.T, specs []PageSectionSpec) {
				if len(specs) != 3 || specs[0].Node.Data != "div" {
					t.Errorf("header spec node = %v, want the site-header div", specs[0].Node)
				}
			},
		},
		{
			name: "residual classification",
			doc: `<html><body>
				<div><p>Intro blurb before the article.</p></div>
				<nav><a href="/">Home</a><a href="/a">A</a></nav>
				<div><a href="/t1">tag1</a> <a href="/t2">tag2</a> <a href="/t3">tag3</a> <a href="/t4">tag4</a></div>
				<main><p>The article itself, with plenty of running prose to be the dominant region of this page by far.</p></main>
				<div><p>Follow-up note after the article.</p></div>
			</body></html>`,
			wantKinds: []string{
				SectionHeader,     // intro before main
				SectionNavigation, // the <nav>
				SectionNavigation, // link-dense tag cloud
				SectionMain,
				SectionFooter, // note after main
			},
		},
		{
			name: "fallback promotes the largest residual",
			doc: `<html><body>
				<div><a href="/">Home</a></div>
				<div><p>This block has the most text on the page by a wide margin, so with no findArticle callback and no landmarks it must be promoted to the main region.</p></div>
				<div><p>Tail.</p></div>
			</body></html>`,
			wantKinds: []string{SectionHeader, SectionMain, SectionFooter},
			check: func(t *testing.T, specs []PageSectionSpec) {
				// The single linked "Home" block is link-dense by the metrics
				// but below residualNavMinLinks (3 links): it must classify as
				// header, not navigation.
				if specs[0].Kind == SectionNavigation {
					t.Error("single-link block classified as navigation; link-count floor failed")
				}
			},
		},
		{
			name: "drops removal-classed residuals",
			doc: `<html><body>
				<div class="ad-banner"><a href="/buy">Buy now</a><a href="/buy2">Buy2</a><a href="/buy3">Buy3</a></div>
				<main><p>Article prose.</p></main>
				<div class="social-share"><a href="/tw">Tweet</a><a href="/fb">Share</a><a href="/ma">Mail</a></div>
			</body></html>`,
			wantKinds: []string{SectionMain},
		},
		{
			name: "ARIA landmarks",
			doc: `<html><body>
				<div role="banner"><a href="/">Home</a></div>
				<div role="main"><p>Article prose.</p></div>
				<div role="contentinfo"><p>© 2026</p></div>
				<div role="navigation"><a href="/a">A</a><a href="/b">B</a></div>
			</body></html>`,
			wantKinds: []string{SectionHeader, SectionMain, SectionFooter, SectionNavigation},
		},
		{
			// ASP.NET-style single <form> around the whole layout: the form
			// is a wrapper to descend, not a section.
			name: "form wrapping whole page",
			doc: `<html><body>
				<form runat="server">
					<nav><a href="/">Home</a><a href="/n">News</a></nav>
					<main><p>Server-rendered article prose, long enough to be the page's main region.</p></main>
				</form>
			</body></html>`,
			wantKinds: []string{SectionNavigation, SectionMain},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc, err := parseHTML(tt.doc)
			if err != nil {
				t.Fatalf("parseHTML: %v", err)
			}
			specs := SegmentPage(doc, nil)
			if got := kindsOf(specs); !slices.Equal(got, tt.wantKinds) {
				t.Fatalf("kinds = %v, want %v", got, tt.wantKinds)
			}
			if tt.wantChrome != nil {
				if got := chromeFlagsOf(specs); !slices.Equal(got, tt.wantChrome) {
					t.Fatalf("chrome flags = %v, want %v", got, tt.wantChrome)
				}
			}
			if tt.check != nil {
				tt.check(t, specs)
			}
		})
	}
}

func TestSegmentPageNilAndEmpty(t *testing.T) {
	t.Parallel()

	if specs := SegmentPage(nil, nil); specs != nil {
		t.Fatalf("SegmentPage(nil) = %v, want nil", specs)
	}
	doc, err := parseHTML(`<html><body></body></html>`)
	if err != nil {
		t.Fatalf("parseHTML: %v", err)
	}
	if specs := SegmentPage(doc, nil); specs != nil {
		t.Fatalf("SegmentPage(empty body) = %v, want nil", specs)
	}
}

// TestSegmentPageFallbackFindArticle covers the findArticle fallback in the
// descendant direction: div soup with no landmark tokens at all (every class
// deliberately token-free) — the callback locates the article node inside a
// residual, promoting that residual to main, and the remaining residuals
// classify positionally around it.
func TestSegmentPageFallbackFindArticle(t *testing.T) {
	t.Parallel()

	doc, err := parseHTML(`<html><body>
		<div class="mast"><a href="/">Home</a></div>
		<div><h1>Title</h1><p>Long article prose that the callback will locate. It keeps going with more than a little text so nothing else rivals it in length on this particular page.</p></div>
		<div class="legal"><p>© 2026</p></div>
	</body></html>`)
	if err != nil {
		t.Fatalf("parseHTML: %v", err)
	}

	var article *html.Node
	WalkNodes(doc, func(n *html.Node) bool {
		if article == nil && n.Type == html.ElementNode && n.Data == "div" &&
			n.FirstChild != nil && n.FirstChild.Type == html.ElementNode && n.FirstChild.Data == "h1" {
			article = n
		}
		return article == nil
	})
	if article == nil {
		t.Fatal("test fixture error: article div not found")
	}

	specs := SegmentPage(doc, func(*html.Node) *html.Node { return article })
	want := []string{SectionHeader, SectionMain, SectionFooter}
	if got := kindsOf(specs); !slices.Equal(got, want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
}

// TestSegmentPageFallbackFindArticleAncestor covers findArticle widened
// UPWARD (the scorer's climbToContentWrapper shape): the returned node is an
// ANCESTOR spanning several residuals — here the wrapper that also carries
// the <nav>. Every residual inside it is a fragment of the article and must
// be promoted to main; the old containment test matched only the descendant
// direction, so the second half was classified as footer chrome.
func TestSegmentPageFallbackFindArticleAncestor(t *testing.T) {
	t.Parallel()

	doc, err := parseHTML(`<html><body><div class="wrap">
		<nav><a href="/">Home</a><a href="/about">About</a></nav>
		<div><p>first half of a long article with plenty of words so the callback prefers the wrapper that holds it</p></div>
		<div><p>second half of the same article, equally wordy so it is not dropped as a stub</p></div>
	</div></body></html>`)
	if err != nil {
		t.Fatalf("parseHTML: %v", err)
	}

	var wrap *html.Node
	WalkNodes(doc, func(n *html.Node) bool {
		if wrap == nil && n.Type == html.ElementNode && n.Data == "div" {
			for _, attr := range n.Attr {
				if attr.Key == "class" && attr.Val == "wrap" {
					wrap = n
				}
			}
		}
		return wrap == nil
	})
	if wrap == nil {
		t.Fatal("test fixture error: wrapper div not found")
	}

	specs := SegmentPage(doc, func(*html.Node) *html.Node { return wrap })
	want := []string{SectionNavigation, SectionMain, SectionMain}
	if got := kindsOf(specs); !slices.Equal(got, want) {
		t.Fatalf("kinds = %v, want %v (article fragments under the wrapper must be main)", got, want)
	}
}
