package sequencing

import (
	"math"
	"testing"
)

func areNear(f1 float32, f2 float32, margin float32) bool {
	return math.Abs(float64(f1-f2)) < float64(margin)
}

// Ticker could be stubbed to fasten test
func TestNewLinearInterpolator(t *testing.T) {

	assertNearValue := func(t testing.TB, value, expected float32) {
		t.Helper()
		if !areNear(value, expected, 0.01) {
			t.Errorf("got %f but expected %f", value, expected)
		}
	}

	// test subject
	interpolator := NewLinearInterpolator(0.0, 1.0, 300, 60)
	// expected values
	expected := []float32{0.2, 0.4, 0.6, 0.8, 1.0}
	// launch test
	i := 0
	for value := range interpolator.C {
		assertNearValue(t, value, expected[i])
		i++
	}

}
