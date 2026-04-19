// Package location provides parsers and expansion helpers for location query
// parameters used by Atmosoar services (points, polylines, rectangles,
// bounding boxes, polygons, WMO station shortcuts, and named country
// shortcuts).
//
// The package exposes several package-level registries (LocationShortcutsMap,
// StationMap, ActiveShapeProvider) that consumers populate at service startup
// from their own configuration. This mirrors the original in-tree API in
// MMA so the migration is a drop-in import swap.
package location

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	"atmosoar.io/atmosoar-api-libraries/shapefile"
)

// Location represents different location types
type Location struct {
	Type    LocationType `json:"type"`
	Payload interface{}  `json:"payload"` // Actual location data
}

// LocationType is a type to not use a raw string for readeablity.
// Name preserved from MMA-171's in-tree implementation so the cutover is a pure import swap.
//
//nolint:revive // parity: renaming would break the MMA-171 drop-in swap contract.
type LocationType string

const (
	// LocationTypePoint for one singular point
	LocationTypePoint LocationType = "point"
	// LocationTypePointList for list of Locations
	LocationTypePointList LocationType = "point_list"
	// LocationTypePolyline for polyline of locations
	LocationTypePolyline LocationType = "polyline"
	// LocationTypeRectangle for an area
	LocationTypeRectangle LocationType = "rectangle"
	// LocationTypeRadius for a circular area around a point (km)
	LocationTypeRadius LocationType = "radius"
	// LocationTypePolygon for a country polygon boundary
	LocationTypePolygon LocationType = "polygon"
	// LocationTypeBbox for a bounding box used by map overlay grid requests (MMA-163)
	LocationTypeBbox LocationType = "bbox"
)

// Location payloads

// PointLocation for one singular point
// in case of a shortcut or a station they will be resolved to this too
type PointLocation struct {
	Lat float64 `json:"lat" validate:"required"`
	Lon float64 `json:"lon" validate:"required"`
}

// PointLine is defined by the beginning, end and resolution, meaning the amount of points inbetween
type PointLine struct {
	StartPoint PointLocation    `json:"start"`
	EndPoint   PolylineLocation `json:"end"`
	Resolution int16            `json:"resolution"`
}

// PointListLocation list of Locations
type PointListLocation struct {
	Points []PointLocation `json:"points" validate:"required"`
}

// PolylineLocation polyline of locations
type PolylineLocation struct {
	Coordinates []PointLocation `json:"polyline" validate:"required"`
}

// RectangleLocation an area
type RectangleLocation struct {
	NorthEast PointLocation `json:"north_east" validate:"required"`
	SouthWest PointLocation `json:"south_west" validate:"required"`
}

// RadiusLocation an point with radius in km
type RadiusLocation struct {
	Lat    float64 `json:"lat"    validate:"required"`
	Lon    float64 `json:"lon"    validate:"required"`
	Radius float64 `json:"radius" validate:"required,gt=0"`
}

// PolygonLocation represents a country polygon boundary.
// Rings[0] is the outer ring; subsequent rings are holes (interior rings).
// For multi-polygon geometries all sub-polygons' rings are flattened here.
type PolygonLocation struct {
	Rings [][]PointLocation `json:"rings"`
}

// BboxLocation represents a bounding box for map overlay grid requests (MMA-163).
// Uses west,south,east,north ordering (GIS convention, WGS84 decimal degrees).
type BboxLocation struct {
	West    float64 `json:"west"    validate:"required"`
	South   float64 `json:"south"   validate:"required"`
	East    float64 `json:"east"    validate:"required"`
	North   float64 `json:"north"   validate:"required"`
	Density int     `json:"density"` // points per degree (0 = use config default)
}

// registryMu guards concurrent access to the package-level registries
// (LocationShortcutsMap, StationMap, ActiveShapeProvider) via the
// Register*/Lookup*/SetShapeProvider/ShapeProvider accessors.
//
// BUG-001 / ADR-001: direct writes to the deprecated exported vars below do
// NOT take this lock — they remain race-prone for backwards compatibility and
// will be removed in v2.
var registryMu sync.RWMutex

