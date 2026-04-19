package location

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"atmosoar.io/atmosoar-api-libraries/shapefile"
)

// TestRegistryAccessors_Concurrent exercises the BUG-001 locked accessors
// from many goroutines. Run with `go test -race` to catch any unguarded
// access.
func TestRegistryAccessors_Concurrent(t *testing.T) {
	// Reset registries via the locked accessors so we have a clean slate.
	registryMu.Lock()
	LocationShortcutsMap = map[string]RectangleLocation{}
	StationMap = map[string]PointLocation{}
	ActiveShapeProvider = nil
	registryMu.Unlock()

	const goroutines = 64
	const ops = 200

	sp := shapefile.NewShapeProvider(nil)

	var wg sync.WaitGroup
	wg.Add(goroutines * 4)

	// Writers + readers for LocationShortcutsMap.
	for range goroutines {
		go func() {
			defer wg.Done()
			for range ops {
				RegisterLocationShortcut("loc", RectangleLocation{
					NorthEast: PointLocation{Lat: 1, Lon: 2},
					SouthWest: PointLocation{Lat: 0, Lon: 0},
				})
			}
		}()
		go func() {
			defer wg.Done()
			for range ops {
				_, _ = LookupLocationShortcut("loc")
			}
		}()
	}

	// Writers + readers for StationMap.
	for range goroutines / 2 {
		go func() {
			defer wg.Done()
			for range ops {
				RegisterStation("st", PointLocation{Lat: 10, Lon: 20})
			}
		}()
		go func() {
			defer wg.Done()
			for range ops {
				_, _ = LookupStation("st")
			}
		}()
	}

	// Writers + readers for the shape provider singleton.
	for range goroutines / 2 {
		go func() {
			defer wg.Done()
			for range ops {
				SetShapeProvider(sp)
			}
		}()
		go func() {
			defer wg.Done()
			for range ops {
				_ = ShapeProvider()
			}
		}()
	}

	wg.Wait()

	got, ok := LookupLocationShortcut("loc")
	assert.True(t, ok)
	assert.Equal(t, 1.0, got.NorthEast.Lat)

	// Cleanup.
	registryMu.Lock()
	LocationShortcutsMap = map[string]RectangleLocation{}
	StationMap = map[string]PointLocation{}
	ActiveShapeProvider = nil
	registryMu.Unlock()
}
