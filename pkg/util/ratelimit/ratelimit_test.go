package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLimiter(t *testing.T) {
	var sleptFor []time.Duration
	now := time.Unix(0, 0)

	l := NewLimiter(100)
	l.now = func() time.Time { return now }
	l.sleep = func(d time.Duration) {
		sleptFor = append(sleptFor, d)
		now = now.Add(d)
	}

	l.Wait(100)
	assert.Len(t, sleptFor, 0)

	l.Wait(100)
	require.Len(t, sleptFor, 1)
	assert.Equal(t, time.Second, sleptFor[0])

	l.Wait(100)
	require.Len(t, sleptFor, 2)
	assert.Equal(t, time.Second, sleptFor[1])
}