// LocationShortcutsMap holds the location shortcuts defined in the config.toml.
//
// Deprecated: use RegisterLocationShortcut / LookupLocationShortcut instead.
// Direct access to this map is not thread-safe and will be removed in v2.
var LocationShortcutsMap = map[string]RectangleLocation{}

// ActiveShapeProvider is set at startup by the consuming service. It enables
// polygon resolution for country shortcuts. When nil, shortcuts fall back to
// RectangleLocation.
//
// Deprecated: use SetShapeProvider / ShapeProvider instead. Direct access is
// not thread-safe and will be removed in v2.
var ActiveShapeProvider *shapefile.ShapeProvider

// StationMap holds the stations defined in the config.toml.
//
// Deprecated: use RegisterStation / LookupStation instead. Direct access to
// this map is not thread-safe and will be removed in v2.
var StationMap = map[string]PointLocation{}

// RegisterLocationShortcut adds or replaces a shortcut entry in
// LocationShortcutsMap under the package-level lock.
func RegisterLocationShortcut(name string, rect RectangleLocation) {
	registryMu.Lock()
	defer registryMu.Unlock()
	LocationShortcutsMap[name] = rect
}

// LookupLocationShortcut returns the RectangleLocation registered under name
// and whether it exists. Safe for concurrent use.
func LookupLocationShortcut(name string) (RectangleLocation, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	rect, ok := LocationShortcutsMap[name]
	return rect, ok
}

// RegisterStation adds or replaces a station entry in StationMap under the
// package-level lock.
func RegisterStation(id string, pt PointLocation) {
	registryMu.Lock()
	defer registryMu.Unlock()
	StationMap[id] = pt
}

// LookupStation returns the PointLocation registered under id and whether it
// exists. Safe for concurrent use.
func LookupStation(id string) (PointLocation, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	pt, ok := StationMap[id]
	return pt, ok
}

// SetShapeProvider installs the package-level ActiveShapeProvider under the
// package-level lock. Pass nil to clear it.
func SetShapeProvider(sp *shapefile.ShapeProvider) {
	registryMu.Lock()
	defer registryMu.Unlock()
	ActiveShapeProvider = sp
}

// ShapeProvider returns the currently installed ActiveShapeProvider. Safe for
// concurrent use.
func ShapeProvider() *shapefile.ShapeProvider {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return ActiveShapeProvider
}

// ParseLocation parses the Location string and returns a Location struct of the right type (point,polyline,WMO etc.)
// 150: The input is lowercased before shortcut/station lookup so that
// e.g. "Germany", "GERMANY", "germany" all resolve identically.
func ParseLocation(locStr string) (*Location, error) {
	// 150: Lowercase for case-insensitive shortcut and station matching.
	// Maps are populated with lowercase keys at startup (see bootstrap.go).
	locLower := strings.ToLower(locStr)

	// AC-9 (MMA-163): Check for bbox: prefix (case-insensitive).
	if strings.HasPrefix(locLower, "bbox:") {
		bbox, err := parseBbox(locStr[5:]) // skip "bbox:" prefix, use original case for numbers
		if err != nil {
			return nil, err
		}
		return &Location{
			Type:    LocationTypeBbox,
			Payload: *bbox,
		}, nil
	}

	// Check for station ID (WMO format)
	if isPredefinedStation(locLower) {
		return &Location{
			Type:    LocationTypePoint,
			Payload: getLocationFromStationID(locLower),
		}, nil
	}

	// Check for predefined shortcuts — try polygon resolution first, fall back to rectangle.
	if isPredefinedShortcut(locLower) {
		if poly := tryPolygonFromShortcut(locLower); poly != nil {
			return &Location{
				Type:    LocationTypePolygon,
				Payload: *poly,
			}, nil
		}
		return &Location{
			Type:    LocationTypeRectangle,
			Payload: getLocationFromShortcut(locLower),
		}, nil
	}

	// try "lat,lon|radiusKm"
	if rLoc, ok, rErr := tryParseRadius(locStr); ok {
		if rErr != nil {
			return nil, rErr
		}
		return &Location{
			Type:    LocationTypeRadius,
			Payload: *rLoc,
		}, nil
	}

	// Try parsing as coordinates
	coords, err := parseCoordinates(locStr)
	if err != nil {
		return nil, err
	}

	// Determine location type based on coordinate count
	switch len(coords) {
	case 1:
		return &Location{
			Type:    LocationTypePoint,
			Payload: coords[0],
		}, nil
	case 2:
		// Could be rectangle or polyline with 2 points
		if isRectangle(coords) {
			return &Location{
				Type: LocationTypeRectangle,
				Payload: RectangleLocation{
					NorthEast: coords[0],
					SouthWest: coords[1],
				},
			}, nil
		}
		fallthrough
	default:
		return &Location{
			Type:    LocationTypePolyline,
			Payload: PolylineLocation{Coordinates: coords},
		}, nil
	}
}

