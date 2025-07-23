package ratelimit

import (
	"io"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriter(t *testing.T) {
	var sleptFor time.Duration
	now := time.Unix(0, 0)

	limiter := NewLimiter(100)
	limiter.now = func() time.Time { return now }
	limiter.sleep = func(d time.Duration) {
		sleptFor += d
		now = now.Add(d)
	}

	w := NewWriter(io.Discard, limiter)

	const N = 1 << 10
	n, err := io.CopyN(w, rand.New(rand.NewSource(42)), N)
	require.NoError(t, err)
	assert.EqualValues(t, n, N)

	t.Log("written in", sleptFor)
	assert.Greater(t, sleptFor, time.Second*9)
}
