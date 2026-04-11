package shapefile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewShapeProvider(t *testing.T) {
	sp := NewShapeProvider(nil)
	require.NotNil(t, sp)
	assert.Greater(t, len(sp.lookup), 0, "lookup map should have entries")
}

func TestLookup_ByName(t *testing.T) {
	sp := NewShapeProvider(nil)

	tests := []struct {
		name  string
		query string
	}{
		{"Germany by name", "Germany"},
		{"Germany lowercase", "germany"},
		{"Germany uppercase", "GERMANY"},
		{"France by name", "France"},
		{"France lowercase", "france"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cp := sp.Lookup(tt.query)
			require.NotNil(t, cp, "expected match for %q", tt.query)
			assert.NotEmpty(t, cp.Rings, "should have at least one ring")
			assert.NotEmpty(t, cp.Rings[0], "outer ring should have points")
		})
	}
}

func TestLookup_ByISO(t *testing.T) {
	sp := NewShapeProvider(nil)

	tests := []struct {
		name  string
		query string
		want  string
	}{
		{"Germany ISO_A2", "DE", "Germany"},
		{"Germany ISO_A3", "DEU", "Germany"},
		{"France ISO_A2 via EH fallback", "FR", "France"},
		{"France ISO_A3 via EH fallback", "FRA", "France"},
		{"Norway ISO_A2 via EH fallback", "NO", "Norway"},
		{"United Kingdom ISO_A2", "GB", "United Kingdom"},
		{"United Kingdom ISO_A3", "GBR", "United Kingdom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cp := sp.Lookup(tt.query)
			require.NotNil(t, cp, "expected match for ISO code %q", tt.query)
			assert.Equal(t, tt.want, cp.Name)
		})
	}
}

func TestLookup_CaseInsensitive(t *testing.T) {
	sp := NewShapeProvider(nil)

	// ISO codes should be case-insensitive too.
	assert.NotNil(t, sp.Lookup("de"))
	assert.NotNil(t, sp.Lookup("De"))
	assert.NotNil(t, sp.Lookup("deu"))
	assert.NotNil(t, sp.Lookup("DEU"))
}

// BUG-020: TestLookup_CompoundNames tests that compound country names (no spaces)
// match their Natural Earth equivalents.
func TestLookup_CompoundNames(t *testing.T) {
	sp := NewShapeProvider(nil)

	tests := []struct {
		name  string
		query string
		want  string
	}{
		{"UnitedKingdom (no spaces)", "UnitedKingdom", "United Kingdom"},
		{"unitedkingdom (lowercase)", "unitedkingdom", "United Kingdom"},
		{"CzechRepublic (no spaces)", "CzechRepublic", "Czechia"},
		{"czechrepublic (lowercase)", "czechrepublic", "Czechia"},
		{"Czech Republic (with space)", "Czech Republic", "Czechia"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cp := sp.Lookup(tt.query)
			require.NotNil(t, cp, "expected match for %q", tt.query)
			assert.Equal(t, tt.want, cp.Name)
			assert.NotEmpty(t, cp.Rings, "should have at least one ring")
		})
	}
}

func TestLookup_NotFound(t *testing.T) {
	sp := NewShapeProvider(nil)
	assert.Nil(t, sp.Lookup("Atlantis"))
	assert.Nil(t, sp.Lookup(""))
	assert.Nil(t, sp.Lookup("ZZZZZ"))
}

func TestLookup_NilProvider(t *testing.T) {
	var sp *ShapeProvider
	assert.Nil(t, sp.Lookup("Germany"))
}

func TestLookup_MultiPolygon(t *testing.T) {
	sp := NewShapeProvider(nil)

	// France has MultiPolygon geometry in Natural Earth 110m — multiple rings expected.
	cp := sp.Lookup("France")
	require.NotNil(t, cp)
	assert.Greater(t, len(cp.Rings), 1,
		"France (MultiPolygon) should have more than one ring")
}

func TestLookup_PolygonPoints(t *testing.T) {
	sp := NewShapeProvider(nil)

	// Germany should be a simple Polygon with one outer ring.
	cp := sp.Lookup("Germany")
	require.NotNil(t, cp)
	require.NotEmpty(t, cp.Rings)

	// Verify points have reasonable lat/lon for Germany.
	for _, pt := range cp.Rings[0] {
		assert.True(t, pt.Lat > 40 && pt.Lat < 60,
			"Germany lat should be roughly 47-55, got %f", pt.Lat)
		assert.True(t, pt.Lon > 4 && pt.Lon < 16,
			"Germany lon should be roughly 6-15, got %f", pt.Lon)
	}
}
