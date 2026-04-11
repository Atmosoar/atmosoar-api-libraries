package shapefile

import "go.uber.org/fx"

// Module is the FX module for the shapefile package, providing the ShapeProvider.
var Module = fx.Module(
	"shapefile",
	fx.Provide(NewShapeProvider),
)