func getLocationFromShortcut(locStr string) RectangleLocation {
	// BUG-001: read via locked accessor.
	loc, _ := LookupLocationShortcut(locStr)
	return RectangleLocation{NorthEast: loc.NorthEast, SouthWest: loc.SouthWest}
}

// tryPolygonFromShortcut attempts to resolve a shortcut name to a PolygonLocation
// via the ActiveShapeProvider. Returns nil if no shape provider is set or no match
// is found (caller should fall back to RectangleLocation).
func tryPolygonFromShortcut(name string) *PolygonLocation {
	// BUG-001: read via locked accessor.
	sp := ShapeProvider()
	if sp == nil {
		return nil
	}
	cp := sp.Lookup(name)
	if cp == nil {
		return nil
	}
	// Convert shapefile.PointXY rings to PointLocation rings.
	rings := make([][]PointLocation, 0, len(cp.Rings))
	for _, ring := range cp.Rings {
		pts := make([]PointLocation, 0, len(ring))
		for _, p := range ring {
			pts = append(pts, PointLocation{Lat: p.Lat, Lon: p.Lon})
		}
		rings = append(rings, pts)
	}
	return &PolygonLocation{Rings: rings}
}

func getLocationFromStationID(stationID string) PointLocation {
	// BUG-001: read via locked accessor.
	loc, _ := LookupStation(stationID)
	return PointLocation{Lat: loc.Lat, Lon: loc.Lon}
}

// Helper functions
func isPredefinedShortcut(name string) bool {
	// BUG-001: read via locked accessor.
	_, ok := LookupLocationShortcut(name)
	return ok
}

func isPredefinedStation(name string) bool {
	// BUG-001: read via locked accessor.
	_, ok := LookupStation(name)
	return ok
}

func tryParseRadius(s string) (*RadiusLocation, bool, error) {
	parts := strings.Split(s, "|")
	if len(parts) != 2 {
		return nil, false, nil
	}

	// first part must be "lat,lon"
	ll := strings.Split(parts[0], ",")
	if len(ll) != 2 {
		return nil, false, nil
	}

	lat, err1 := strconv.ParseFloat(strings.TrimSpace(ll[0]), 64)
	lon, err2 := strconv.ParseFloat(strings.TrimSpace(ll[1]), 64)

	// second part must be a single number (km)
	if strings.Contains(parts[1], ",") {
		return nil, false, nil
	}
	radius, err3 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)

	if err1 != nil {
		return nil, true, fmt.Errorf("invalid latitude: %w", err1)
	}
	if err2 != nil {
		return nil, true, fmt.Errorf("invalid longitude: %w", err2)
	}
	if err3 != nil {
		return nil, true, fmt.Errorf("invalid radius: %w", err3)
	}
	if radius <= 0 {
		return nil, true, fmt.Errorf("radius must be > 0 km")
	}

	return &RadiusLocation{Lat: lat, Lon: lon, Radius: radius}, true, nil
}

