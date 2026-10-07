package async

import (
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
)

type closeTrackingBucket struct {
	objstore.Bucket
	closed bool
}

func (b *closeTrackingBucket) Close() error {
	b.closed = true
	return nil
}

func TestStoreBucketOwnership(t *testing.T) {
	for _, owned := range []bool{false, true} {
		name := "shared"
		if owned {
			name = "dedicated"
		}
		t.Run(name, func(t *testing.T) {
			bucket := &closeTrackingBucket{}
			var options []StoreOption
			if owned {
				options = append(options, WithOwnedBucket())
			}
			store := NewStore(log.NewNopLogger(), bucket, prometheus.NewRegistry(), options...)
			require.NoError(t, store.stopping(nil))
			require.Equal(t, owned, bucket.closed)
		})
	}
}
