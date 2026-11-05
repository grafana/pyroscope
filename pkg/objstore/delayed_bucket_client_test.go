package objstore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/memory"
)

func TestDelayedBucketClient_EqualDelaysDoNotPanic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		minDelay time.Duration
		maxDelay time.Duration
	}{
		{
			name:     "zero delay",
			minDelay: 0,
			maxDelay: 0,
		},
		{
			name:     "fixed non-zero delay",
			minDelay: time.Millisecond,
			maxDelay: time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bucket := NewDelayedBucketClient(memory.NewInMemBucket(), tt.minDelay, tt.maxDelay)

			require.NotPanics(t, func() {
				_, err := bucket.Exists(context.Background(), "missing")
				require.NoError(t, err)
			})
		})
	}
}

func TestNewDelayedBucketClient_InvalidConfigurationPanics(t *testing.T) {
	t.Parallel()

	require.Panics(t, func() {
		NewDelayedBucketClient(memory.NewInMemBucket(), time.Second, 0)
	})
}
