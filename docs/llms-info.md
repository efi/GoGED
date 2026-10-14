# Notes for LLM agents working on goged

This file records how goged was built so far, why things are the way they
are, and what tripped us up. It is written for an LLM coding agent (or a
human) taking over the project. It reflects the state of commit `8e16243`
on branch `claude/gedcom-browser-tui-oirhyo`.

## 1. What the project is

goged is a terminal (TUI) browser for GEDCOM genealogy files, written in Go,
plus a batch mode for scripts. The original request was: "A TUI based GEDCOM
file browser written in go. Search for names and other properties, have
important events listed, jump to relatives, render tree views. Extensive test
suite to go alongside." Everything since grew from that brief and from
feedback on the GEDCOM-L sample file (`testdata/Muster_GEDCOM_UTF-8.ged`).

Main features, by TUI tab (keys 1–9, or alt+1–9 while typing):

1. **Search**: query language with fields, ranges, negation, phonetic matching.
2. **Person**: parents, siblings, families, associations, timeline, notes,
   sources; every relative is a link; browser-like back and forward.
3. **Tree**: horizontal pedigree chart or indented descendant chart.
4. **Events**: all or the vital events, chronological, with a filter.
5. **Places**: jurisdiction hierarchy, events per place.
6. **Map** (experimental): Braille world map with markers, using embedded
   Natural Earth data.
7. **Stats**: counts, time span, names, "On this day", warnings.
8. **Checks**: plausibility checks with thresholds that can be changed
   while looking at the findings.
9. **Export**: everybody or the people connected to the marked person, as a
   GEXF graph or as PDF pages.

The batch flags are `-q`, `-tree`, `-desc`, `-timeline`, `-relate ID1,ID2`,
`-events FILTER` (plus `-all`), `-places`, `-stats`, `-gen`, `-ascii`,
`-person`, `-no-mouse`, `-show-restricted`, `-version` and `-licenses`, and
`-export FORMAT` with `-pivot ID` and `-o FILE` (default: the input's path
with the format's extension; `-` is standard output, and required with
stdin input). Passing `-` as the file reads from stdin. The batch export
gets its formats from `tui.ExportFormats`, so it offers the same as the
Export tab, PDF included.

## 2. Working agreements with the user

These came from the user and should be kept unless they say otherwise.

- **One commit per fix:** when asked to fix a list of issues, commit each fix
  on its own. Split work so that each commit builds and passes tests on its
  own. If hunks of one file belong to different commits, reconstruct the
  intermediate file states; `git add -p` is not available.
- **"Don't commit yet" means it:** if the user says "implement without
  committing" or "don't commit until I reviewed it", wait. That holds even
  when a stop hook complains about uncommitted changes. Say why you are
  waiting instead.
- **Commit messages:** an imperative subject, then a body explaining what and
  why. They end with the attribution trailer lines that the session's harness
  prescribes. Never put model identifiers in commits, code or docs.
- **Releases:** "make a release" means triggering the `Release` workflow
  (`workflow_dispatch`) on the branch. Releases are tagged with the build
  date; a second run on the same day replaces that day's release, and the
  user accepted that. Afterwards, download one binary, check it against
  `SHA256SUMS` and run it.
- **Reports:** after sanity checks or reviews, report findings grouped as
  what works, goged issues, and problems in the data itself. Then offer fixes
  rather than starting them unasked.
- **Asking first:** the user steers closely and interrupts tool calls to
  redirect. When an approach fails, report it and offer two or three options
  rather than escalating on your own. "Try 1) first" was taken to mean "then
  continue with 2)", but the user then stopped the work. Prefer asking.
- **Demo recording:** the user will record a demo GIF manually later. All
  terminalizer work was scrapped. See section 9 for what was learned.

## 3. Environment and tooling (cloud container)

- **Go:** 1.24 (`go.mod`: `go 1.24.2`), and `GOTOOLCHAIN=local`, so no
  toolchain downloads. Module path: `github.com/efi/goged`.
- **Repository rename:** the GitHub repository was renamed to `efi/GoGED`.
  Pushes still work through GitHub's redirect, and the MCP GitHub tools
  accept `efi/goged`. The module path was not changed (an open question for
  the user).
