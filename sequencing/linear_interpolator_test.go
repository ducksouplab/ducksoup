package sequencing

import (
	"log"
	"math"
	"testing"
	"time"
)

func isNear(f1 float32, f2 float32, margin float32) bool {
	return math.Abs(float64(f1-f2)) < float64(margin)
}

// Ticker could be stubbed to fasten test
func TestNewLinearInterpolator(t *testing.T) {
	duration := 300 * time.Millisecond
	step := 60 * time.Millisecond
	// test subject
	interpolator := NewLinearInterpolator(0.0, 1.0, duration, step)
	// expected values
	expected := []float32{0.2, 0.4, 0.6, 0.8, 1.0}
	// launch test
	i := 0
	for value := range interpolator.C {
		log.Println(value)
		if !isNear(value, expected[i], 0.01) {
			t.Errorf("got %f but expected %f", value, expected[i])
		}
		i++
	}

}
