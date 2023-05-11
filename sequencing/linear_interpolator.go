package sequencing

import (
	"time"
)

type LinearInterpolator struct {
	// API
	C chan float32
	// private
	ticker *time.Ticker
}

func NewLinearInterpolator(initialValue float32, finalValue float32, durationMs int, stepMs int) *LinearInterpolator {
	duration := time.Duration(durationMs) * time.Millisecond
	step := time.Duration(stepMs) * time.Millisecond
	start := time.Now()
	ticker := time.NewTicker(step)

	interpolator := &LinearInterpolator{make(chan float32), ticker}

	go func() {
		for range ticker.C {
			elapsed := time.Since(start)
			if elapsed > duration {
				interpolator.C <- finalValue
				interpolator.Stop()
			} else {
				ratio := float32(elapsed) / float32(duration)
				currentValue := initialValue + (finalValue-initialValue)*ratio
				interpolator.C <- currentValue
			}
		}
	}()

	return interpolator
}

func (t *LinearInterpolator) Stop() {
	t.ticker.Stop()
	close(t.C)
}
