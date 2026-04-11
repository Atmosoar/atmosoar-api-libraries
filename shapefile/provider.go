// Package shapefile provides an embedded Natural Earth 1:110m country polygon lookup.
// The GeoJSON is parsed once at startup and stored in an in-memory map keyed by
// country name and ISO-A2/ISO-A3 codes (case-insensitive).
package shapefile

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"
)

//go:embed ne_110m_admin_0_countries.geojson
var embeddedGeoJSON []byte

// PointXY is a simple longitude/latitude pair used during GeoJSON parsing.
type PointXY struct {
	Lon float64
	Lat float64
}

// PolygonRings holds the outer ring (index 0) and optional inner rings (holes).
// Each ring is a slice of PointXY.
type PolygonRings = [][]PointXY

// CountryPolygon holds the polygon data for a single country.
// Rings contains all polygon rings: for a simple Polygon geometry the outer
// ring is at index 0 followed by any holes. For MultiPolygon geometries all
// sub-polygons' rings are flattened into this single slice (outer ring first
// per sub-polygon, then its holes).
type CountryPolygon struct {
	Name  string
	Rings PolygonRings
}

// ShapeProvider is the injectable provider for country polygon lookups.
type ShapeProvider struct {
	// lookup maps lowercase keys (NAME, ISO_A2, ISO_A3) to CountryPolygon.
	lookup map[string]*CountryPolygon
}

// geoJSONFeatureCollection is the top-level GeoJSON structure.
type geoJSONFeatureCollection struct {
	Features []geoJSONFeature `json:"features"`
}

type geoJSONFeature struct {
	Properties geoJSONProperties `json:"properties"`
	Geometry   geoJSONGeometry   `json:"geometry"`
}

type geoJSONProperties struct {
	Name    string `json:"NAME"`
	ISOA2   string `json:"ISO_A2"`
	ISOA3   string `json:"ISO_A3"`
	ISOA2EH string `json:"ISO_A2_EH"`
	ISOA3EH string `json:"ISO_A3_EH"`
}

type geoJSONGeometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

// NewShapeProvider parses the embedded GeoJSON and builds the lookup map.
// It panics if the embedded data is corrupt (programmer error, not runtime).
func NewShapeProvider(log *zap.SugaredLogger) *ShapeProvider {
	var fc geoJSONFeatureCollection
	if err := json.Unmarshal(embeddedGeoJSON, &fc); err != nil {
		panic(fmt.Sprintf("shapefile: failed to parse embedded GeoJSON: %v", err))
	}

	lookup := make(map[string]*CountryPolygon, len(fc.Features)*3)

	for _, feat := range fc.Features {
		rings, err := parseGeometry(feat.Geometry)
		if err != nil {
			if log != nil {
				log.Warnw("shapefile: skipping feature with unparseable geometry",
					"name", feat.Properties.Name,
					"error", err,
				)
			}
			continue
		}

		cp := &CountryPolygon{
			Name:  feat.Properties.Name,
			Rings: rings,
		}

		// Register under NAME.
		if feat.Properties.Name != "" {
			nameLower := strings.ToLower(feat.Properties.Name)
			lookup[nameLower] = cp

			// BUG-020: Also register under the name with spaces removed so that
			// compound config.toml keys like "UnitedKingdom" match "United Kingdom".
			nameNoSpaces := strings.ReplaceAll(nameLower, " ", "")
			if nameNoSpaces != nameLower {
				lookup[nameNoSpaces] = cp
			}
		}

		// Register under ISO_A2 (prefer ISO_A2_EH when ISO_A2 is -99).
		isoA2 := feat.Properties.ISOA2
		if isoA2 == "-99" || isoA2 == "" {
			isoA2 = feat.Properties.ISOA2EH
		}
		if isoA2 != "" && isoA2 != "-99" {
			lookup[strings.ToLower(isoA2)] = cp
		}

		// Register under ISO_A3 (prefer ISO_A3_EH when ISO_A3 is -99).
		isoA3 := feat.Properties.ISOA3
		if isoA3 == "-99" || isoA3 == "" {
			isoA3 = feat.Properties.ISOA3EH
		}
		if isoA3 != "" && isoA3 != "-99" {
			lookup[strings.ToLower(isoA3)] = cp
		}
	}

	// BUG-020: Register aliases for countries whose Natural Earth NAME differs
	// from common compound forms used in config.toml shortcuts.
	for _, alias := range countryAliases {
		target := strings.ToLower(alias.naturalEarthName)
		if cp, ok := lookup[target]; ok {
			lookup[strings.ToLower(alias.alias)] = cp
		}
	}

	if log != nil {
		log.Infow("shapefile: loaded country polygons",
			"features", len(fc.Features),
			"lookupKeys", len(lookup),
		)
	}

	return &ShapeProvider{lookup: lookup}
}

