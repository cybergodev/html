package html

// processor_adapter_internal_test.go — white-box tests for contentNodeAdapter
// (processor.go). The black-box TestContentNodeAdapter in boundary_test.go only
// observes nodes a real extraction hands to a custom scorer, so it never sees
// most node types; Type() sat at 27% coverage. Constructing adapters directly
// pins every branch of the node-type switch and every nil-guard branch.

import (
	"testing"

	stdxhtml "golang.org/x/net/html"
)

// TestContentNodeAdapterType drives contentNodeAdapter.Type() through every
// branch of its node-type switch, including the default ("unknown") branch.
func TestContentNodeAdapterType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		nodeType stdxhtml.NodeType
		want     string
	}{
		{"error node", stdxhtml.ErrorNode, "error"},
		{"text node", stdxhtml.TextNode, "text"},
		{"document node", stdxhtml.DocumentNode, "document"},
		{"element node", stdxhtml.ElementNode, "element"},
		{"comment node", stdxhtml.CommentNode, "comment"},
		{"doctype node", stdxhtml.DoctypeNode, "doctype"},
		{"raw node", stdxhtml.RawNode, "raw"},
		{"unknown node type", stdxhtml.NodeType(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			n := contentNodeAdapter{&stdxhtml.Node{Type: tt.nodeType}}
			if got := n.Type(); got != tt.want {
				t.Errorf("Type() = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("nil node returns empty string", func(t *testing.T) {
		t.Parallel()
		var n contentNodeAdapter
		if got := n.Type(); got != "" {
			t.Errorf("Type() on nil node = %q, want empty", got)
		}
	})
}

// TestContentNodeAdapterNilGuards pins the nil-receiver / nil-child branches of
// every accessor: a nil embedded node must yield zero values, and a node whose
// FirstChild/NextSibling/Parent pointer is nil must yield a nil ContentNode
// (never a non-nil adapter wrapping nil).
func TestContentNodeAdapterNilGuards(t *testing.T) {
	t.Parallel()

	t.Run("nil embedded node", func(t *testing.T) {
		t.Parallel()
		var n contentNodeAdapter
		if got := n.Data(); got != "" {
			t.Errorf("Data() on nil node = %q, want empty", got)
		}
		if got := n.AttrValue("href"); got != "" {
			t.Errorf("AttrValue on nil node = %q, want empty", got)
		}
		if attrs := n.Attrs(); attrs != nil {
			t.Errorf("Attrs() on nil node = %v, want nil", attrs)
		}
		if got := n.FirstChild(); got != nil {
			t.Errorf("FirstChild() on nil node = %v, want nil", got)
		}
		if got := n.NextSibling(); got != nil {
			t.Errorf("NextSibling() on nil node = %v, want nil", got)
		}
		if got := n.Parent(); got != nil {
			t.Errorf("Parent() on nil node = %v, want nil", got)
		}
	})

	t.Run("leaf node yields nil navigation results", func(t *testing.T) {
		t.Parallel()
		// A standalone element node has no parent, child, or sibling.
		n := contentNodeAdapter{&stdxhtml.Node{
			Type: stdxhtml.ElementNode,
			Data: "p",
		}}
		if got := n.FirstChild(); got != nil {
			t.Errorf("FirstChild() = %v, want nil for leaf", got)
		}
		if got := n.NextSibling(); got != nil {
			t.Errorf("NextSibling() = %v, want nil for unlinked node", got)
		}
		if got := n.Parent(); got != nil {
			t.Errorf("Parent() = %v, want nil for unlinked node", got)
		}
	})

	t.Run("linked nodes navigate correctly", func(t *testing.T) {
		t.Parallel()
		// parent -> child (two children: a, b).
		parent := &stdxhtml.Node{Type: stdxhtml.ElementNode, Data: "div"}
		a := &stdxhtml.Node{Type: stdxhtml.ElementNode, Data: "a", Parent: parent}
		b := &stdxhtml.Node{Type: stdxhtml.ElementNode, Data: "b", Parent: parent}
		parent.FirstChild, parent.LastChild = a, b
		a.NextSibling, b.PrevSibling = b, a

		np := contentNodeAdapter{parent}
		first := np.FirstChild()
		if first == nil || first.Data() != "a" {
			t.Errorf("FirstChild() = %v (Data %q), want node 'a'", first, first.Data())
		}
		second := first.NextSibling()
		if second == nil || second.Data() != "b" {
			t.Errorf("NextSibling() = %v (Data %q), want node 'b'", second, second.Data())
		}
		back := second.Parent()
		if back == nil || back.Data() != "div" {
			t.Errorf("Parent() = %v (Data %q), want node 'div'", back, back.Data())
		}
	})

	t.Run("attribute access", func(t *testing.T) {
		t.Parallel()
		n := contentNodeAdapter{&stdxhtml.Node{
			Type: stdxhtml.ElementNode,
			Data: "img",
			Attr: []stdxhtml.Attribute{
				{Key: "src", Val: "a.png"},
				{Key: "alt", Val: "pic"},
			},
		}}
		if got := n.AttrValue("src"); got != "a.png" {
			t.Errorf("AttrValue(src) = %q, want %q", got, "a.png")
		}
		if got := n.AttrValue("missing"); got != "" {
			t.Errorf("AttrValue(missing) = %q, want empty", got)
		}
		attrs := n.Attrs()
		if len(attrs) != 2 || attrs[0].Key != "src" || attrs[0].Value != "a.png" {
			t.Errorf("Attrs() = %+v, want [{src a.png} {alt pic}]", attrs)
		}
	})
}
