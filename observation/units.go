package observation

import "math"

// Conversion factors to SI, exact where the unit is defined exactly.
const (
	mpsPerMph   = 0.44704
	mpsPerKnot  = 1852.0 / 3600.0
	hPaPerInHg  = 33.8638866667
	mmPerInch   = 25.4
	kmhPerMps   = 3.6
	paPerHPa    = 100.0
	magnusA     = 17.62
	magnusBDegC = 243.12
)

// FahrenheitToCelsius converts °F to °C.
func FahrenheitToCelsius(f float64) float64 {
	return (f - 32) * 5 / 9
}

// MphToMps converts miles per hour to metres per second.
func MphToMps(v float64) float64 {
	return v * mpsPerMph
}

// KmhToMps converts kilometres per hour to metres per second.
func KmhToMps(v float64) float64 {
	return v / kmhPerMps
}

// KnotsToMps converts knots to metres per second.
func KnotsToMps(v float64) float64 {
	return v * mpsPerKnot
}

// InHgToHPa converts inches of mercury to hectopascals.
func InHgToHPa(v float64) float64 {
	return v * hPaPerInHg
}

// PaToHPa converts pascals to hectopascals.
func PaToHPa(v float64) float64 {
	return v / paPerHPa
}

// InchToMm converts inches (of rain) to millimetres.
func InchToMm(v float64) float64 {
	return v * mmPerInch
}

// DewpointMagnus derives the dewpoint in °C from air temperature (°C) and
// relative humidity (%) with the Magnus approximation (Sonntag coefficients).
// It returns NaN for humidity outside (0, 100].
func DewpointMagnus(tempC, relativeHumidity float64) float64 {
	if relativeHumidity <= 0 || relativeHumidity > 100 {
		return math.NaN()
	}
	gamma := math.Log(relativeHumidity/100) + magnusA*tempC/(magnusBDegC+tempC)
	return magnusBDegC * gamma / (magnusA - gamma)
}

// Round rounds v to the given number of decimal places.
func Round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}