// parseBbox parses "west,south,east,north" into a BboxLocation.
// AC-10 (MMA-163): Validates south < north and west < east.
func parseBbox(s string) (*BboxLocation, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return nil, fmt.Errorf("bbox requires exactly 4 values: west,south,east,north (got %d)", len(parts))
	}

	west, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return nil, fmt.Errorf("invalid bbox west: %w", err)
	}
	south, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return nil, fmt.Errorf("invalid bbox south: %w", err)
	}
	east, err := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
	if err != nil {
		return nil, fmt.Errorf("invalid bbox east: %w", err)
	}
	north, err := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
	if err != nil {
		return nil, fmt.Errorf("invalid bbox north: %w", err)
	}

	// AC-10: south < north
	if south >= north {
		return nil, fmt.Errorf("bbox south (%.4f) must be less than north (%.4f)", south, north)
	}
	// AC-10: west < east (antimeridian wrapping out of scope for v1)
	if west >= east {
		return nil, fmt.Errorf(
			"bbox west (%.4f) must be less than east (%.4f); antimeridian wrapping is not supported in v1",
			west, east,
		)
	}

	return &BboxLocation{West: west, South: south, East: east, North: north}, nil
}

// ExpandBboxToGrid expands a BboxLocation into an evenly-spaced grid of PointLocations.
// MMA-163: Uses configurable density (points per degree). Returns grid points, rows, and cols.
func ExpandBboxToGrid(bbox BboxLocation, pointsPerDegree int) ([]PointLocation, int, int) {
	latSpan := bbox.North - bbox.South
	lonSpan := bbox.East - bbox.West

	rows := int(math.Round(latSpan*float64(pointsPerDegree))) + 1
	cols := int(math.Round(lonSpan*float64(pointsPerDegree))) + 1

	// Ensure at least 1 row and 1 col
	if rows < 1 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}

	latStep := 0.0
	if rows > 1 {
		latStep = latSpan / float64(rows-1)
	}
	lonStep := 0.0
	if cols > 1 {
		lonStep = lonSpan / float64(cols-1)
	}

	pts := make([]PointLocation, 0, rows*cols)
	// Rows = north→south (first row = northernmost), columns = west→east.
	for r := 0; r < rows; r++ {
		lat := bbox.North - float64(r)*latStep
		for c := 0; c < cols; c++ {
			lon := bbox.West + float64(c)*lonStep
			pts = append(pts, PointLocation{Lat: lat, Lon: lon})
		}
	}

	return pts, rows, cols
}

func parseCoordinates(coordStr string) ([]PointLocation, error) {
	parts := strings.Split(coordStr, "|")
	var coords []PointLocation

	for _, part := range parts {
		pointParts := strings.Split(part, ",")
		switch len(pointParts) {
		case 2:
			// Regular coordinate pair (lat,lon)
			lat, err := strconv.ParseFloat(pointParts[0], 64)
			if err != nil {
				return nil, fmt.Errorf("invalid latitude: %w", err)
			}

			lon, err := strconv.ParseFloat(pointParts[1], 64)
			if err != nil {
				return nil, fmt.Errorf("invalid longitude: %w", err)
			}
			coords = append(coords, PointLocation{Lat: lat, Lon: lon})

		case 5:
			// Polyline definition (startLat,startLon,endLat,endLon,pointCount)
			startLat, err := strconv.ParseFloat(pointParts[0], 64)
			if err != nil {
				return nil, fmt.Errorf("invalid start latitude: %w", err)
			}

			startLon, err := strconv.ParseFloat(pointParts[1], 64)
			if err != nil {
				return nil, fmt.Errorf("invalid start longitude: %w", err)
			}

			endLat, err := strconv.ParseFloat(pointParts[2], 64)
			if err != nil {
				return nil, fmt.Errorf("invalid end latitude: %w", err)
			}

			endLon, err := strconv.ParseFloat(pointParts[3], 64)
			if err != nil {
				return nil, fmt.Errorf("invalid end longitude: %w", err)
			}

			pointCount, err := strconv.Atoi(pointParts[4])
			if err != nil || pointCount < 2 {
				return nil, fmt.Errorf("point count must be integer >= 2")
			}

			// Generate intermediate points
			generated, err := generatePoints(
				PointLocation{Lat: startLat, Lon: startLon},
				PointLocation{Lat: endLat, Lon: endLon},
				pointCount,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to generate points: %w", err)
			}
			coords = append(coords, generated...)

		default:
			return nil, fmt.Errorf(
				"invalid coordinate format %s, expected either 2 (lat-lon for a point) or 5 (start lat-lon, end-lan-lon, resolution for a polyline) or a shortcut coseparated values optionally seperated with a pipe",
				coordStr,
			)
		}
	}

	return coords, nil
}

