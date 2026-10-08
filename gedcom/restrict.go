package gedcom

import "strings"

// IsRestricted reports whether a RESN value asks for the data to be withheld:
// "confidential" or "privacy" (GEDCOM 7 allows a list such as
// "CONFIDENTIAL, LOCKED"). "locked" only protects the data from changes.
func IsRestricted(resn string) bool {
	for _, v := range strings.Split(resn, ",") {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "confidential", "privacy":
			return true
		}
	}
	return false
}

// restricted reports whether n has a RESN child asking to withhold it.
func restricted(n *Node) bool {
	for _, c := range n.Children {
		if c.Tag == "RESN" && IsRestricted(c.Value) {
			return true
		}
	}
	return false
}

// keptOfRestrictedRecord lists what remains of an individual or family
// marked confidential or private: enough to keep the family tree intact.
var keptOfRestrictedRecord = map[string]bool{
	"NAME": true, "SEX": true, "FAMC": true, "FAMS": true,
	"HUSB": true, "WIFE": true, "CHIL": true, "RESN": true,
}

// Redacted returns a copy of the document without the data marked
// confidential or private with RESN. Restricted events and other
// structures are left out; of restricted individuals and families only the
// names, sex and family links remain; other restricted records lose all
// their content. Redactions counts what was withheld.
func (d *Document) Redacted() *Document {
	count := 0
	var prune func(n *Node, parent *Node) *Node
	prune = func(n *Node, parent *Node) *Node {
		c := &Node{Level: n.Level, Xref: n.Xref, Tag: n.Tag, Value: n.Value, Line: n.Line, Parent: parent}
		whole := n.Level == 0 && restricted(n)
		if whole {
			count++
		}
		for _, child := range n.Children {
			switch {
			case whole && !(n.Tag == "INDI" || n.Tag == "FAM") && child.Tag != "RESN":
				continue
			case whole && (n.Tag == "INDI" || n.Tag == "FAM") && !keptOfRestrictedRecord[child.Tag]:
				continue
			case restricted(child):
				count++
				continue
			}
			c.Children = append(c.Children, prune(child, c))
		}
		return c
	}
	records := make([]*Node, len(d.Records))
	for i, r := range d.Records {
		records[i] = prune(r, nil)
	}
	out := newDocument(records, d.inputWarnings)
	out.Encoding = d.Encoding
	out.Redactions = count
	return out
}
