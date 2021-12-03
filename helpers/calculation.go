package helpers

import "math"

func AbsPercentageDiff(old, new uint64) float64 {
	diff := float64(new-old) / float64(old) * 100
	return math.Abs(diff)
}
