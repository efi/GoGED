# Tools and techniques for LLM agents working on goged

This file adds to [llms-info.md](llms-info.md), which describes the
project and the cloud container it was first built in. It records how an
agent worked on goged on the user's own Linux machine, where much of that
tooling is missing, and the techniques that proved useful there: a private
Go toolchain, probe programs outside the module, and text and image
captures of the running interface. It was written after commit `1170a54`.

`$SCRATCH` below stands for the session's scratch directory. It belongs to
one session, so everything placed there has to be set up again in the next.

## 1. The local machine

- **Hardware:** host "workhorse", Debian with a 2-core Intel Atom C2338 at
  1.74 GHz. Timings are several times those in llms-info.md: parsing a
  synthetic file with 100,000 people takes about 9 s here. Compare numbers
  measured on the same machine only.
- **Missing:** Go, the `gh` CLI, the fontconfig tools (`fc-list`,
  `fc-query`), and the Python modules PIL and fontTools.
- **Present:** git, curl, Python 3.13, tmux 3.5a, and textimg 3.2.3, which
  the user installed for screenshots.
- **Git identity:** none is configured. All commits so far are authored
  by `Claude <noreply@anthropic.com>`; keep that per command without
  changing the git configuration:

  ```sh
  git -c user.name=Claude -c user.email=noreply@anthropic.com commit -F msg.txt
  ```

- **No GitHub access:** `git fetch` fails from this machine, so the user
  pushes the commits. CI results are not visible here; ask the user about
  them.
- **Working directory:** the harness resets the shell's directory after a
  command that changes it. Use absolute paths.

## 2. A private Go toolchain

Use Go 1.24.2, the version in `go.mod` that CI uses through
`go-version-file`. Download it into the scratch directory and check it
against the checksum that go.dev publishes:

```sh
mkdir -p $SCRATCH/dl && cd $SCRATCH/dl
curl -sSfL -o go.tgz https://go.dev/dl/go1.24.2.linux-amd64.tar.gz
curl -sSf 'https://go.dev/dl/?mode=json&include=all' | python3 -I -c '
import json, sys
for r in json.load(sys.stdin):
    for f in r["files"]:
        if f["filename"] == "go1.24.2.linux-amd64.tar.gz":
            print(f["sha256"])'
sha256sum go.tgz        # must print the same checksum
tar -xzf go.tgz -C $SCRATCH
```

Shell variables do not survive from one command to the next, so start every
command that needs Go with:

```sh
export PATH=$SCRATCH/go/bin:$PATH GOPATH=$SCRATCH/gopath \
  GOMODCACHE=$SCRATCH/gopath/pkg/mod GOCACHE=$SCRATCH/gocache \
  GOTOOLCHAIN=local GOFLAGS=-modcacherw
```

`-modcacherw` keeps the module cache writable, so the scratch directory
can be deleted. Nothing is installed outside it.

## 3. Checks

These are the checks of the pre-push checklist in llms-info.md, section 3:

```sh
gofmt -l .                       # must print nothing
go vet ./...
go test ./...
go test -race ./...
go run honnef.co/go/tools/cmd/staticcheck@2025.1.1 ./...
go mod tidy && git diff --exit-code go.mod go.sum
go test -run '^$' -fuzz '^FuzzParse$' -fuzztime 20s ./gedcom   # after model changes
```

- **Staticcheck:** the first run downloads and builds it, which takes
  minutes on this machine. Long runs can go to the background, with the
  output written to a file in `$SCRATCH`.
- **Exit codes:** a pipe such as `cmd | tail` reports the exit code of
  `tail`. Read the output, or use `set -o pipefail`.
- **Fuzz corpus:** fuzzing writes its corpus to `GOCACHE`, not into the
  repository. Only a failing input lands in `testdata/fuzz/`; commit it
  together with the fix.

## 4. Probing without touching the repository

To try the packages from outside, for example to reproduce a suspected
bug before writing a test or to measure performance, use a separate module
in the scratch directory that points at the checkout:

```
// $SCRATCH/probe/go.mod
module probe

go 1.24.2

require github.com/efi/goged v0.0.0

replace github.com/efi/goged => /home/workhorse/claude/GoGED
```

Run it with `GOFLAGS="-modcacherw -mod=mod" go run .`; `-mod=mod` lets Go
fill in `go.sum` on the first run. Unlike the gated throwaway test file
from llms-info.md section 6, a probe cannot be committed by accident.