// generatePoints creates evenly spaced points between start and end
func generatePoints(start, end PointLocation, count int) ([]PointLocation, error) {
	if count < 2 {
		return nil, fmt.Errorf("point count must be >= 2")
	}

	var points []PointLocation
	latStep := (end.Lat - start.Lat) / float64(count-1)
	lonStep := (end.Lon - start.Lon) / float64(count-1)

	for i := 0; i < count; i++ {
		points = append(points, PointLocation{
			Lat: start.Lat + (float64(i) * latStep),
			Lon: start.Lon + (float64(i) * lonStep),
		})
	}

	return points, nil
}

func isRectangle(coords []PointLocation) bool {
	// Simple check - first point NE, second point SW
	return coords[0].Lat > coords[1].Lat &&
		coords[0].Lon > coords[1].Lon
}

// maxGridPoints is the upper bound for rectangle grid expansion (5).
const maxGridPoints = 250

// expandRectangleToGrid expands a RectangleLocation into an adaptive grid of
// evenly-spaced PointLocations. The step size is chosen based on the minimum
// dimension of the rectangle (2) and silently coarsened if the point count
// would exceed maxGridPoints (5).
func expandRectangleToGrid(rect RectangleLocation) []PointLocation {
	latMin := math.Min(rect.SouthWest.Lat, rect.NorthEast.Lat)
	latMax := math.Max(rect.SouthWest.Lat, rect.NorthEast.Lat)
	lonMin := math.Min(rect.SouthWest.Lon, rect.NorthEast.Lon)
	lonMax := math.Max(rect.SouthWest.Lon, rect.NorthEast.Lon)

	latSpan := latMax - latMin
	lonSpan := lonMax - lonMin

	// 13: Degenerate rectangle (single point).
	if latSpan == 0 && lonSpan == 0 {
		return []PointLocation{{Lat: latMin, Lon: lonMin}}
	}

	// 2: Choose step from the 4-tier table based on minimum dimension.
	step := chooseGridStep(math.Min(latSpan, lonSpan))

	// 5: Silent coarsening — double the step while count > maxGridPoints.
	for gridPointCount(latSpan, lonSpan, step) > maxGridPoints {
		step *= 2
	}

	// 3 / 4: Generate grid points, always including SW corner and NE corner.
	return generateGrid(latMin, latMax, lonMin, lonMax, step)
}

// chooseGridStep returns the initial step size for the given minimum dimension (2).
func chooseGridStep(minDim float64) float64 {
	switch {
	case minDim >= 1.0:
		return 1.0
	case minDim >= 0.5:
		return 0.5
	case minDim >= 0.25:
		return 0.25
	default:
		return 0.1
	}
}

// gridPointCount estimates the number of grid points for the given spans and step.
func gridPointCount(latSpan, lonSpan, step float64) int {
	latSteps := int(math.Ceil(latSpan/step)) + 1
	lonSteps := int(math.Ceil(lonSpan/step)) + 1
	return latSteps * lonSteps
}

