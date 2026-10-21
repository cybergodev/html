package internal

// entity_decode_regression_test.go — regression test that ReplaceHTMLEntities
// decodes exactly one level of entities. The fast common-entity pass's output
// used to be fed through the full decoder again, so a replacement such as
// &amp; → & that synthesized a new entity token was decoded a second time:
// "&amp;amp;lt;" came out as "<" instead of "&lt;". Every case below expects
// the same single-level result as the standard library's html.UnescapeString.

import "testing"

func TestReplaceHTMLEntitiesSingleLevelDecoding(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		// One decode level: "&amp;" → "&", the rest stays literal — exactly
		// what html.UnescapeString returns for these inputs. (The double-decode
		// bug produced "&lt;", "&amp;", and "&hellip;" here.)
		{"&amp;amp;lt;", "&amp;lt;"},
		{"&amp;amp;amp;", "&amp;amp;"},
		{"&amp;amp;hellip;", "&amp;hellip;"},
		{"&amp;#65;", "&#65;"},
		{"x &amp;lt; y", "x &lt; y"},
		// Single-level sanity (unchanged behavior).
		{"&amp;", "&"},
		{"Tom &amp; Jerry", "Tom & Jerry"},
		{"&lt;p&gt;", "<p>"},
		{"&#65;&#x41;", "AA"},
		{"no entities here", "no entities here"},
		{"Tom & Jerry", "Tom & Jerry"},
	}
	for _, c := range cases {
		if got := ReplaceHTMLEntities(c.in); got != c.want {
			t.Errorf("ReplaceHTMLEntities(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