What probes were used for:

- **Bug reports:** reproducing them with `search.NewIndex(doc).SearchString(q)`,
  `gedcom.ParseString` with a small inline file, `doc.Warnings`, and
  `Date.IsValid`. A finding was reported as confirmed only after a probe
  had shown it.
- **Performance:**
  - A Go program generated a file with 100,000 people (2,000 founders,
    then couples with three children each, with dates, places and
    coordinates) and timed `ParseBytes`, `search.NewIndex`, `tui.New`,
    `genealogy.Places`, `PlaceNode.AllEvents`, `genealogy.Relate` and
    `World.Render`.
  - Running the same program with 25,000 people tells linear growth from
    quadratic growth.
  - CPU profiles come from `runtime/pprof` and
    `go tool pprof -top -cum BINARY FILE.prof`.

## 5. Proving that a test catches the bug

Each fix comes with a test that fails without it (llms-info.md section 6).
To check, stash only the fixed files, by path, so that the new test stays:

```sh
git stash push search/index.go search/query.go
go test ./search -run TestSearchNamesWithPunctuation   # must fail
git stash pop
```

Run the whole suite before every commit, so that each commit passes on its
own. Commits that only change documentation carry `[skip ci]` in the
message, which stops the CI workflow.

## 6. Driving the interface with tmux

Build a binary (`go build -o $SCRATCH/goged .`) and run it in a detached
tmux session. Use a separate server (`-L goged`) and no configuration
(`-f /dev/null`), so the user's own tmux is not affected:

```sh
cd /home/workhorse/claude/GoGED/testdata
tmux -L goged -f /dev/null new-session -d -s demo -x 124 -y 30 \
  "env TERM=screen-256color COLORTERM=truecolor COLORFGBG='15;0' $SCRATCH/goged Muster_GEDCOM_UTF-8.ged"
sleep 1.5
```

- **Environment:** `TERM=screen-256color` stops termenv from asking the
  terminal for its background colour and waiting 5 s for an answer
  (llms-info.md section 7). `COLORTERM=truecolor` gives 24-bit colours in
  colour captures, and `COLORFGBG='15;0'` declares a dark background.
- **Keys:** `tmux -L goged send-keys -t demo Down Enter`.
  - Key names: `Up`, `Down`, `Left`, `Right`, `Enter`, `Escape`, `Space`,
    `BSpace`, `Tab`, `BTab`, and `M-4` for alt+4, which switches views even
    while the search box takes the keys.
  - Text: `send-keys -l 'born:1930..1950'`.
  - Wait about 0.15 s between keys and 0.6 s before a capture.
- **Captures:** `tmux -L goged capture-pane -p -t demo` gives plain text;
  add `-e` to keep the colour sequences.
- **Size:** `tmux -L goged resize-window -t demo -x 122 -y 30`. The fourth
  generation of the sample's pedigree charts needs at least 122 columns;
  100 columns cut it off.
- **Finish:** `tmux -L goged send-keys -t demo q`, then
  `tmux -L goged kill-server`.

The tour used for the screenshots follows the demo plan in llms-info.md
section 9:

| View | Keys from the previous step |
|------|-----------------------------|
| Search | type `born:1930..1950` |
| Person (Max Manfred), marked | `Down`×7, `Enter`, `m` |
| Erwin, then Leon | `Down`×6, `Enter`; `Down`×7, `Enter` |
| Pedigree | `t` |
| Descendants of Otto, 5 generations | `Right`×3, `Space`, `d`, `+` |
| Marriages | `M-4`, `f`, type `type:marr`, `Enter` |
| Places | `M-5` |
| Events in Brosowo | `Down`×6, `Enter` |
| Map, zoom 4 | `Escape`, `M`, `-`×6 |
| Stats | `M-7` |
| Help | `?` |

## 7. Text captures

`capture-pane -p` output goes into fenced code blocks in the chat:

- Leave out the empty rows between the content and the key hints, and say
  so.
- Box-drawing and Braille characters display fine.
- Colours and the highlight on the selected entry are lost. In the tree
  view the header names the selected person instead.
- The `-ascii` option draws charts with ASCII characters only.

## 8. Images with textimg

textimg reads text with ANSI colour sequences, including 24-bit
`38;2;r;g;b`, from standard input and writes a PNG. It uses a single font
file without fallback; only emoji may come from a second font.