- **Network:** an egress proxy is in place. `gitlab.genealogy.net` is
  blocked, so the user uploaded the GEDCOM-L sample instead. The npm
  registry, `proxy.golang.org` and GitHub release downloads work.
- **No `gh` CLI:** use the MCP GitHub tools (`actions_list`,
  `actions_run_trigger`, `get_release_by_tag`, ...).
- **staticcheck:** CI runs `go run honnef.co/go/tools/cmd/staticcheck@2025.1.1`.
  Locally a built binary sits in the Go build cache; find it with
  `find / -name staticcheck -type f -perm -u+x`.
- **Pre-push checklist:**
  - `gofmt -l .` must print nothing. CI fails otherwise, and an import-order
    slip happened once.
  - `go vet ./...`, `go test ./...`, staticcheck.
  - `go test -race ./...` and a short fuzz run for model changes.
  - `go mod tidy` leaves `go.mod` and `go.sum` unchanged.
- **CI timing:** a run takes about 1.5 minutes. Wait with a background
  `sleep`, then query `actions_list`; there is no polling tool.
- **Write tool and Unicode escapes:** the Write tool turned `\uXXXX` escapes
  in Go source into literal characters. Combining marks and format
  characters then become invisible or merge. Keep such escapes in files
  written with Python, or check the result.
- **Untrusted downloads:** put downloaded files in their own directory and
  run Python on them with `-I`.
- **Screenshots of the TUI:** what worked was tmux with
  `COLORTERM=truecolor`, then `capture-pane -e`, an ANSI-to-HTML script, and
  a Playwright screenshot (Chromium is in `/opt/pw-browsers`). Electron, by
  contrast, needs an X display: Xvfb exists, but the user did not want it
  used, and headless Ozone crashes in Electron 25 and 33.

## 4. Architecture

```
main.go            flag parsing, batch output, starts the TUI (run() is testable)
gedcom/            parsing and the typed model (no UI)
genealogy/         analyses: relationships, timeline, places, stats, on-this-day
search/            query language, folding, phonetics, person and event indexes
chart/             text rendering of pedigree and descendant charts
worldmap/          embedded Natural Earth data, projection, Braille renderer
worldmap/mkworld/  converter from Natural Earth GeoJSON to world.bin
export/            selection of people and links, GEXF writer, atomic file writing
fonts/             Martian Mono Condensed (regular, bold) and its OFL, for the PDF
tui/               Bubble Tea model, one file per view
licenses/          embedded third-party license texts (generated)
scripts/           build-release.sh (makefat universal macOS), gen-licenses.sh
testdata/          family.ged (own, fictional), Muster_GEDCOM_UTF-8.ged (CC BY 4.0)
```

Data flows in one direction: bytes → `gedcom.Decode` (charset) → lines →
node tree → `Document` with `Individual`, `Family`, `Source`, `Location`
and `Association`. The `genealogy`, `search` and `chart` packages read the
model; `tui` and `main` present it. `gedcom` imports nothing from the other
packages.

### gedcom

- **Parser** (`parser.go`, `line.go`): lenient. Problems become `Warning`s
  instead of errors. CONT/CONC are merged into the value; the only error is
  `ErrNoRecords`.
- **Charsets** (`decode.go`, `ansel.go`): BOM, UTF-16, ANSEL with combining
  marks (a dangling diacritic is dropped), CP1252, Latin-1, CP437/850 and
  MacRoman. Detection uses the HEAD.CHAR value and the bytes themselves.
- **Linking** (`document.go`): links are repaired in both directions and
  reported as warnings. `checkAncestryLoops` finds people who are their own
  ancestors. `resolvePedigrees` works out the link to each parent.
- **Dates** (`date.go`, `calendar.go`):
  - All GEDCOM 5.5.1 and 7 forms, plus common deviations (full month names,
    `Abt.`, ISO dates, "Month day, year").
  - Gregorian, Julian, Hebrew and French Republican calendars, mapped to
    Julian Day Numbers (Hebrew epoch JDN 347997).
  - `Span`, `Key`, `YearRange`, `ShortYear`, `String`.
  - `Fit(width)` returns a compact column form plus whatever it left out.
  - `AgeBetween` refuses open-ended dates.
