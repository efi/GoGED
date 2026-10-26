package export

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/efi/goged/gedcom"
)

func load(t *testing.T, name string) *gedcom.Document {
	t.Helper()
	doc, err := gedcom.ParseFile(filepath.Join("..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func parse(t *testing.T, ged string) *gedcom.Document {
	t.Helper()
	doc, err := gedcom.ParseString(ged)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// twoFamilies has two unrelated families, a person without relatives, and
// two siblings whose parents are unknown.
const twoFamilies = `0 HEAD
0 @A1@ INDI
1 NAME Adam /Ash/
1 SEX M
0 @A2@ INDI
1 NAME Alma /Ash/
1 SEX F
0 @A3@ INDI
1 NAME Arno /Ash/
1 FAMC @FA@
2 PEDI adopted
0 @B1@ INDI
1 NAME Bert /Birch/
0 @B2@ INDI
1 NAME Bea /Birch/
0 @L@ INDI
1 NAME Lone /Single/
0 @S1@ INDI
1 NAME Sam /Sib/
0 @S2@ INDI
1 NAME Sue /Sib/
0 @FA@ FAM
1 HUSB @A1@
1 WIFE @A2@
1 CHIL @A3@
0 @FB@ FAM
1 HUSB @B1@
1 CHIL @B2@
0 @FS@ FAM
1 CHIL @S1@
1 CHIL @S2@
0 TRLR
`

func ids(people []*gedcom.Individual) string {
	var out []string
	for _, p := range people {
		out = append(out, p.ID)
	}
	return strings.Join(out, ",")
}

func linkText(l Link) string {
	s := l.From.ID + "-" + l.To.ID + "@" + l.Family.ID
	if l.Kind == Parent {
		s = l.From.ID + ">" + l.To.ID + "@" + l.Family.ID
		if l.Pedigree != gedcom.PedigreeUnknown {
			s += "(" + string(l.Pedigree) + ")"
		}
	}
	return s
}

func linkList(sel *Selection) string {
	var out []string
	for _, l := range sel.Links {
		out = append(out, linkText(l))
	}
	return strings.Join(out, " ")
}

func TestSelectAll(t *testing.T) {
	doc := parse(t, twoFamilies)
	sel, err := Select(doc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(sel.People); got != "A1,A2,A3,B1,B2,L,S1,S2" {
		t.Errorf("people = %s", got)
	}
	// Partners, then each partner with each child; siblings are not linked.
	if got := linkList(sel); got != "A1-A2@FA A1>A3@FA(adopted) A2>A3@FA(adopted) B1>B2@FB" {
		t.Errorf("links = %s", got)
	}
	if !sel.Has(doc.Individual("L")) {
		t.Error("people without relatives belong to the whole file")
	}
}

func TestSelectComponent(t *testing.T) {
	doc := parse(t, twoFamilies)
	for _, tt := range []struct {
		root, people, links string
	}{
		{"A3", "A1,A2,A3", "A1-A2@FA A1>A3@FA(adopted) A2>A3@FA(adopted)"},
		{"A1", "A1,A2,A3", "A1-A2@FA A1>A3@FA(adopted) A2>A3@FA(adopted)"},
		{"B2", "B1,B2", "B1>B2@FB"},
		{"L", "L", ""},
		{"S1", "S1", ""}, // a sibling is connected through parents only
	} {
		sel, err := Select(doc, Options{Scope: Component, Root: doc.Individual(tt.root)})
		if err != nil {
			t.Fatal(err)
		}
		if got := ids(sel.People); got != tt.people {
			t.Errorf("%s: people = %s, want %s", tt.root, got, tt.people)
		}
		if got := linkList(sel); got != tt.links {
			t.Errorf("%s: links = %s, want %s", tt.root, got, tt.links)
		}
	}

	// The GEDCOM-L sample: Max Manfred is connected to all but four people.
	muster := load(t, "Muster_GEDCOM_UTF-8.ged")
	all, _ := Select(muster, Options{})
	sel, err := Select(muster, Options{Scope: Component, Root: muster.Individual("I1")})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.People) != 37 || len(sel.People) != 33 || len(sel.Links) != 50 {
		t.Errorf("all %d, component %d people and %d links", len(all.People), len(sel.People), len(sel.Links))
	}
	for _, l := range all.Links {
		if sel.Has(l.From) != sel.Has(l.To) {
			t.Errorf("link %s leaves the component", linkText(l))
		}
	}
}

func TestSelectWithoutRoot(t *testing.T) {
	doc := parse(t, twoFamilies)
	other := parse(t, twoFamilies)
	for _, root := range []*gedcom.Individual{nil, other.Individual("A1")} {
		if _, err := Select(doc, Options{Scope: Component, Root: root}); !errors.Is(err, ErrNoRoot) {
			t.Errorf("root %v: err = %v", root, err)
		}
	}
}

func TestWriteGEXF(t *testing.T) {
	doc := parse(t, `0 HEAD
0 @I1@ INDI
1 NAME Tom "Tommy" /Smith & Sons <Ltd>/
1 SEX M
1 BIRT
2 DATE ABT 1850
2 PLAC Leeds
1 BURI
2 PLAC York
0 @I2@ INDI
1 NAME Ann /O'Neil/
1 SEX F
1 DEAT
2 DATE 3 JAN 1920
0 @I3@ INDI
1 NAME Kit /Smith/
1 FAMC @F1@
2 PEDI foster
0 @I4@ INDI
1 NAME Jan /Berg/
2 SPFX van der
0 @I5@ INDI
1 NAME /Nobody/
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 CHIL @I3@
1 MARR
2 DATE 12 MAR 1875
0 TRLR
`)
	opts := Options{Title: "smith.ged", Generated: time.Date(2026, 10, 12, 9, 30, 0, 0, time.UTC)}
	sel, err := Select(doc, opts)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := WriteGEXF(&b, sel, opts); err != nil {
		t.Fatal(err)
	}
	want := `<?xml version="1.0" encoding="UTF-8"?>
<gexf xmlns="http://www.gexf.net/1.2draft" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="http://www.gexf.net/1.2draft http://www.gexf.net/1.2draft/gexf.xsd" version="1.2">
  <meta lastmodifieddate="2026-10-12">
    <creator>goged</creator>
    <description>All 5 people of smith.ged and their relationships</description>
  </meta>
  <graph mode="static" defaultedgetype="directed">
    <attributes class="node">
      <attribute id="firstname" title="first name" type="string"></attribute>
      <attribute id="lastname" title="last name" type="string"></attribute>
      <attribute id="sex" title="sex" type="string"></attribute>
      <attribute id="birth" title="birth" type="string"></attribute>
      <attribute id="birthyear" title="birth year" type="integer"></attribute>
      <attribute id="birthplace" title="birth place" type="string"></attribute>
      <attribute id="death" title="death" type="string"></attribute>
      <attribute id="deathyear" title="death year" type="integer"></attribute>
      <attribute id="deathplace" title="death place" type="string"></attribute>
    </attributes>
    <attributes class="edge">
      <attribute id="relation" title="relation" type="string"></attribute>
      <attribute id="pedigree" title="pedigree" type="string"></attribute>
      <attribute id="family" title="family" type="string"></attribute>
      <attribute id="marriageyear" title="marriage year" type="integer"></attribute>
    </attributes>
    <nodes>
      <node id="I1" label="Tom &#34;Tommy&#34; Smith &amp; Sons &lt;Ltd&gt;">
        <attvalues>
          <attvalue for="firstname" value="Tom &#34;Tommy&#34;"></attvalue>
          <attvalue for="lastname" value="Smith &amp; Sons &lt;Ltd&gt;"></attvalue>
          <attvalue for="sex" value="male"></attvalue>
          <attvalue for="birth" value="about 1850"></attvalue>
          <attvalue for="birthyear" value="1850"></attvalue>
          <attvalue for="birthplace" value="Leeds"></attvalue>
          <attvalue for="deathplace" value="York"></attvalue>
        </attvalues>
      </node>
      <node id="I2" label="Ann O&#39;Neil">
        <attvalues>
          <attvalue for="firstname" value="Ann"></attvalue>
          <attvalue for="lastname" value="O&#39;Neil"></attvalue>
          <attvalue for="sex" value="female"></attvalue>
          <attvalue for="death" value="3 Jan 1920"></attvalue>
          <attvalue for="deathyear" value="1920"></attvalue>
        </attvalues>
      </node>
      <node id="I3" label="Kit Smith">
        <attvalues>
          <attvalue for="firstname" value="Kit"></attvalue>
          <attvalue for="lastname" value="Smith"></attvalue>
          <attvalue for="sex" value="unknown"></attvalue>
        </attvalues>
      </node>
      <node id="I4" label="Jan van der Berg">
        <attvalues>
          <attvalue for="firstname" value="Jan"></attvalue>
          <attvalue for="lastname" value="van der Berg"></attvalue>
          <attvalue for="sex" value="unknown"></attvalue>
        </attvalues>
      </node>
      <node id="I5" label="Nobody">
        <attvalues>
          <attvalue for="lastname" value="Nobody"></attvalue>
          <attvalue for="sex" value="unknown"></attvalue>
        </attvalues>
      </node>
    </nodes>
    <edges>
      <edge id="0" source="I1" target="I2" type="mutual" label="partners">
        <attvalues>
          <attvalue for="relation" value="partners"></attvalue>
          <attvalue for="family" value="F1"></attvalue>
          <attvalue for="marriageyear" value="1875"></attvalue>
        </attvalues>
      </edge>
      <edge id="1" source="I1" target="I3" label="father">
        <attvalues>
          <attvalue for="relation" value="parent"></attvalue>
          <attvalue for="pedigree" value="foster"></attvalue>
          <attvalue for="family" value="F1"></attvalue>
        </attvalues>
      </edge>
      <edge id="2" source="I2" target="I3" label="mother">
        <attvalues>
          <attvalue for="relation" value="parent"></attvalue>
          <attvalue for="pedigree" value="foster"></attvalue>
          <attvalue for="family" value="F1"></attvalue>
        </attvalues>
      </edge>
    </edges>
  </graph>
</gexf>
`
	if got := b.String(); got != want {
		t.Errorf("GEXF:\n%s\nwant:\n%s", got, want)
	}

	// The component's description names the root; nothing is dated
	// without a date.
	opts = Options{Scope: Component, Root: doc.Individual("I3")}
	sel, _ = Select(doc, opts)
	b.Reset()
	if err := WriteGEXF(&b, sel, opts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "<description>3 people of a GEDCOM file connected to Kit Smith (I3) and their relationships</description>") ||
		strings.Contains(b.String(), "lastmodifieddate") {
		t.Errorf("component meta:\n%s", b.String()[:400])
	}
}

// gexfFile is the part of a GEXF file the tests read back.
type gexfFile struct {
	Attributes []struct {
		Class string `xml:"class,attr"`
		List  []struct {
			ID string `xml:"id,attr"`
		} `xml:"attribute"`
	} `xml:"graph>attributes"`
	Nodes []struct {
		ID     string `xml:"id,attr"`
		Label  string `xml:"label,attr"`
		Values []struct {
			For string `xml:"for,attr"`
		} `xml:"attvalues>attvalue"`
	} `xml:"graph>nodes>node"`
	Edges []struct {
		Source string `xml:"source,attr"`
		Target string `xml:"target,attr"`
		Values []struct {
			For string `xml:"for,attr"`
		} `xml:"attvalues>attvalue"`
	} `xml:"graph>edges>edge"`
}

func TestWriteGEXFReadsBack(t *testing.T) {
	doc := load(t, "Muster_GEDCOM_UTF-8.ged")
	opts := Options{Scope: Component, Root: doc.Individual("I1")}
	sel, _ := Select(doc, opts)
	var b bytes.Buffer
	if err := WriteGEXF(&b, sel, opts); err != nil {
		t.Fatal(err)
	}
	var g gexfFile
	if err := xml.Unmarshal(b.Bytes(), &g); err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != len(sel.People) || len(g.Edges) != len(sel.Links) {
		t.Fatalf("%d nodes, %d edges", len(g.Nodes), len(g.Edges))
	}
	declared := map[string]bool{}
	for _, a := range g.Attributes {
		for _, attr := range a.List {
			declared[a.Class+":"+attr.ID] = true
		}
	}
	nodes := map[string]bool{}
	for _, n := range g.Nodes {
		nodes[n.ID] = true
		for _, v := range n.Values {
			if !declared["node:"+v.For] {
				t.Errorf("node %s: undeclared attribute %s", n.ID, v.For)
			}
		}
	}
	for _, e := range g.Edges {
		if !nodes[e.Source] || !nodes[e.Target] {
			t.Errorf("edge %s-%s leads outside the graph", e.Source, e.Target)
		}
		for _, v := range e.Values {
			if !declared["edge:"+v.For] {
				t.Errorf("edge %s-%s: undeclared attribute %s", e.Source, e.Target, v.For)
			}
		}
	}
	if g.Nodes[0].Label != "Max Manfred Mustermann" {
		t.Errorf("first node = %+v", g.Nodes[0])
	}
}

// failAfter writes n bytes, then fails.
type failAfter struct{ n int }

func (f *failAfter) Write(p []byte) (int, error) {
	if len(p) > f.n {
		return f.n, errors.New("disk full")
	}
	f.n -= len(p)
	return len(p), nil
}

func TestWriteFile(t *testing.T) {
	doc := parse(t, twoFamilies)
	sel, _ := Select(doc, Options{})
	dir := t.TempDir()
	path := filepath.Join(dir, "two.gexf")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, Formats[0], sel, Options{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(data, []byte(`<node id="L" label="Lone Single">`)) {
		t.Fatalf("file: %v\n%s", err, data)
	}
	if info, _ := os.Stat(path); runtime.GOOS != "windows" && info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v", info.Mode())
	}

	// A failed export leaves the file as it was and no temporary file.
	broken := Format{Name: "broken", Extension: ".x", Write: func(w io.Writer, sel *Selection, opts Options) error {
		_, err := io.WriteString(&failAfter{10}, "partial content")
		return err
	}}
	if err := WriteFile(path, broken, sel, Options{}); err == nil {
		t.Error("the error was lost")
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(after, data) {
		t.Error("a failed export changed the file")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("left behind: %v", entries)
	}
	if err := WriteFile(filepath.Join(dir, "missing", "x.gexf"), Formats[0], sel, Options{}); err == nil {
		t.Error("writing into a missing directory succeeded")
	}
}

func TestFormats(t *testing.T) {
	if len(Formats) == 0 || Formats[0].Name != "GEXF" || Formats[0].Extension != ".gexf" || Formats[0].Write == nil {
		t.Errorf("formats = %+v", Formats)
	}
}