### Fonts

- **The installed fonts fall short.** DejaVu Sans Mono and Noto Sans Mono
  lack the Braille patterns of the map, which become boxes. DejaVu Sans
  Mono also lacks ⚭. DejaVu Sans has Braille but is proportional, so
  columns would not line up.
- **Use JuliaMono** (SIL Open Font License), downloaded into its own
  directory:

  ```sh
  mkdir -p $SCRATCH/fontdl $SCRATCH/fonts
  curl -sSfL -o $SCRATCH/fontdl/JuliaMono-ttf.tar.gz \
    https://github.com/cormullion/juliamono/releases/latest/download/JuliaMono-ttf.tar.gz
  tar -xzf $SCRATCH/fontdl/JuliaMono-ttf.tar.gz -C $SCRATCH/fonts JuliaMono-Regular.ttf LICENSE
  ```

- **Checking coverage:** without fontconfig, render a test line and look at
  the PNG (the Read tool shows images):

  ```sh
  printf '▸▾★◉●⚭ ─┌┤│└├ …·– ⢸⣀⡠⠤⠒⠊⠉ ↑↓←→ Müller\n' |
    textimg -f $SCRATCH/fonts/JuliaMono-Regular.ttf -F 16 -o $SCRATCH/test.png
  ```

### Taking a shot

This script sends keys, captures the pane and renders it. `-F 16` gives
8 px per column and 17 px per row, so 124×30 cells make an image of about
1000×544 pixels. textimg sizes the image to the longest line, so the
script adds a margin of two columns and one line on every side. The
`\x1b[0m` before the right margin keeps a highlighted row from extending
into it.

```sh
#!/usr/bin/env bash
# usage: shot.sh NAME [keys...]   keys are tmux key names; LIT:text types text
S=$(dirname "$0")
name=$1; shift
for k in "$@"; do
  case "$k" in
    LIT:*) tmux -L goged send-keys -t demo -l "${k#LIT:}" ;;
    *) tmux -L goged send-keys -t demo "$k" ;;
  esac
  sleep 0.15
done
sleep 0.7
tmux -L goged capture-pane -e -p -t demo > "$S/shots/$name.ans"
# A margin of two columns and one line on every side.
{ echo; sed 's/^/  /; s/$/\x1b[0m  /' "$S/shots/$name.ans"; echo; } |
  textimg -f "$S/fonts/JuliaMono-Regular.ttf" -F 16 -b 30,30,36,255 -g 220,220,220,255 -o "$S/shots/$name.png"
```

Put it at `$SCRATCH/shot.sh` and create `$SCRATCH/shots`. Then, for
example: `$SCRATCH/shot.sh 01-search "LIT:born:1930..1950"`,
`$SCRATCH/shot.sh 05-pedigree t`.

- **Check before sending:** look at every image, then send the set with
  the file-sending tool so that it is rendered.
- **Known flaws:**
  - textimg leaves small gaps between rows, so vertical tree lines look
    slightly broken.
  - JuliaMono draws ⚭ small.
  - The images differ in width, since each follows its longest line.
- **README screenshots:** the screenshots in the README were made
  differently, in the cloud container, with an ANSI-to-HTML conversion and
  Playwright (llms-info.md section 3).

## 9. Unicode escapes in tool input

- **The problem:** the tool layer decodes `\uXXXX` escapes before they
  reach a file or the shell. That happens in the Write and Edit tools
  (llms-info.md section 3), and also in Bash commands, heredocs and Python
  raw strings. A typed escape arrives as the raw character, so combining
  marks and format characters end up invisible in Go source.
- **The workaround:** write a placeholder such as `<u0308>` and let Python
  turn it into the Go escape, building the backslash with `chr(92)`, which
  the tool layer leaves alone. `search/unicode_test.go` was written this
  way, with this helper (`$SCRATCH/esc.py`):