- **Names** (`name.go`): the NAME value wins over GIVN and SURN.
  - NPFX is a title: it is cut from the given name when the value repeats it,
    shown in `String()`, and left out of `SurnameFirst()`.
  - SPFX marks a sorting prefix: `SortSurname` drops it, giving "Stradonitz,
    Maria von". A prefix that is only part of SURN keeps its place ("von
    Stradonitz, Erich Karl"), as agreed by GEDCOM-L.
  - `CallName` comes from `_RUFNAME`.
- **Events** (`event.go`):
  - A table of tags with classes vital, event and attribute.
  - `Label()`: an EVEN or FACT uses its TYPE, and an EVEN without TYPE uses
    its value. MARR with TYPE CIVIL or RELI becomes "Civil marriage" or
    "Religious marriage".
  - `Detail()` gives the value, the type, "by X" for adoptions and the cause.
  - `Facts()` gives the address, RELI, AGNC, `_GODP` and `_WITN`.
- **Places** (`event.go`, `location.go`): `Place` has a name, coordinates
  from PLAC.MAP, a `Location` (the `_LOC` record) and a GOV id. Place
  records carry names with LANG, TYPE, `_GOV`, MAP and dated superior
  `_LOC` links.
- **Pedigree** (`individual.go`):
  - `Pedigree` values: unknown, birth, adopted, foster, sealed, step, other.
    `ParsePedigree` also accepts `_FREL`/`_MREL` values such as "Natural".
  - `FamilyLink` has a `Husband` and a `Wife` pedigree. `ParentLinks()`
    lists each parent once with the closest link. `BirthParents()` exists,
    and `Father()`/`Mother()` prefer birth parents.
- **Privacy** (`restrict.go`): `IsRestricted` (confidential or privacy, not
  locked). `Document.Redacted()` deep-copies the records, prunes them and
  rebuilds the document from `inputWarnings`, so warnings are not doubled;
  `Redactions` counts what was withheld. main applies it unless
  `-show-restricted` is given.
- **Associations** (`association.go`): ASSO, and `_ASSO` below events (used
  for marriage witnesses), with RELA (5.5.1) or ROLE (7).
  `Individual.Associations()`, `AssociatedBy()`, and `Aliases()` (ALIA links
  in both directions). An ALIA holding a name instead of a link becomes an
  alternate name of type "alias".

### genealogy

- **`Relate(a, b)`** (`relationship.go`) works in this order:
  1. Blood, following birth links only, via BFS over ancestors and over
     ancestral families. A full relationship needs a shared family; a half
     relationship needs one shared individual.
  2. Spouse.
  3. Adoptive: the same search over all links, recording the most distant
     kind of link (`KindAdoptive`, `Relationship.Pedigree`). It gives
     "adoptive father", "foster sister", "stepbrother", and "X (sealed)".
  4. In-laws through a spouse or through a relative.
  5. `twoMarriages`: co-parents-in-law, step-siblings, "wife's sister's
     husband", "husband's stepmother".

  The describe rules put "half-" before the great-prefix ("half-great-aunt").
- **`Timeline`** (`timeline.go`): own events plus the relatives' births,
  marriages and deaths within the lifetime. `Title()` includes the partner,
  with natural prepositions ("Divorce from X filed", "Engagement to X").
  Relatives are qualified ("adoptive father").
- **`Places`** (`places.go`): a tree built from the comma-separated
  jurisdictions. Events sharing a `_LOC` record are grouped under the place
  as written for the most recent dated event (`canonicalPlaces`); the other
  spellings become `Aliases`.
- **Plausibility checks** (`checks.go`): `Checks` is a data-driven list of
  `Check`s (ID, title with `%d` for the threshold, default `Param`, `Min`,
  `Max`, unit, run function) returning `Finding`s (person, other person,
  message). Imprecise dates are handled conservatively by `bounds`: BEF and
  AFT are open on one side, ABT/CAL/EST widened by two years, so only
  certain problems are reported. Ages are completed years from calendar
  dates (`ages`/`completed`), not days divided by 365.25, which gave
  off-by-one errors on birthdays. Parental age checks follow birth links
  only. Duplicates are bucketed by Soundex+Cologne of the first given
  name and the surname; namesake siblings only count if they may share a
  birthday. The TUI (`tui/checks.go`) keeps a threshold, an on/off switch
  and the findings per check and reruns only the changed check. The
  GEDCOM-L sample yields three findings: Wilhelm Schüchter's two birth
  dates, and two siblings born 81 and 189 days apart.
