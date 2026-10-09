package bucket

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	thanos "github.com/thanos-io/objstore"

	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/profiledump"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockobjstore"
)

func TestCaptureNamespaceDiscovery(t *testing.T) {
	ctx := context.Background()
	storage := objstore.NewPrefixedBucket(objstore.NewBucket(thanos.NewInMemBucket()), "customer/prefix")
	capture, _, err := profiledump.NewNativeObjectKey("tenant", time.Now())
	require.NoError(t, err)
	for _, key := range []string{
		capture, strings.TrimSuffix(capture, ".pprof") + ".json",
		"__pyroscope_cluster/other-diagnostics/object", "bucket-index.json",
		"regular/phlaredb/block", "profile-debug-dumps/phlaredb/block", "profile-debug-dumps-other/phlaredb/block",
	} {
		require.NoError(t, storage.Upload(ctx, key, strings.NewReader("opaque")))
	}
	b := mockobjstore.NewMockBucketWithHelper(t)
	b.EXPECT().Iter(mock.Anything, "", mock.Anything).RunAndReturn(storage.Iter)
	users, err := ListUsers(context.Background(), b)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"regular", "profile-debug-dumps", "profile-debug-dumps-other"}, users)
	b.AssertNumberOfCalls(t, "Iter", 1)
}

func TestListUsersRootListingFailure(t *testing.T) {
	for _, failure := range []error{errors.New("root listing failed"), context.Canceled} {
		t.Run(failure.Error(), func(t *testing.T) {
			b := mockobjstore.NewMockBucketWithHelper(t)
			b.MockIter("", []string{"regular/", "profile-debug-dumps/", PyroscopeInternalsPrefix + "/"}, failure)
			users, err := ListUsers(context.Background(), b)
			require.ErrorIs(t, err, failure)
			require.Equal(t, []string{"regular", "profile-debug-dumps"}, users)
			b.AssertNumberOfCalls(t, "Iter", 1)
		})
	}
}
