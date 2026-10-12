// Package fonts embeds the font of goged's PDF export: Martian Mono
// Condensed by Evil Martians, a monospaced font, in regular and bold.
package fonts

import _ "embed"

var (
	// Regular and Bold are TrueType files.
	//
	//go:embed MartianMono-CnRg.ttf
	Regular []byte
	//go:embed MartianMono-CnBd.ttf
	Bold []byte

	//go:embed OFL.txt
	ofl string
)

// License names the embedded fonts and gives their license, the SIL Open
// Font License 1.1.
var License = `Font for PDF exports: Martian Mono 1.1.0 (Condensed Regular and Condensed
Bold) by Evil Martians, https://github.com/evilmartians/mono

` + ofl
