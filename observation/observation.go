// Package observation defines the common, unit-normalised weather observation
// model shared by Atmosoar ingest services. Field names and units match
// observation-api's Observation, so a reading pushed directly by a station
// uses the same vocabulary as one decoded from a synoptic network: SI units
// throughout (°C, %, m/s, degrees true, hPa, mm, mm/h, W/m²).
//
// Every measurement is a pointer. nil means "not reported", which is not the
// same as a reported zero (calm wind, no rain).
package observation

import "time"

// Observation is one station reading at one instant.
type Observation struct {
	// StationID is the ingesting service's handle for the station.
	StationID string `json:"station_id"`
	// Source names the payload family the reading was decoded from, e.g. "ecowitt".
	Source string `json:"source"`
	// Timestamp is the observation time in UTC.
	Timestamp time.Time `json:"timestamp"`

	// Lat is the station latitude in decimal degrees, WGS84.
	Lat *float64 `json:"lat"`
	// Lon is the station longitude in decimal degrees, WGS84.
	Lon *float64 `json:"lon"`
	// Elevation is the station height above mean sea level, in metres.
	Elevation *float64 `json:"elevation"`

	// Temperature2m is the air temperature, in degrees Celsius.
	Temperature2m *float64 `json:"temperature_2m"`
	// Dewpoint is the dewpoint temperature, in degrees Celsius.
	Dewpoint *float64 `json:"dewpoint"`
	// RelativeHumidity is the relative humidity, in percent (0..100).
	RelativeHumidity *float64 `json:"relative_humidity"`
	// WindSpeed is the mean wind speed, in metres per second.
	WindSpeed *float64 `json:"wind_speed"`
	// WindGust is the maximum gust speed, in metres per second.
	WindGust *float64 `json:"wind_gust"`
	// WindDirection is the direction the wind blows from, in degrees true (0..360).
	WindDirection *float64 `json:"wind_direction"`
	// SurfacePressure is the station-level (absolute) pressure, in hectopascals.
	SurfacePressure *float64 `json:"surface_pressure"`
	// Mslp is the pressure reduced to mean sea level, in hectopascals.
	Mslp *float64 `json:"mslp"`
	// Precipitation is an accumulated total over PrecipitationPeriodHours, in
	// millimetres. It is not a rate; see PrecipitationRate.
	Precipitation *float64 `json:"precipitation"`
	// PrecipitationPeriodHours is the accumulation window of Precipitation, in hours.
	PrecipitationPeriodHours *float64 `json:"precipitation_period_hours"`
	// PrecipitationRate is the instantaneous precipitation rate, in mm/h.
	PrecipitationRate *float64 `json:"precipitation_rate"`
	// SolarRadiation is the instantaneous global irradiance, in W/m².
	SolarRadiation *float64 `json:"solar_radiation"`
	// UVIndex is the UV index (dimensionless).
	UVIndex *float64 `json:"uv_index"`

	// Extra carries source-specific values that have no column in the common
	// model (battery state, indoor sensors, daily rain counters, lightning).
	// Keys are the source's own names so nothing is silently reinterpreted.
	Extra map[string]any `json:"extra,omitempty"`
}

// Float returns a pointer to v, for filling Observation fields inline.
func Float(v float64) *float64 {
	return &v
}

// HasMeasurement reports whether at least one measurement is set. Position
// fields do not count: a payload carrying only coordinates is not a reading.
func (o *Observation) HasMeasurement() bool {
	for _, f := range o.fields() {
		if f.measurement && *f.ptr != nil {
			return true
		}
	}
	return false
}

// field binds a JSON field name to its pointer so bounds checks and presence
// checks walk one list instead of repeating every field by hand.
type field struct {
	name        string
	ptr         **float64
	measurement bool
}

func (o *Observation) fields() []field {
	return []field{
		{name: "lat", ptr: &o.Lat},
		{name: "lon", ptr: &o.Lon},
		{name: "elevation", ptr: &o.Elevation},
		{name: "temperature_2m", ptr: &o.Temperature2m, measurement: true},
		{name: "dewpoint", ptr: &o.Dewpoint, measurement: true},
		{name: "relative_humidity", ptr: &o.RelativeHumidity, measurement: true},
		{name: "wind_speed", ptr: &o.WindSpeed, measurement: true},
		{name: "wind_gust", ptr: &o.WindGust, measurement: true},
		{name: "wind_direction", ptr: &o.WindDirection, measurement: true},
		{name: "surface_pressure", ptr: &o.SurfacePressure, measurement: true},
		{name: "mslp", ptr: &o.Mslp, measurement: true},
		{name: "precipitation", ptr: &o.Precipitation, measurement: true},
		{name: "precipitation_period_hours", ptr: &o.PrecipitationPeriodHours},
		{name: "precipitation_rate", ptr: &o.PrecipitationRate, measurement: true},
		{name: "solar_radiation", ptr: &o.SolarRadiation, measurement: true},
		{name: "uv_index", ptr: &o.UVIndex, measurement: true},
	}
}
