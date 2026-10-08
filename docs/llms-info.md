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

Main features, by TUI tab (keys 1–7, or alt+1–7 while typing):

1. **Search**: query language with fields, ranges, negation, phonetic matching.
2. **Person**: parents, siblings, families, associations, timeline, notes,
   sources; every relative is a link; browser-like back and forward.
3. **Tree**: horizontal pedigree chart or indented descendant chart.
4. **Events**: all or the vital events, chronological, with a filter.
5. **Places**: jurisdiction hierarchy, events per place.
6. **Map** (experimental): Braille world map with markers, using embedded
   Natural Earth data.
7. **Stats**: counts, time span, names, "On this day", warnings.

The batch flags are `-q`, `-tree`, `-desc`, `-timeline`, `-relate ID1,ID2`,
`-events FILTER` (plus `-all`), `-places`, `-stats`, `-gen`, `-ascii`,
`-person`, `-show-restricted`, `-version` and `-licenses`. Passing `-` as
the file reads from stdin.

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
  column widths per generation.
- **`Descendants`:** an indented tree with spouse lines. It marks
  non-birth children "(adopted)", and does not expand a person a second time
  ("(see above)").
- **Nodes** carry `Line`, `Col`, `Width`, `Up`, `Down` and `More`, so the
  TUI can select and navigate them.
- **Label widths:** `fitLabel` shortens the name, not the life dates. The TUI
  passes an unreachable limit for descendant charts (full labels) and 36 for
  pedigrees. main does the same for `-desc`.

### worldmap

- `world.bin` (about 1.4 MB) is Natural Earth 1:10m data. The format is
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

### tui

- One `Model` holds all views; each view has its own state struct and file.
  Overlays are help and licenses.
- **Keys:** digits switch views except while typing, so alt+N also works.
  Multi-rune key messages are split.
- **`docView`:** a scrollable list of lines, where lines with a target
  person or a query are links.
- **Tree view:** generations range from 2–12 for pedigrees and 2–20 for
  descendant charts, and switching to a pedigree clamps to 12. Horizontal
  scrolling cuts lines with `ansi.Cut`; see section 7 for why not `CutWc`.
- **Options:** `Title`, `StartPerson`, `Generations`, `ASCII`, and `Today`
  (for tests of "On this day").

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
  - The pedigree limit of 12 generations; descendant charts allow 20.
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
