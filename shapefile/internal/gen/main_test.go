package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

// TestEmbeddedFileIsGenerated fails when the embedded GeoJSON carries more
// than strip keeps, i.e. someone replaced it without running go generate.
func TestEmbeddedFileIsGenerated(t *testing.T) {
	embedded, err := os.ReadFile("../../ne_110m_admin_0_countries.geojson")
	if err != nil {
		t.Fatal(err)
	}
	got, err := strip(embedded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, embedded) {
		t.Fatal("embedded GeoJSON is not stripped; run go generate ./shapefile/")
	}
}

func TestStripKeepsOnlyLookupProperties(t *testing.T) {
	in := []byte(`{"type":"FeatureCollection","name":"x","features":[
	  {"type":"Feature","bbox":[0,0,1,1],
	   "properties":{"ADMIN":"Atlantis","NAME":"Atlantis","ISO_A2":"-99","ISO_A3":"-99","ISO_A2_EH":"AT","ISO_A3_EH":"ATL","POP_EST":1},
	   "geometry":{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,0]]]}}]}`)
	out, err := strip(in)
	if err != nil {
		t.Fatal(err)
	}
	var fc struct {
		Features []struct {
			Properties map[string]any  `json:"properties"`
			Geometry   json.RawMessage `json:"geometry"`
			BBox       any             `json:"bbox"`
		} `json:"features"`
	}
	if err := json.Unmarshal(out, &fc); err != nil {
		t.Fatal(err)
	}
	f := fc.Features[0]
	if len(f.Properties) != len(keptProperties) || f.Properties["ISO_A3_EH"] != "ATL" || f.Properties["ADMIN"] != nil {
		t.Fatalf("unexpected properties: %v", f.Properties)
	}
	if f.BBox != nil {
		t.Fatalf("bbox should be dropped")
	}
	if string(f.Geometry) != `{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,0]]]}` {
		t.Fatalf("geometry changed: %s", f.Geometry)
	}
}
