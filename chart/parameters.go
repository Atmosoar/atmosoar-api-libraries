package chart

import (
	"fmt"
	"strconv"
	"strings"
)

// Break is one threshold edge: values on the good side of At carry Status.
type Break struct {
	At     float64 `json:"at"`
	Status Status  `json:"status"`
}

// Thresholds partitions a parameter's values into status bands.
//
// The bands are ordered best-first. For a normal parameter a value belongs to
// the first break it does not exceed; for an inverted one (visibility, cloud
// base — where less is worse) to the first break it still reaches. Anything past
// the last break is Worst.
type Thresholds struct {
	// Inverted reports that lower values are worse.
	Inverted bool    `json:"inverted,omitempty"`
	Breaks   []Break `json:"breaks"`
	Worst    Status  `json:"worst"`
}

// ParameterDef is everything the chart layer knows about one weather parameter:
// how to label it, what unit it arrives in, how it should be drawn, and where
// its values stop being comfortable.
type ParameterDef struct {
	// Key is the canonical name — the observation package's vocabulary.
	Key   string `json:"key"`
	Label string `json:"label"`
	Unit  string `json:"unit"`
	// Mark is the form this parameter reads best as. Accumulations are bars,
	// traces are lines.
	Mark Mark `json:"mark,omitempty"`
	// Decimals is how many places to show in labels and tooltips.
	Decimals int `json:"decimals"`
	// Zero forces the y-axis to include zero: an accumulation from a floating
	// baseline misstates its own magnitude.
	Zero bool `json:"zero,omitempty"`
	// Thresholds is nil for parameters with no operational bands (pressure,
	// dewpoint): not everything is good or bad.
	Thresholds *Thresholds `json:"thresholds,omitempty"`
}

// The canonical parameter registry.
//
// Keys and units follow the observation package (SI: °C, %, m/s, degrees true,
// hPa, mm, mm/h, W/m²). The threshold numbers for wind, temperature,
// precipitation, visibility, cloud base, humidity and icing are the ones the
// Multi-Model API's marker renderer used before this package existed, so a
// value that was amber on a map stays amber on a chart.
var parameterDefs = map[string]ParameterDef{
	"temperature_2m": {
		Key: "temperature_2m", Label: "Temperature", Unit: "°C", Decimals: 1,
		Thresholds: &Thresholds{
			Breaks: []Break{{At: 30, Status: StatusGood}, {At: 35, Status: StatusWarning}},
			Worst:  StatusCritical,
		},
	},
	"dewpoint": {Key: "dewpoint", Label: "Dewpoint", Unit: "°C", Decimals: 1},
	"relative_humidity": {
		Key: "relative_humidity", Label: "Relative humidity", Unit: "%", Decimals: 0,
		Thresholds: &Thresholds{
			Breaks: []Break{{At: 80, Status: StatusGood}, {At: 95, Status: StatusWarning}},
			Worst:  StatusCritical,
		},
	},
	"wind_speed": {
		Key: "wind_speed", Label: "Wind speed", Unit: "m/s", Decimals: 1, Zero: true,
		Thresholds: &Thresholds{
			Breaks: []Break{{At: 10, Status: StatusGood}, {At: 20, Status: StatusWarning}},
			Worst:  StatusCritical,
		},
	},
	"wind_gust": {
		Key: "wind_gust", Label: "Wind gust", Unit: "m/s", Decimals: 1, Zero: true,
		Thresholds: &Thresholds{
			Breaks: []Break{{At: 10, Status: StatusGood}, {At: 20, Status: StatusWarning}},
			Worst:  StatusCritical,
		},
	},
	"wind_direction": {Key: "wind_direction", Label: "Wind direction", Unit: "°", Decimals: 0},
	"surface_pressure": {
		Key: "surface_pressure", Label: "Surface pressure", Unit: "hPa", Decimals: 1,
	},
	"mslp": {Key: "mslp", Label: "Pressure (MSL)", Unit: "hPa", Decimals: 1},
	"precipitation": {
		Key: "precipitation", Label: "Precipitation", Unit: "mm", Mark: MarkBar, Decimals: 1, Zero: true,
		Thresholds: &Thresholds{
			Breaks: []Break{{At: 1, Status: StatusGood}, {At: 5, Status: StatusWarning}},
			Worst:  StatusCritical,
		},
	},
	"precipitation_rate": {
		Key: "precipitation_rate", Label: "Precipitation rate", Unit: "mm/h", Mark: MarkBar, Decimals: 1, Zero: true,
	},
	"solar_radiation": {
		Key: "solar_radiation", Label: "Solar radiation", Unit: "W/m²", Decimals: 0, Zero: true,
	},
	"uv_index": {Key: "uv_index", Label: "UV index", Unit: "", Decimals: 1, Zero: true},
	"visibility": {
		Key: "visibility", Label: "Visibility", Unit: "km", Decimals: 1, Zero: true,
		Thresholds: &Thresholds{
			Inverted: true,
			Breaks:   []Break{{At: 10, Status: StatusGood}, {At: 5, Status: StatusWarning}},
			Worst:    StatusCritical,
		},
	},
	"cloud_base": {
		Key: "cloud_base", Label: "Cloud base", Unit: "m", Decimals: 0, Zero: true,
		Thresholds: &Thresholds{
			Inverted: true,
			Breaks:   []Break{{At: 1000, Status: StatusGood}, {At: 500, Status: StatusWarning}},
			Worst:    StatusCritical,
		},
	},
	"icing_risk": {
		Key: "icing_risk", Label: "Icing risk", Unit: "%", Decimals: 0, Zero: true,
		Thresholds: &Thresholds{
			Breaks: []Break{{At: 20, Status: StatusGood}, {At: 50, Status: StatusWarning}},
			Worst:  StatusCritical,
		},
	},
	"pressure_difference": {
		Key: "pressure_difference", Label: "Pressure difference", Unit: "hPa", Decimals: 1,
	},
}

