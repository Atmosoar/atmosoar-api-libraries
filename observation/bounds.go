package observation

import "math"

// Bounds is an inclusive plausibility range for one field.
type Bounds struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// Violation records a value Sanitize removed.
type Violation struct {
	Field  string  `json:"field"`
	Value  float64 `json:"value"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Reason string  `json:"reason"`
}

// DefaultBounds returns plausibility ranges keyed by JSON field name. They are
// deliberately wide — they reject sensor faults and unit mix-ups (a °F value
// stored as °C, Pa stored as hPa), not unusual weather.
func DefaultBounds() map[string]Bounds {
	return map[string]Bounds{
		"lat":                        {Min: -90, Max: 90},
		"lon":                        {Min: -180, Max: 180},
		"elevation":                  {Min: -500, Max: 9000},
		"temperature_2m":             {Min: -90, Max: 65},
		"dewpoint":                   {Min: -100, Max: 45},
		"relative_humidity":          {Min: 0, Max: 100},
		"wind_speed":                 {Min: 0, Max: 120},
		"wind_gust":                  {Min: 0, Max: 150},
		"wind_direction":             {Min: 0, Max: 360},
		"surface_pressure":           {Min: 300, Max: 1100},
		"mslp":                       {Min: 850, Max: 1090},
		"precipitation":              {Min: 0, Max: 2000},
		"precipitation_period_hours": {Min: 0, Max: 8784},
		"precipitation_rate":         {Min: 0, Max: 500},
		"solar_radiation":            {Min: 0, Max: 2000},
		"uv_index":                   {Min: 0, Max: 25},
	}
}

// Sanitize clears every field that is not finite or lies outside its bounds
// and returns what it cleared. Fields without an entry in bounds are only
// checked for being finite.
func (o *Observation) Sanitize(bounds map[string]Bounds) []Violation {
	var out []Violation
	for _, f := range o.fields() {
		if *f.ptr == nil {
			continue
		}
		v := **f.ptr
		b, ok := bounds[f.name]
		switch {
		case math.IsNaN(v) || math.IsInf(v, 0):
			out = append(out, Violation{Field: f.name, Min: b.Min, Max: b.Max, Reason: "not finite"})
		case ok && (v < b.Min || v > b.Max):
			out = append(out, Violation{Field: f.name, Value: v, Min: b.Min, Max: b.Max, Reason: "out of range"})
		default:
			continue
		}
		*f.ptr = nil
	}
	return out
}