- **Hints** (`hints.go`) are `Check`s with `Hint` set, after the checks,
  shown as a section of their own. A `Context` brings the marked person
  and today's date (`Options.Today` in the TUI); `UsesMarked` checks run
  again when the mark changes (`markChanged`). `summary` gives a line
  under the title (the completeness of the marked person's ancestry per
  generation, counted by positions); `ordered` keeps the run's order
  (generations; place groups). `missing-parents` walks birth links,
  reporting each person once at the nearest generation. `unsourced`
  counts a SOUR anywhere below the event or directly in the record. A
  family counts as married with a MARR (also `MARR Y`) or a DIV/ANUL.
- **Place-name groups** (`similar-places`): a threshold of `Levels`
  ("almost identical", "similar", "somewhat resembling"), as the user
  wanted the complexity hidden. Only the children of one `PlaceNode` are
  compared, so different Neustadts stay apart; groups are connected
  components (union-find), names and groups sorted alphabetically.
  Almost identical: same `newPlaceName` key (folded, umlauts written out
  as plain vowels, ß as s, St/Sankt/Saint, no punctuation or spaces).
  Similar: also both phonetic codes equal, one edit (4+ letters), or an
  abbreviation word by word (4+ letters, or a dot in the name). Somewhat
  resembling: also one code, two edits (5+), a quarter of the letters
  (8+), or one name beginning with the other. The samples have no group
  at "similar"; at the loosest, "St Peter's"/"St Peter's Churchyard" and
  "Berlin"/"Berlin (West)". Findings carry `Places`; enter opens the
  first in the Places tab (`showPlace` expands and clears the filter), o
  the next.
- **Pedigree collapse** (`Stats.PedigreeCollapse`): one number for the
  file. Every line of birth links (`ParentLinks`, `IsBirth`, so
  unspecified links count) from a person up to an ancestor is a position
  in that person's pedigree; the share of positions taken by an ancestor
  already in another position of the same pedigree, summed over all
  pedigrees, is the collapse. Positions are counted by recurrence (a
  float: they double per generation of collapse), distinct ancestors by a
  marked walk per person. Uniqueness must be per pedigree: counting
  child-parent links against the distinct parents of the whole file
  measured family size (65% for family.ged), which the user rightly
  doubted. family.ged and the GEDCOM-L sample have no collapse (0.0%);
  a child of first cousins has 2 repeats in 10 positions. The Stats view
  shows it before the warnings, `-stats` too.
- **Distributions** (`Stats.BirthYears`, `DeathAges`, `MarriageAges`,
  `FamilySizes`, each a sorted `Distribution` of value counts) feed the
  bar charts of the Stats view; `Distribution.Buckets(width, last)` groups
  them in aligned ranges, keeps empty ranges between, and gathers values
  from `last` on in an open bucket. Birth years skip open-ended dates
  and take the middle of ranges; ages use `AgeBetween`, as the lifespans
  do. The view picks the period of the births so that at most 25 bars
  show, and links each bar of births to a `born:` search. Bars are
  lines, `━` in half columns with `╸` (`=` with `-ascii`): the user chose
  them over solid blocks, whose large areas were an eye-sore, and over
  `▄`, which is not centered in the row. A selected bar shows only its
  label and count in reverse (`line.colsOnly`), so that the bar keeps its
  look. The view lays the statistics out again on resize (`buildStats`),
  without computing them again.
- **`Compute`** (`stats.go`) for statistics, and **`OnThisDay`**
  (`onthisday.go`): exact dates only, compared in the Gregorian calendar,
  with 29 February shown on 28 February in common years.

### search

- **Folding:** `Fold` applies NFKD per rune, with a `sync.Map` cache, and
  transliterates ß, æ, ø and ligatures.
- **Phonetics:** `~name` needs Soundex **and** Cologne phonetics (Kölner
  Phonetik) to agree. Soundex alone confused Mustermann and Musterow (both
  M236). `Cologne` matches the published reference values (Wikipedia →
  3412, Müller-Lüdenscheidt → 65752682).