// parameterAliases maps the names other services already publish onto the
// canonical keys, so a caller never has to translate before charting.
var parameterAliases = map[string]string{
	"2m_t":                 "temperature_2m",
	"t_2m":                 "temperature_2m",
	"temp":                 "temperature_2m",
	"temperature":          "temperature_2m",
	"2m_dewpoint":          "dewpoint",
	"dew_point":            "dewpoint",
	"2m_rel_humidity":      "relative_humidity",
	"relative_humidity_2m": "relative_humidity",
	"humidity":             "relative_humidity",
	"10m_wind_speed":       "wind_speed",
	"wind_speed_10m":       "wind_speed",
	"windspeed":            "wind_speed",
	"10m_wind_gust":        "wind_gust",
	"wind_gusts_10m":       "wind_gust",
	"10m_wind_direction":   "wind_direction",
	"wind_direction_10m":   "wind_direction",
	"winddirection":        "wind_direction",
	"wind_dir":             "wind_direction",
	"pressure":             "surface_pressure",
	"pressure_msl":         "mslp",
	"sea_level_pressure":   "mslp",
	"precip":               "precipitation",
	"total_precipitation":  "precipitation",
	"rain":                 "precipitation",
	"precip_rate":          "precipitation_rate",
	"rain_rate":            "precipitation_rate",
	"solar":                "solar_radiation",
	"shortwave_radiation":  "solar_radiation",
	"uv":                   "uv_index",
	"vis":                  "visibility",
	"ceiling":              "cloud_base",
	"delta_p":              "pressure_difference",
	"dp":                   "pressure_difference",
}

// Parameter resolves a parameter name — canonical or alias, any case — to its
// definition. The second return is false for names the registry does not know,
// which is not an error: an unknown parameter still charts, it just carries no
// unit, no preferred mark and no threshold bands.
func Parameter(name string) (ParameterDef, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return ParameterDef{}, false
	}
	if canonical, ok := parameterAliases[key]; ok {
		key = canonical
	}
	def, ok := parameterDefs[key]
	return def, ok
}

// Classify places a value in its parameter's status bands. It reports
// StatusNone for a parameter with no thresholds, which is the honest answer:
// pressure is not good or bad.
func Classify(parameter string, value float64) Status {
	def, ok := Parameter(parameter)
	if !ok || def.Thresholds == nil {
		return StatusNone
	}
	return def.Thresholds.Classify(value)
}

// Classify places one value in the bands.
func (t *Thresholds) Classify(value float64) Status {
	for _, b := range t.Breaks {
		if t.Inverted {
			if value >= b.At {
				return b.Status
			}
			continue
		}
		if value <= b.At {
			return b.Status
		}
	}
	return t.Worst
}

// BandsFor builds the shaded threshold bands for a parameter, for the panel
// behind the marks. It returns nil when the parameter has no thresholds.
//
// Only the bands that warn are drawn: shading the comfortable range too would
// paint the whole plot and leave nothing for the data.
func BandsFor(parameter string) []Band {
	def, ok := Parameter(parameter)
	if !ok || def.Thresholds == nil || len(def.Thresholds.Breaks) == 0 {
		return nil
	}
	t := def.Thresholds
	out := make([]Band, 0, len(t.Breaks))
	for i := 1; i < len(t.Breaks); i++ {
		from, to := t.Breaks[i-1].At, t.Breaks[i].At
		if t.Inverted {
			from, to = t.Breaks[i].At, t.Breaks[i-1].At
		}
		out = append(out, Band{
			From:   Float(from),
			To:     Float(to),
			Status: t.Breaks[i].Status,
			Label:  statusLabel(t.Breaks[i].Status, def, from, to),
		})
	}
	// The open-ended worst band: everything past the last break.
	last := t.Breaks[len(t.Breaks)-1].At
	worst := Band{Status: t.Worst, Label: statusLabel(t.Worst, def, last, last)}
	if t.Inverted {
		worst.To = Float(last)
	} else {
		worst.From = Float(last)
	}
	return append(out, worst)
}

// statusLabel names a band in the reader's terms — "caution ≥ 20 m/s" — because
// a status colour never carries its meaning alone.
func statusLabel(s Status, def ParameterDef, from, to float64) string {
	word := map[Status]string{
		StatusGood:     "ok",
		StatusWarning:  "caution",
		StatusSerious:  "poor",
		StatusCritical: "unfavourable",
		StatusNone:     "unclassified",
	}[s]
	unit := def.Unit
	if unit != "" {
		unit = " " + unit
	}
	if def.Thresholds != nil && def.Thresholds.Inverted {
		return fmt.Sprintf("%s ≤ %s%s", word, formatValue(to, def.Decimals), unit)
	}
	return fmt.Sprintf("%s ≥ %s%s", word, formatValue(from, def.Decimals), unit)
}

// FormatValue renders a value with its parameter's precision.
func FormatValue(parameter string, value float64) string {
	def, ok := Parameter(parameter)
	if !ok {
		return formatValue(value, 1)
	}
	return formatValue(value, def.Decimals)
}

func formatValue(v float64, decimals int) string {
	return strconv.FormatFloat(v, 'f', decimals, 64)
}

// ParameterLabel is the display label for a parameter, falling back to the raw
// name so an unregistered parameter still reads as something.
func ParameterLabel(parameter string) string {
	if def, ok := Parameter(parameter); ok {
		return def.Label
	}
	return parameter
}
