package gedcom

import "strings"

// Location is a place record (_LOC) as agreed by the German-speaking
// genealogy software vendors (GEDCOM-L). Events refer to it with a _LOC
// pointer below PLAC, so that differently written places can be recognized
// as the same one and coordinates need to be recorded only once.
type Location struct {
	ID    string
	Node  *Node
	Names []LocationName // the first one is the primary name
	Type  string         // TYPE, e.g. "Stadtbezirk" or "39 Ort"
	GOV   string         // _GOV: identifier in the historic gazetteer GOV
	// Lat and Lon are the coordinates from MAP.LATI/LONG, valid if
	// HasCoords is set.
	Lat, Lon  float64
	HasCoords bool
	// Parents are the superior jurisdictions, possibly changing over time.
	Parents []LocationLink
}

// LocationName is one of the names of a location.
type LocationName struct {
	Name string
	Lang string // LANG, e.g. "German"
	Date Date   // DATE: when the name was used, if known
}

// LocationLink connects a location to a superior one.
type LocationLink struct {
	Location *Location
	Date     Date   // DATE: when the location belonged to the superior one
	Type     string // TYPE, e.g. "POLI" for political jurisdictions
}

// Name returns the primary name.
func (l *Location) Name() string {
	if len(l.Names) == 0 {
		return ""
	}
	return l.Names[0].Name
}

// GOVURL returns the address of the location in the GOV gazetteer, or "".
func (l *Location) GOVURL() string {
	if l.GOV == "" {
		return ""
	}
	return GOVURL(l.GOV)
}

// GOVURL returns the address of a GOV identifier in the gazetteer.
func GOVURL(id string) string { return "https://gov.genealogy.net/item/show/" + id }

// Location returns the location record with the given identifier, or nil.
func (d *Document) Location(id string) *Location { return d.locations[normalizeID(id)] }

// buildLocation reads the names, coordinates and type of a _LOC record;
// links to superior locations are resolved by linkLocations.
func (d *Document) buildLocation(l *Location) {
	for _, c := range l.Node.Children {
		switch c.Tag {
		case "NAME":
			if name := strings.TrimSpace(c.Value); name != "" {
				l.Names = append(l.Names, LocationName{Name: name, Lang: strings.TrimSpace(c.Val("LANG")), Date: ParseDate(c.Val("DATE"))})
			}
		case "TYPE":
			if l.Type == "" {
				l.Type = strings.TrimSpace(c.Value)
			}
		case "_GOV":
			if l.GOV == "" {
				l.GOV = strings.TrimSpace(c.Value)
			}
		case "MAP":
			if !l.HasCoords {
				l.Lat, l.Lon, l.HasCoords = coordinates(c)
			}
		}
	}
}

func (d *Document) linkLocations() {
	for _, l := range d.Locations {
		for _, c := range l.Node.Children {
			if c.Tag != "_LOC" || !c.IsPointer() {
				continue
			}
			sup := d.locations[StripXref(c.Value)]
			if sup == nil {
				d.warn(c.Line, "place %s refers to missing place %s", l.ID, c.Value)
				continue
			}
			l.Parents = append(l.Parents, LocationLink{Location: sup, Date: ParseDate(c.Val("DATE")), Type: strings.TrimSpace(c.Val("TYPE"))})
		}
	}
}

// coordinates reads a MAP structure.
func coordinates(m *Node) (lat, lon float64, ok bool) {
	lat, ok1 := ParseCoordinate(m.Val("LATI"), 'N', 'S', 90)
	lon, ok2 := ParseCoordinate(m.Val("LONG"), 'E', 'W', 180)
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	return lat, lon, true
}
