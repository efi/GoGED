package gedcom

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Warning is a non-fatal problem found while reading a file.
type Warning struct {
	Line int // 0 if the warning is not tied to a line
	Msg  string
}

func (w Warning) String() string {
	if w.Line > 0 {
		return fmt.Sprintf("line %d: %s", w.Line, w.Msg)
	}
	return w.Msg
}

// ErrNoRecords is returned when the input contains no GEDCOM records at all.
var ErrNoRecords = errors.New("gedcom: no records found (is this a GEDCOM file?)")

// ParseFile reads and parses the GEDCOM file at path.
func ParseFile(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseBytes(data)
}

// Parse reads all of r and parses it as GEDCOM.
func Parse(r io.Reader) (*Document, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return ParseBytes(data)
}

// ParseString parses GEDCOM text that is already decoded.
func ParseString(s string) (*Document, error) {
	return ParseBytes([]byte(s))
}

// ParseBytes detects the character encoding of data, decodes it and parses
// it into a Document.
func ParseBytes(data []byte) (*Document, error) {
	text, enc, encWarnings := Decode(data)
	records, warnings := parseRecords(text)
	if len(records) == 0 {
		return nil, ErrNoRecords
	}
	var all []Warning
	for _, w := range encWarnings {
		all = append(all, Warning{Msg: w})
	}
	all = append(all, warnings...)
	doc := newDocument(records, all)
	doc.Encoding = enc
	return doc, nil
}

// parseRecords builds the node tree from decoded text.
func parseRecords(text string) ([]*Node, []Warning) {
	var (
		records       []*Node
		warnings      []Warning
		stack         []*Node // stack[i] is the most recent node at level i
		last          *Node   // most recently created node, target for stray text
		trailer       bool    // a TRLR record has been seen
		warnedTrailer bool
	)
	warn := func(line int, format string, args ...any) {
		warnings = append(warnings, Warning{line, fmt.Sprintf(format, args...)})
	}

	forEachLine(text, func(num int, raw string) {
		if strings.TrimSpace(raw) == "" {
			return
		}
		if trailer {
			// Whatever follows the trailer, valid lines or not, is ignored.
			if !warnedTrailer {
				warn(num, "data after TRLR record ignored")
				warnedTrailer = true
			}
			return
		}
		l, err := ParseLine(raw, num)
		if err != nil {
			// A common defect is a note containing raw line breaks. Treat
			// unparseable text as a continuation of the previous value.
			if last != nil {
				warn(num, "%s; treated as continuation of line %d", err.(*SyntaxError).Msg, last.Line)
				last.Value += "\n" + strings.TrimSpace(raw)
			} else {
				warn(num, "%s; line ignored", err.(*SyntaxError).Msg)
			}
			return
		}

		if l.Level == 0 {
			n := &Node{Level: 0, Xref: l.Xref, Tag: l.Tag, Value: l.Value, Line: num}
			records = append(records, n)
			stack = []*Node{n}
			last = n
			if l.Tag == "TRLR" {
				trailer = true
			}
			return
		}
		if len(stack) == 0 {
			warn(num, "line at level %d appears before any record; ignored", l.Level)
			return
		}

		level := l.Level
		if level > len(stack) {
			warn(num, "level jumps from %d to %d; attached to the preceding line", len(stack)-1, level)
			level = len(stack)
		}
		stack = stack[:level]
		parent := stack[level-1]

		switch l.Tag {
		case "CONT":
			parent.Value += "\n" + l.Value
			return
		case "CONC":
			parent.Value += l.Value
			return
		}
		if l.Xref != "" {
			warn(num, "cross-reference identifier %s on a level %d line ignored", l.Xref, l.Level)
		}
		n := &Node{Level: level, Tag: l.Tag, Value: l.Value, Line: num, Parent: parent}
		parent.Children = append(parent.Children, n)
		stack = append(stack, n)
		last = n
	})

	if len(records) > 0 {
		if records[0].Tag != "HEAD" {
			warn(records[0].Line, "file does not start with a HEAD record")
		}
		if !trailer {
			warn(0, "missing TRLR record; the file may be truncated")
		}
	}
	return records, warnings
}