// generateGrid produces the flat list of grid points. The SW corner is always
// included as the first point. The NE corner is boundary-snapped if it does
// not fall exactly on a step boundary (3).
func generateGrid(latMin, latMax, lonMin, lonMax, step float64) []PointLocation {
	pts := make([]PointLocation, 0, 64)

	// Collect unique lat values.
	lats := steppedValues(latMin, latMax, step)
	lons := steppedValues(lonMin, lonMax, step)

	for _, lat := range lats {
		for _, lon := range lons {
			pts = append(pts, PointLocation{Lat: lat, Lon: lon})
		}
	}
	return pts
}

// steppedValues returns evenly-spaced values from min to max (inclusive).
// If max does not fall exactly on a step boundary it is appended (boundary snap, 3).
func steppedValues(lower, upper, step float64) []float64 {
	vals := make([]float64, 0, 16)
	for v := lower; v <= upper+step*1e-9; v += step {
		// Clamp to upper to avoid floating-point overshoot.
		if v > upper {
			v = upper
		}
		vals = append(vals, v)
		if v >= upper {
			break
		}
	}
	// Boundary snap: ensure upper is included.
	// BUG-003: use an epsilon comparison to avoid appending a duplicate
	// `upper` when the last computed value is numerically very close (e.g.
	// 0.9999999999999999 vs 1.0) due to floating-point accumulation in the
	// increment loop. When the last value is within epsilon of `upper`,
	// snap it to exactly `upper` instead of appending a duplicate.
	epsilon := step * 1e-9
	switch {
	case len(vals) == 0:
		vals = append(vals, upper)
	case math.Abs(vals[len(vals)-1]-upper) > epsilon:
		vals = append(vals, upper)
	default:
		vals[len(vals)-1] = upper
	}
	return vals
}

// ExpandLocations resolves a Location into a flat slice of PointLocations.
func ExpandLocations(loc Location) []PointLocation {
	switch loc.Type {
	case LocationTypePoint:
		if v, ok := loc.Payload.(PointLocation); ok {
			return []PointLocation{v}
		}
		if v, ok := loc.Payload.(*PointLocation); ok && v != nil {
			return []PointLocation{*v}
		}
	case LocationTypePointList:
		if v, ok := loc.Payload.(PointListLocation); ok {
			return v.Points
		}
		if v, ok := loc.Payload.(*PointListLocation); ok && v != nil {
			return v.Points
		}
	case LocationTypePolyline:
		if v, ok := loc.Payload.(PolylineLocation); ok {
			return v.Coordinates
		}
		if v, ok := loc.Payload.(*PolylineLocation); ok && v != nil {
			return v.Coordinates
		}
	case LocationTypeRectangle:
		// 1: Expand rectangle into an adaptive grid of evenly-spaced points.
		if v, ok := loc.Payload.(RectangleLocation); ok {
			return expandRectangleToGrid(v)
		}
		if v, ok := loc.Payload.(*RectangleLocation); ok && v != nil {
			return expandRectangleToGrid(*v)
		}
	case LocationTypePolygon:
		// 5: Return the outer ring (index 0) points only.
		if v, ok := loc.Payload.(PolygonLocation); ok && len(v.Rings) > 0 {
			return v.Rings[0]
		}
		if v, ok := loc.Payload.(*PolygonLocation); ok && v != nil && len(v.Rings) > 0 {
			return v.Rings[0]
		}
	case LocationTypeBbox:
		// MMA-163: Bbox expansion is handled separately in the controller
		// (needs config for density). Here we return a single center point as fallback.
		if v, ok := loc.Payload.(BboxLocation); ok {
			centerLat := (v.North + v.South) / 2
			centerLon := (v.East + v.West) / 2
			return []PointLocation{{Lat: centerLat, Lon: centerLon}}
		}
		if v, ok := loc.Payload.(*BboxLocation); ok && v != nil {
			centerLat := (v.North + v.South) / 2
			centerLon := (v.East + v.West) / 2
			return []PointLocation{{Lat: centerLat, Lon: centerLon}}
		}
	case LocationTypeRadius:
		var rad *RadiusLocation
		if v, ok := loc.Payload.(RadiusLocation); ok {
			rad = &v
		} else if v, ok := loc.Payload.(*RadiusLocation); ok && v != nil {
			rad = v
		}
		if rad != nil {
			// If radius < 3 km → only the center point
			if rad.Radius < 3.0 {
				return []PointLocation{{Lat: rad.Lat, Lon: rad.Lon}}
			}

			// Build rings every 3 km; ring k has 3 + k points
			// BUG-004:
			//   1) Pre-size pts using the closed-form sum of ring sizes.
			//      For k = 0, 3, 6, …, 3*K (K = maxKm/3) ring sizes are
			//      3, 6, 9, …, 3*(K+1). Sum = 3 * n * (n+1) / 2 where n = K+1.
			//   2) Hoist sin/cos(angDist) out of the inner bearing loop —
			//      angDist depends only on k.
			maxKm := int(math.Floor(rad.Radius))
			ringCount := maxKm/3 + 1
			capacity := 3 * ringCount * (ringCount + 1) / 2
			pts := make([]PointLocation, 0, capacity)

			for k := 0; k <= maxKm; k += 3 {
				n := 3 + k
				step := 360.0 / float64(n)
				angDist := float64(k) / earthRadiusKm
				sinAng := math.Sin(angDist)
				cosAng := math.Cos(angDist)
				for j := 0; j < n; j++ {
					bearing := float64(j) * step
					lat2, lon2 := destinationPointPrecomputed(rad.Lat, rad.Lon, sinAng, cosAng, bearing)
					pts = append(pts, PointLocation{Lat: lat2, Lon: lon2})
				}
			}
			return pts
		}
	}
	// Unknown/empty -> no points
	return nil
}

