package timeseries

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidateStep(t *testing.T) {
	boundary := math.Ldexp(1, 63) / float64(time.Second)
	for _, tc := range []struct {
		name  string
		step  float64
		valid bool
	}{
		{"minimum", 0.001, true},
		{"ordinary", 60, true},
		{"below upper boundary", math.Nextafter(boundary, 0), true},
		{"zero", 0, false},
		{"negative", -1, false},
		{"sub-millisecond", 0.0009, false},
		{"NaN", math.NaN(), false},
		{"infinity", math.Inf(1), false},
		{"upper boundary", boundary, false},
		{"overflow", 1e10, false},
		{"maximum float", math.MaxFloat64, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateStep(tc.step)
			if tc.valid {
				require.NoError(t, err)
				require.GreaterOrEqual(t, time.Duration(tc.step*float64(time.Second)), MinimumStep)
			} else {
				require.Error(t, err)
			}
		})
	}
}