- **Query language** (`query.go`): plain words, phrases, `-` negation, `~`,
  and these fields:
  - `given:`, `surname:`, `name:`, `born:`, `died:`, `place:`, `year:`,
    `alive:`, `sex:`, `id:`
  - `occupation:`, `note:`, `source:`, `any:`, `tag:PATH=value`,
    `has:WHAT`
  - year ranges such as `1850..1860`, `<1900`, `~1850` and `1850s`
- **Year windows:** `eventYears(d, openEnded)` widens ABT, CAL and EST by ±2
  years. BEF and AFT are widened by 10 years only for born:, died: and
  alive: (`openEnded`); for `year:` and the events filter they count for the
  named year only.
- **Indexes:** `Index` precomputes folded names, spans, place names
  (including `_LOC` names) and a sort key built from `SortSurname`.
  `EventIndex` precomputes keys; computing them inside the sort comparator
  was a performance bug.

### chart

- **`Pedigree`:** a horizontal chart with fathers above and mothers below;
  column widths per generation. `Next` (up and down) stays within a
  generation.
  - **Repeated ancestors:** after marriages between relatives, the same
    person appears more than once. Their ancestors are drawn at one
    occurrence only; the others get a `KindDuplicate` placeholder node
    that reads "truncated", "as", "duplicate" on three lines where the
    parents would be.
  - **Which occurrence:** `expandedOccurrences` walks the generations
    outwards, so by default it is the one closest to the root, the topmost
    of those. `Options.Expanded` chooses another by `Node.Path` ("FM" =
    father's mother); a choice that is not in the chart, or has no room
    for parents, is dropped. This keeps the chart linear in the number of
    people; before, a 20-generation pedigree of 41 inbred people had a
    million nodes.
  - **In the TUI,** enter or space on a placeholder stores the choice in
    `treeState.expanded`. The choices are reset when the root changes, and
    changing the generations keeps the selection by path.
- **`Descendants`:** an indented tree with spouse lines. It marks
  non-birth children "(adopted)", and does not expand a person a second time
  ("(see above)").
- **Nodes** carry `Line`, `Col`, `Width`, `Up`, `Down`, `More`, `Kind` and,
  in pedigrees, `Path` (`FindPath` looks it up), so the TUI can select and
  navigate them.
- **Label widths:** `fitLabel` shortens the name, not the life dates. The TUI
  passes an unreachable limit for descendant charts (full labels) and 36 for
  pedigrees. main does the same for `-desc`.

### worldmap

- `world.bin` (about 1.8 MB) is Natural Earth 1:10m data, with the regional
  river supplements for Europe, North America and Australia (they added
  about 380 KB to the executables). The format is
  "GOGEDMAP" v1: gzip, coordinates scaled by 1e4, zigzag varint deltas, and
  the Natural Earth `min_zoom` for level of detail. It is embedded with
  `go:embed`, so goged stays a single executable.
- The renderer uses a Web Mercator projection, Liang–Barsky clipping and
  Bresenham lines on a Braille canvas (2×4 dots per cell). Borders are drawn
  dotted. Labels are placed right of a marker, else left. A rune value of
  −1 marks the second cell of a wide character.
- Natural Earth data is public domain; it is still credited ("Made with
  Natural Earth"). The `L` key in the TUI and the `-licenses` flag show all
  licenses.

### export

- **`Select`** decides who is exported: `All`, or `Component`, everybody
  connected to `Options.Root` (the marked person). It also returns the
  `Links`: the partners of each family, and each partner with each child,
  with the child's pedigree towards that parent. The component is computed
  over these same links, so it is connected in every export; siblings
  without known parents are not linked.
- **Formats** are values in `Formats` (name, description, extension and a
  `Write` function), so the TUI lists them without knowing them. Filters
  still to come, such as a temporal cutoff, belong in `Options` and
  `Select`, so that every format gets them.
- **GEXF** is written as version 1.2 (draft), the one Gephi, networkx and
  sigma.js all read, with `encoding/xml` streaming. People are nodes (id =
  GEDCOM ID). Parent edges are directed; partner edges have the type
  `mutual`, because networkx refuses undirected edges in a directed graph.
  The output was validated against the official XSD with lxml and loaded
  with networkx; see llms-toolinfo.md for the throwaway environment.
- **`WriteFile`** writes a temporary file in the target directory and
  renames it, so a failed export leaves an existing file alone.

### PDF and HTML export (tui/book.go, pdf.go, html.go)

- **The book:** `bookBlocks` (book.go) builds what both present: the
  contents, a block per person and the places, each a list of `bookLine`s
  (a line and the block it refers to) with an anchor `id`. The PDF wraps
  them to the page (`wrapToPage`) and paginates; the HTML (`writeHTML`)
  writes one page with a `<section>` per block and lets the browser wrap:
  columns become `<span class="c" style="width:Nch">` in a monospaced
  font, the text after them `<span class="t">` with its leading spaces as
  `padding-left`, and lines that refer to a block are `<a>` links. The
  HTML is well-formed XML (self-closed `<meta />`, no named entities), so
  tests parse it with `encoding/xml`. No JavaScript yet; search and
  styling are for later.
- **Same lines as the views:** the PDF prints the `line`s of the interface:
  `personLines(ind, width)` (the person view), `eventLine(ev, width)` (the
  rows of the event lists) and `placeInfo`. It lives in `tui` because
  `tui` imports `export`; `exportFormats()` appends it to `export.Formats`.
  Changes to the views show up in the PDF.
- **Nobody is marked:** `bookBlocks` works on a copy of the model without
  the reference person, so no ★ and no relationship lines are printed.
- **Layout:** A4, a grid of 96 columns of Martian Mono Condensed (0.6 em)
  at 9 pt; the views are built for 90 columns (88 plus the interface's
  2-column margin), and the last 8 columns hold page references.
  Characters are placed by terminal width (`ansi.StringWidth`), text is
  NFC-normalized first. Long lines are wrapped (`expand`), the page
  reference goes with the first row, and `paginate` keeps the rows of a
  wrapped line together.
  `paginate` starts each person on a new page and keeps headings
  (`line.heading`) and block starts together with at least two more lines.
- **Links:** `writePDF` adds a link and a bookmark per block (contents,
  people, places); lines that refer to a block print its page and link to
  it. The footer says "page n of m".
- **fpdf:** `codeberg.org/go-pdf/fpdf` is pinned to v0.11.1, because
  v0.12.0 requires Go 1.25. Its output is not byte-identical between runs:
  the font subset is built by iterating Go maps, so tests check the
  structure and the paginated lines instead of bytes.
- **Font coverage:** Martian Mono covers Latin-1, most of Latin Extended-A
  and Cyrillic, and the box drawing goged uses, but not ★ ⚭ ▸ ▾, Greek,
  Hebrew, Arabic or most Vietnamese letters, which print as boxes.

### tui

- One `Model` holds all views; each view has its own state struct and file.
  Overlays are help and licenses.
- **Keys:** digits switch views except while typing, so alt+N also works.
  Multi-rune key messages are split.
- **`docView`:** a scrollable list of lines, where lines with a target
  person or a query are links.
- **Lines, columns and wrapping:** a `span` with a `width` is a column; the
  spans after the last column flow. `line.layout(width, wrapped)` either
  cuts (columns padded or cut with "…", the rest runs on and `fit` cuts it
  at the screen edge, as the interface always did) or wraps: `flow` breaks
  at spaces and after hyphens within words, every column continues in its
  own place, flowing text continues where it began after its leading
  spaces. `expand` turns lines into rows marked `cont`; a wrapped link is
  one link in `docView` (selected and scrolled to together). The global
  `W` key (`Model.wrap`) wraps the person view; the PDF always wraps.
  Span literals are keyed (`span{text: …, style: …}`) since `width`
  exists.
- **Tree view:** generations range from 2 to 20 in both charts; the user
  raised the pedigree limit from 12 in `01f4fad`. The limits are separate
  constants, and switching charts clamps to the new chart's limit should
  they differ again. Horizontal scrolling cuts lines with `ansi.Cut`; see
  section 7 for why not `CutWc`.
- **Options:** `Title`, `StartPerson`, `Generations`, `ASCII`, `NoMouse`
  (read by `runTUI` in main.go), `Path` and `Load` (for reloading; not
  set for stdin), and `Today` (for tests of "On this day").
- **Mouse** (`mouse.go`): the program asks for cell motion events. A click
  selects and a click on the selection presses enter (`Model.press`
  replays keys, so the views keep one place for what enter does); a click
  outside a text input ends its editing as enter would. The click
  positions repeat the layout of the views' render functions (body rows
  start after `headerRows`; `exportRowY` mirrors `viewExport`); `tabs`
  and `renderTab` are shared by the tab bar and its clicks. The wheel
  scrolls by `wheelLines` and keeps the selection on screen
  (`scrollList`, `docView.scrollBy`); on the map it zooms around the
  pointer (`mapPoint`), and dragging pans it. The tests in
  `mouse_test.go` click on text found on the screen (`clickOn`).
- **Reloading** (`reload.go`): with `Options.Path` and `Load` (main.go
  passes the loader it used, so `-show-restricted` holds), a `tea.Tick`
  stats the file every second (`statFile` runs in the command, not in
  `Update`). A new size or modification time must stay the same for one
  more tick before the dialog asks: "Yes, reload", "Not this time" (asks
  again at the next version), "Never this session" (stops the ticks).
  Keys within `dialogGrace` (0.5 s) after it appears are ignored, as they
  were typed for the view (`n` or `N` in a search). The dialog is drawn
  over the body by `overlay`; `reloadDialog` lays it out for both drawing
  and clicks. Loading runs in a command; `replaceDoc` builds a new
  `Model` with `New` and carries over what still exists by ID or place
  name: history, marked person, query, filters, folded places, the map's
  view and selection, check thresholds, export settings, the view. The
  tests change a copy of family.ged (`rewrite`) and call `statFile`
  themselves (`poll`).

## 5. GEDCOM knowledge worth keeping

- **GEDCOM-L conventions:** the German-speaking vendors' agreements, as used
  by the sample file:
  - `_LOC` place records with `_GOV` ids and dated jurisdictions.
  - MARR TYPE CIVIL/RELI; `_RUFNAME`; `_GODP` and `_WITN` as text; `_ASSO`
    below events.
  - SPFX versus SURN for sorting; ADOP.FAMC.ADOP HUSB/WIFE/BOTH.
  - RESN confidential or privacy.
- **Who adopted:** with `PEDI adopted` on a FAMC, the ADOP event names the
  adopting partner. The other partner keeps the link from the child's other
  families: Markus's birth mother stays his birth mother in her second
  marriage. Without ADOP, the PEDI applies to both partners.
- **Association direction:** `1 ASSO @I2@ / 2 RELA Godparent` in I1's record
  means I2 is I1's godparent. The person view shows it as "Godparent" on I1
  and as "Godparent of" on I2.
- **Data errors in the sample**, reported to the user and not to be fixed in
  code:
  - Hans's note says born 1944, his BIRT says 1942.
  - Roswitha's christening is 1935, but her note says 1933.
  - Gerold's note has the typo "Aprl" and the wrong marriage date.
  - Karl Junior's postcode is 59846 in one place and 58846 in another; his
    emigration is dated 1 Oct in the note but 1 Sep in the event.
  - The F4 note spells "Guteledel"; England and Großbritannien are mixed.

## 6. Testing strategy

- **Coverage:** about 95–99% per package.
- **Data:** `testdata/family.ged` is small and fictional and covers the
  basics. The GEDCOM-L sample covers real-world conventions;
  `TestMusterFile` in `main_test.go` runs the batch flags against it.
- **Golden tests** for charts and CLI output use exact strings. When a
  deliberate behaviour change breaks one, update the expectation in the same
  commit and say so in the message.
- **TUI tests** (`tui/tui_test.go`):
  - An `app` harness sends key messages and checks `View()` with
    `contains`/`notContains`.
  - `newApp` loads family.ged at 100×30; `newMusterApp` loads the sample at
    110×60.
  - Resize when a view needs more room. The person view only shows what fits
    on screen.
- **Quick inspection:** a throwaway test file such as
  `tui/zz_dump_test.go`, gated by an environment variable, can print views
  while you develop. Delete it before committing.
- **Fuzz targets:**
  - `gedcom`: `FuzzParse` (which also runs `Redacted`, `ParentLinks`,
    associations, `Facts` and `Fit`), `FuzzParseDate` (asserts the `Fit`
    width), `FuzzParseLine`.
  - `search`: `FuzzParse`.
- **Bug fixes** come with a test that fails without the fix. Check that by
  temporarily reverting the fix.

## 7. Pitfalls found (and fixed)

- **`ansi.CutWc` is broken.** In x/ansi v0.11.6 it returns the wrong part of
  the string whenever the cut starts after column 0, which broke the tree
  view's horizontal scrolling. Use `ansi.Cut`.
- **Background-colour query.** termenv asks the terminal for its background
  colour (OSC 11) and waits up to 5 s if nothing answers, as with a
  recorder. With `TERM=screen*` or `tmux*` it skips the query. For
  recordings or captures, set `TERM=screen-256color`,
  `COLORTERM=truecolor` and `COLORFGBG=15;0`.
- **Bubble Tea and digits.** In the search box, digits are typed into the
  query, which is why view switching also works with alt+1–7.
- **Truncated labels.** Labels cut to a fixed width lost their life dates;
  hence `fitLabel`, and full labels in descendant charts.
- **Performance.** EventIndex recomputed sort keys inside the comparator, and
  `Fold` was slow without its per-rune cache. With 100k people, `tui.New`
  went from 3 s to 0.77 s.
- **Fold and ligatures.** NFD left ligatures unexpanded; NFKD expands them.
- **Dates:**
  - The Hebrew calendar epoch was off by one.
  - "0 JAN 1850" was accepted; days ≤ 0 are now rejected.
  - Approximate ages were truncated instead of rounded.
- **Terminalizer:**
  - `terminalizer generate` is not implemented in 0.12.0; it only prints a
    message.
  - `terminalizer render` starts Electron, which needs `--no-sandbox` as root
    and a display.
  - `terminalizer record` needs a real TTY on stdin. A Python `pty.fork`
    driver typing the keystrokes worked; the recording itself succeeded.

## 8. CI and release

- **`ci.yml`:**
  - Tests on Ubuntu, macOS and Windows; `-race` everywhere except Windows,
    whose runners lack cgo.
  - Lint: gofmt, vet, staticcheck 2025.1.1, `go mod tidy` diff.
  - Short fuzzing.
- **`release.yml`:** started by hand (`workflow_dispatch`).
  1. Runs the tests, then `scripts/build-release.sh` with `BUILD_DATE`.
  2. Publishes with `gh release` under the tag `YYYY-MM-DD`, replacing a
     release from the same day.
  - Binaries: `goged-DATE-{linux-amd64,linux-arm64,windows-amd64.exe,
    windows-arm64.exe,macos-universal}` plus `goged-DATE-SHA256SUMS.txt`.
  - `-version` prints `DATE-COMMIT`.
- **Latest release:** `2026-10-08` was built from `b6d970d`. Later commits
  (full descendant labels, 20 generations, the scroll fix) are not released
  yet.

## 9. Open ideas and loose ends

- **Not done yet:**
  - "On this day" only exists in the TUI, not in `-stats`; the output would
    change daily.
  - The README has no demo GIF yet; the user records one manually.
  - The module path versus the repository rename (`efi/GoGED`).
- **Demo plan** (about a minute at 100×30, with the sample file):
  1. Search `mustermann`, then `elton` (alternate name), then
     `born:1940..1945`.
  2. Open Max Manfred and mark him with `m`. Move to his son Erwin
     (6× down, enter), then to his grandson Leon (7× down, enter).
  3. Press `t` for the pedigree, move right 3× to Otto, press space to
     re-root, `d` for descendants and `+` for another generation.
  4. alt+4, then `f` and `type:marr`.
  5. alt+5, `f` and `brosowo`, down 4×, `M` for the map, then zoom out with
     `-`.
  6. alt+7, then `q`.
- **Possible future work** (never requested):
  - Media (OBJE) display.
  - Source and repository records as their own view.
  - Editing support.
  - GEDCOM 7 SNOTE and SCHMA specifics beyond reading notes.
  - Exporting charts.