```python
# Usage: python3 -I esc.py FILE  (reads edits from stdin)
# The edits are blocks "<<<OLD\n...\n===\n...\n>>>NEW" in which <uXXXX>
# stands for a Go escape (backslash-u), because the tool layer would turn
# a typed escape into the raw character. Each OLD must occur exactly once.
import re, sys
esc = lambda t: re.sub(r"<u([0-9A-F]{4})>", lambda m: chr(92) + "u" + m.group(1), t)
path = sys.argv[1]
src = open(path, encoding="utf-8").read()
for block in re.findall(r"<<<OLD\n(.*?)\n>>>NEW", sys.stdin.read(), re.S):
    old, new = block.split("\n===\n")
    old, new = esc(old), esc(new)
    if src.count(old) != 1:
        sys.exit(f"{path}: found {src.count(old)} times:\n{old}")
    src = src.replace(old, new)
open(path, "w", encoding="utf-8").write(src)
```

  Use it with a heredoc of edit blocks, each `<<<OLD`, the old text, `===`,
  the new text and `>>>NEW`:

  ```sh
  python3 -I $SCRATCH/esc.py search/unicode_test.go <<'EOF'
  <<<OLD
  ...old lines...
  ===
  ...new lines, with <u0308> where an escape belongs...
  >>>NEW
  EOF
  ```

- **Check the result:** count the escapes and the remaining non-ASCII
  characters:

  ```sh
  python3 -I -c 'import sys; s = open(sys.argv[1], encoding="utf-8").read()
  print(s.count(chr(92) + "u"), sum(ord(c) > 127 for c in s))' FILE
  ```

- **Raw strings:** Go raw strings (backticks) do not interpret escapes.
  Test data with escapes must be in interpreted strings, for example lines
  joined with `strings.Join`.
- **Messages:** `%+q` prints strings with escapes, so failing tests show
  which code points differ.

## 10. Validating exports

The GEXF export was checked against the official schema and with networkx
in a throwaway Python environment in the scratch directory:

- **Python packages:** Debian's Python has no `venv` module (no
  `ensurepip`), and the system Python is externally managed. The
  standalone virtualenv works:

  ```sh
  curl -sSfL -o $SCRATCH/pydl/virtualenv.pyz https://bootstrap.pypa.io/virtualenv.pyz
  python3 -I $SCRATCH/pydl/virtualenv.pyz -q $SCRATCH/venv
  $SCRATCH/venv/bin/pip install -q lxml networkx
  ```

- **The schema:** `gexf.xsd` includes five more files. gexf.net answers
  Python's default user agent with 403 Forbidden, so the fetcher sends one
  like curl's:

```python
# Fetch an XSD and everything it includes or imports into a directory.
import re, sys, urllib.request, os
base, outdir = sys.argv[1], sys.argv[2]
todo, seen = ["gexf.xsd"], set()
while todo:
    name = todo.pop()
    if name in seen:
        continue
    seen.add(name)
    data = urllib.request.urlopen(urllib.request.Request(base + name, headers={"User-Agent": "curl/8"}), timeout=30).read().decode("utf-8")
    open(os.path.join(outdir, name), "w", encoding="utf-8").write(data)
    for ref in re.findall(r'schemaLocation="([^"]+)"', data):
        if "://" not in ref:
            todo.append(ref)
print(sorted(seen))
```

  `python3 -I fetchxsd.py http://gexf.net/1.2draft/ $SCRATCH/xsd`

- **The check:** schema validation with lxml, then reading with networkx,
  which is stricter than the schema (it refuses undirected edges in a
  directed graph):

```python
# Validate GEXF files against the GEXF 1.2draft schema and load them with networkx.
import sys
from lxml import etree
import networkx as nx
schema = etree.XMLSchema(etree.parse(sys.argv[1]))
for path in sys.argv[2:]:
    doc = etree.parse(path)
    ok = schema.validate(doc)
    errors = "; ".join(f"line {e.line}: {e.message}" for e in list(schema.error_log)[:3])
    g = nx.read_gexf(path)
    kinds = {}
    for _, _, d in g.edges(data=True):
        kinds[d.get("relation")] = kinds.get(d.get("relation"), 0) + 1
    print(f"{path.rsplit('/', 1)[-1]}: schema {'valid' if ok else 'INVALID ' + errors}; networkx {type(g).__name__} "
          f"{g.number_of_nodes()} nodes, {g.number_of_edges()} edges {kinds}")
    n = next(iter(g.nodes(data=True)))
    print("   first node:", n)
```

  `$SCRATCH/venv/bin/python -I checkgexf.py $SCRATCH/xsd/gexf.xsd *.gexf`

- **Test files:** a probe program in the scratch directory (section 4)
  wrote exports of the sample files, a file with quotes, `&` and `<` in
  names, and one with control characters, which `encoding/xml` replaces
  with U+FFFD.
