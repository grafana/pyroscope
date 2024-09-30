package ewma

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_Rate(t *testing.T) {
	s := int64(0)
	r := New(time.Second * 10)

	// Expected rate 100.
	for i := 0; i < 300; i++ { // 30s.
		r.Add(10, s)
		s += 1e9 / 10 // 100ms
	}
	assert.InEpsilon(t, 100, r.Value(), 0.1)

	// Rate decreases.
	for i := 0; i < 1000; i++ { // 30s.
		r.Add(5, s)
		s += 1e9 / 10
	}
	assert.InEpsilon(t, 50, r.Value(), 0.1)

	// Exactly 1s rate.
	for i := 0; i < 30; i++ { // 30s
		r.Add(50, s)
		s += 1e9
	}
	assert.InEpsilon(t, 50, r.Value(), 0.1)

	// Sub-second rate.
	for i := 0; i < 50; i++ { // 100s.
		r.Add(1, s)
		s += 1e9 * 2 // Once per two seconds.
	}
	assert.InEpsilon(t, 0.5, r.Value(), 0.05)
}