const earthRadiusKm = 6371.0

func destinationPoint(latDeg, lonDeg, distKm, bearingDeg float64) (float64, float64) {
	angDist := distKm / earthRadiusKm
	return destinationPointPrecomputed(latDeg, lonDeg, math.Sin(angDist), math.Cos(angDist), bearingDeg)
}

// destinationPointPrecomputed is destinationPoint with sin(angDist) and
// cos(angDist) precomputed by the caller. Used by the radius expansion loop
// in ExpandLocations to hoist these per-ring trig calls out of the inner
// bearing loop (BUG-004). Behaviour identical to destinationPoint when
// sinAngDist=sin(distKm/earthRadiusKm) and cosAngDist=cos(distKm/earthRadiusKm).
func destinationPointPrecomputed(latDeg, lonDeg, sinAngDist, cosAngDist, bearingDeg float64) (float64, float64) {
	lat1 := deg2rad(latDeg)
	lon1 := deg2rad(lonDeg)
	brng := deg2rad(bearingDeg)

	sinLat1 := math.Sin(lat1)
	cosLat1 := math.Cos(lat1)

	sinLat2 := sinLat1*cosAngDist + cosLat1*sinAngDist*math.Cos(brng)
	lat2 := math.Asin(sinLat2)

	y := math.Sin(brng) * sinAngDist * cosLat1
	x := cosAngDist - sinLat1*sinLat2
	lon2 := lon1 + math.Atan2(y, x)

	// normalize lon to [-180, 180)
	lon2 = math.Mod(lon2+math.Pi, 2*math.Pi)
	if lon2 < 0 {
		lon2 += 2 * math.Pi
	}
	lon2 -= math.Pi

	return rad2deg(lat2), rad2deg(lon2)
}

func deg2rad(d float64) float64 { return d * math.Pi / 180.0 }
func rad2deg(r float64) float64 { return r * 180.0 / math.Pi }
