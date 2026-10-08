package gedcom

import (
	"fmt"
	"strings"
)

// Node is a GEDCOM line together with its subordinate lines. CONT and CONC
// continuation lines are merged into the value of their parent node during
// parsing and never appear as children.
type Node struct {
	Level    int
	Xref     string // "@I1@" for records that carry an identifier
	Tag      string
	Value    string
	Line     int // line number of the node in the source file
	Parent   *Node
	Children []*Node
}

// First returns the first child with the given tag, or nil.
func (n *Node) First(tag string) *Node {
	if n == nil {
		return nil
	}
	for _, c := range n.Children {
		if c.Tag == tag {
			return c
		}
	}
	return nil
}

// All returns all children with the given tag.
func (n *Node) All(tag string) []*Node {
	if n == nil {
		return nil
	}
	var out []*Node
	for _, c := range n.Children {
		if c.Tag == tag {
			out = append(out, c)
		}
	}
	return out
}

// Val returns the trimmed value of the first child with the given tag.
func (n *Node) Val(tag string) string {
	if c := n.First(tag); c != nil {
		return strings.TrimSpace(c.Value)
	}
	return ""
}

// Path follows a chain of tags, taking the first matching child at each
// step, e.g. n.Path("BIRT", "DATE").
func (n *Node) Path(tags ...string) *Node {
	for _, t := range tags {
		n = n.First(t)
		if n == nil {
			return nil
		}
	}
	return n
}

// IsPointer reports whether the node's value is a cross-reference pointer.
func (n *Node) IsPointer() bool { return n != nil && IsPointer(n.Value) }

// Walk visits n and its descendants in document order. If fn returns false
// the children of that node are skipped.
func (n *Node) Walk(fn func(*Node) bool) {
	if n == nil || !fn(n) {
		return
	}
	for _, c := range n.Children {
		c.Walk(fn)
	}
}

// TagPath returns the dotted tag path from the record down to n, e.g.
// "INDI.BIRT.DATE".
func (n *Node) TagPath() string {
	var parts []string
	for p := n; p != nil; p = p.Parent {
		parts = append(parts, p.Tag)
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, ".")
}

// String renders the node as a GEDCOM line (without its children).
func (n *Node) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d", n.Level)
	if n.Xref != "" {
		b.WriteString(" " + n.Xref)
	}
	b.WriteString(" " + n.Tag)
	if n.Value != "" {
		b.WriteString(" " + strings.ReplaceAll(n.Value, "\n", `\n`))
	}
	return b.String()
}
