package observability

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

// The HTTP latency histogram keeps the client defaults and reaches the
// gateway's 200 s timeout, so slow endpoints such as /impact stay measurable.
func TestHTTPDurationBuckets(t *testing.T) {
	assert.Equal(t, prometheus.DefBuckets, httpDurationBuckets[:len(prometheus.DefBuckets)])
	assert.Equal(t, []float64{30, 60, 120, 200}, httpDurationBuckets[len(prometheus.DefBuckets):])
	assert.Len(t, prometheus.DefBuckets, 11, "append must not have aliased DefBuckets")
}
