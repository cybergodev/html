package internal

// entity_nbsp_regression_test.go — regression for the D-002 P2 fix: the
// fast entity pass maps &nbsp; to a regular space (commonHTMLEntities), but
// the full pass restarted by any other entity in the string used to fall
// back to html.UnescapeString, which yields U+00A0 — the same input decoded
// differently depending on unrelated content. The full pass now consults
// commonHTMLEntities before the stdlib fallback, so both passes agree.

import "testing"

func TestNBSPEntityConsistentAcrossPasses(t *testing.T) {
	t.Parallel()

	// Fast pass alone: table hit, no restart.
	if got := ReplaceHTMLEntities("a&nbsp;b"); got != "a b" {
		t.Errorf("fast pass: ReplaceHTMLEntities = %q, want %q", got, "a b")
	}

	// Full pass: the numeric entity restarts decoding from the original
	// text; &nbsp; must still decode to a regular space there.
	if got := ReplaceHTMLEntities("a&nbsp;b&#65;"); got != "a bA" {
		t.Errorf("full pass: ReplaceHTMLEntities = %q, want %q", got, "a bA")
	}
	if got := ReplaceHTMLEntities("x&nbsp;y&mdash;z"); got != "x y—z" {
		t.Errorf("full pass (named restart): ReplaceHTMLEntities = %q, want %q", got, "x y—z")
	}
}
