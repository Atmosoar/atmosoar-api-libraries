// Command gen rewrites the embedded Natural Earth country GeoJSON down to what
// shapefile.NewShapeProvider reads: the NAME and ISO code properties and the
// geometry, minified. Natural Earth ships 168 properties per feature; every
// consumer binary would otherwise embed and scan them all at startup.
//
// Source: Natural Earth 1:110m Cultural Vectors, Admin 0 - Countries
// (ne_110m_admin_0_countries, public domain). To refresh from upstream, pass
// the downloaded GeoJSON as -in; with no flags it re-strips the embedded file,
// which is a no-op once the file is generated.
//
//	go generate ./shapefile/
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

// keptProperties are the properties shapefile.geoJSONProperties decodes.
var keptProperties = []string{"NAME", "ISO_A2", "ISO_A3", "ISO_A2_EH", "ISO_A3_EH"}

type collection struct {
	Type     string          `json:"type"`
	Name     string          `json:"name,omitempty"`
	CRS      json.RawMessage `json:"crs,omitempty"`
	Features []feature       `json:"features"`
}

type feature struct {
	Type       string          `json:"type"`
	Properties properties      `json:"properties"`
	Geometry   json.RawMessage `json:"geometry"`
}

// properties marshals the kept keys in a fixed order.
type properties map[string]json.RawMessage

func (p properties) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	first := true
	for _, k := range keptProperties {
		v, ok := p[k]
		if !ok {
			continue
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		kb, _ := json.Marshal(k)
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(v)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// strip keeps the collection header, the kept properties and the geometry of
// every feature, and returns minified JSON with a trailing newline.
func strip(in []byte) ([]byte, error) {
	var fc collection
	if err := json.Unmarshal(in, &fc); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	for i, f := range fc.Features {
		kept := make(properties, len(keptProperties))
		for _, k := range keptProperties {
			if v, ok := f.Properties[k]; ok {
				kept[k] = v
			}
		}
		fc.Features[i].Properties = kept
	}
	out, err := json.Marshal(fc)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, out); err != nil {
		return nil, fmt.Errorf("compact: %w", err)
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

func main() {
	in := flag.String("in", "ne_110m_admin_0_countries.geojson", "source GeoJSON")
	out := flag.String("out", "ne_110m_admin_0_countries.geojson", "embedded GeoJSON to write")
	flag.Parse()

	src, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	dst, err := strip(src)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, dst, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
