package server

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// Frames: the screen the model draws is text with SGR sequences for its
// colors and attributes, and nothing else. A line goes to the browser as a
// list of runs, each in one style:
//
//	[text, fg, bg, attrs]         every code point of text takes one cell
//	[text, fg, bg, attrs, cells]  text is one grapheme taking cells cells
//
// fg and bg are "#rrggbb", or "" for the page's colors. Wide characters
// and graphemes of several code points get runs of their own, so that the
// page needs to know nothing about character widths; the widths are those
// the model laid the screen out with.

// Attributes of a run.
const (
	attrBold = 1 << iota
	attrFaint
	attrItalic
	attrUnderline
	attrReverse
	attrStrike
)

type style struct {
	fg, bg string
	attrs  int
}

// run is a run of a line, encoded as a JSON array.
type run []any

// lineRuns splits a line of the screen into runs.
func lineRuns(line string) []run {
	runs := []run{}
	var (
		st    style
		text  strings.Builder
		state byte
	)
	flush := func() {
		if text.Len() > 0 {
			runs = append(runs, run{text.String(), st.fg, st.bg, st.attrs})
			text.Reset()
		}
	}
	for len(line) > 0 {
		seq, width, n, next := ansi.GraphemeWidth.DecodeSequenceInString(line, state, nil)
		line, state = line[n:], next
		switch {
		case width == 1 && utf8.RuneCountInString(seq) == 1:
			text.WriteString(seq)
		case width > 0:
			flush()
			runs = append(runs, run{seq, st.fg, st.bg, st.attrs, width})
		case strings.HasPrefix(seq, "\x1b[") && strings.HasSuffix(seq, "m"):
			next := st.apply(seq[2 : len(seq)-1])
			if next != st {
				flush()
				st = next
			}
		case seq != "" && !unicode.IsControl([]rune(seq)[0]):
			// A mark of no width the decoder left on its own, as after
			// ASCII: it belongs to the cell before.
			if s := text.String(); s != "" {
				_, size := utf8.DecodeLastRuneInString(s)
				text.Reset()
				text.WriteString(s[:len(s)-size])
				flush()
				runs = append(runs, run{s[len(s)-size:] + seq, st.fg, st.bg, st.attrs, 1})
			} else if n := len(runs); n > 0 && len(runs[n-1]) == 5 {
				runs[n-1][0] = runs[n-1][0].(string) + seq
			}
		}
		// Other sequences and control characters take no cell.
	}
	flush()
	return runs
}

// apply returns the style after an SGR sequence with params.
func (st style) apply(params string) style {
	ps := strings.FieldsFunc(params, func(r rune) bool { return r == ';' || r == ':' })
	if len(ps) == 0 {
		return style{}
	}
	num := func(i int) int {
		if i >= len(ps) {
			return -1
		}
		v, err := strconv.Atoi(ps[i])
		if err != nil {
			return -1
		}
		return v
	}
	// color reads an extended color at i, after 38 or 48, and returns it
	// with the number of parameters it took.
	color := func(i int) (string, int) {
		switch num(i) {
		case 5:
			if v := num(i + 1); v >= 0 && v < 256 {
				return palette(v), 2
			}
			return "", 2
		case 2:
			r, g, b := num(i+1), num(i+2), num(i+3)
			if r < 0 || g < 0 || b < 0 {
				return "", 4
			}
			return fmt.Sprintf("#%02x%02x%02x", r&0xff, g&0xff, b&0xff), 4
		}
		return "", 1
	}
	for i := 0; i < len(ps); i++ {
		switch v := num(i); {
		case v == 0:
			st = style{}
		case v == 1:
			st.attrs |= attrBold
		case v == 2:
			st.attrs |= attrFaint
		case v == 3:
			st.attrs |= attrItalic
		case v == 4 || v == 21:
			st.attrs |= attrUnderline
		case v == 7:
			st.attrs |= attrReverse
		case v == 9:
			st.attrs |= attrStrike
		case v == 22:
			st.attrs &^= attrBold | attrFaint
		case v == 23:
			st.attrs &^= attrItalic
		case v == 24:
			st.attrs &^= attrUnderline
		case v == 27:
			st.attrs &^= attrReverse
		case v == 29:
			st.attrs &^= attrStrike
		case v >= 30 && v <= 37:
			st.fg = palette(v - 30)
		case v >= 90 && v <= 97:
			st.fg = palette(v - 90 + 8)
		case v == 39:
			st.fg = ""
		case v >= 40 && v <= 47:
			st.bg = palette(v - 40)
		case v >= 100 && v <= 107:
			st.bg = palette(v - 100 + 8)
		case v == 49:
			st.bg = ""
		case v == 38:
			c, n := color(i + 1)
			st.fg, i = c, i+n
		case v == 48:
			c, n := color(i + 1)
			st.bg, i = c, i+n
		}
	}
	return st
}

// basic are the 16 colors of xterm.
var basic = [16]string{
	"#000000", "#cd0000", "#00cd00", "#cdcd00", "#0000ee", "#cd00cd", "#00cdcd", "#e5e5e5",
	"#7f7f7f", "#ff0000", "#00ff00", "#ffff00", "#5c5cff", "#ff00ff", "#00ffff", "#ffffff",
}

// palette returns color n of the 256 colors of xterm.
func palette(n int) string {
	switch {
	case n < 16:
		return basic[n]
	case n < 232:
		level := func(v int) int {
			if v == 0 {
				return 0
			}
			return 55 + 40*v
		}
		n -= 16
		return fmt.Sprintf("#%02x%02x%02x", level(n/36), level(n/6%6), level(n%6))
	}
	g := 8 + 10*(n-232)
	return fmt.Sprintf("#%02x%02x%02x", g, g, g)
}