// countryAlias maps a common alternate name to its Natural Earth NAME property.
type countryAlias struct {
	alias            string // alternate name (e.g. "CzechRepublic")
	naturalEarthName string // NAME property in Natural Earth GeoJSON (e.g. "Czechia")
}

// countryAliases lists known mismatches between config.toml shortcut keys and
// Natural Earth country names. The space-stripped variant is already handled
// above; these cover cases where the name itself differs (not just spacing).
var countryAliases = []countryAlias{
	{alias: "CzechRepublic", naturalEarthName: "Czechia"},
	{alias: "Czech Republic", naturalEarthName: "Czechia"},
}

// Lookup finds a country polygon by name, ISO-A2 or ISO-A3 code (case-insensitive).
// Returns nil if no match is found.
func (sp *ShapeProvider) Lookup(name string) *CountryPolygon {
	if sp == nil {
		return nil
	}
	return sp.lookup[strings.ToLower(strings.TrimSpace(name))]
}

// parseGeometry converts a GeoJSON geometry into a flat slice of rings.
func parseGeometry(g geoJSONGeometry) (PolygonRings, error) {
	switch g.Type {
	case "Polygon":
		return parsePolygonCoords(g.Coordinates)
	case "MultiPolygon":
		return parseMultiPolygonCoords(g.Coordinates)
	default:
		return nil, fmt.Errorf("unsupported geometry type: %s", g.Type)
	}
}

// parsePolygonCoords parses [[[lon,lat], ...], ...] (one polygon with rings).
func parsePolygonCoords(raw json.RawMessage) (PolygonRings, error) {
	var coords [][][]float64
	if err := json.Unmarshal(raw, &coords); err != nil {
		return nil, fmt.Errorf("parsing polygon coordinates: %w", err)
	}

	rings := make(PolygonRings, 0, len(coords))
	for _, ring := range coords {
		pts := make([]PointXY, 0, len(ring))
		for _, coord := range ring {
			if len(coord) < 2 {
				continue
			}
			pts = append(pts, PointXY{Lon: coord[0], Lat: coord[1]})
		}
		rings = append(rings, pts)
	}
	return rings, nil
}

// parseMultiPolygonCoords parses [[[[lon,lat], ...], ...], ...] (multiple polygons).
func parseMultiPolygonCoords(raw json.RawMessage) (PolygonRings, error) {
	var multiCoords [][][][]float64
	if err := json.Unmarshal(raw, &multiCoords); err != nil {
		return nil, fmt.Errorf("parsing multi-polygon coordinates: %w", err)
	}

	var allRings PolygonRings
	for _, polygon := range multiCoords {
		for _, ring := range polygon {
			pts := make([]PointXY, 0, len(ring))
			for _, coord := range ring {
				if len(coord) < 2 {
					continue
				}
				pts = append(pts, PointXY{Lon: coord[0], Lat: coord[1]})
			}
			allRings = append(allRings, pts)
		}
	}
	return allRings, nil
}
