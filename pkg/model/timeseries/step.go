package timeseries

import (
	"fmt"
	"math"
	"time"
)

// MinimumStep is the smallest supported resolution for millisecond-based series.
const MinimumStep = time.Millisecond

// ValidateStep checks a step in seconds before conversion to time.Duration.
func ValidateStep(step float64) error {
	if math.IsNaN(step) || math.IsInf(step, 0) || step < MinimumStep.Seconds() {
		return fmt.Errorf("step must be >= 1ms and finite")
	}
	// float64(math.MaxInt64) rounds up to 2^63, which is already outside
	// the duration range. Reject that boundary as well as larger values.
	if step*float64(time.Second) >= math.Ldexp(1, 63) {
		return fmt.Errorf("step exceeds the maximum supported duration")
	}
	return nil
}
