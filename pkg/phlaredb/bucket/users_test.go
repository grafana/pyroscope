package bucket

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	thanos "github.com/thanos-io/objstore"

	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/profiledump"
)

func TestCaptureNamespaceDiscovery(t *testing.T) {
	ctx := context.Background()
	storage := objstore.NewPrefixedBucket(objstore.NewBucket(thanos.NewInMemBucket()), "customer/prefix")
	capture, _, err := profiledump.NewObjectKey("tenant", time.Now())
	require.NoError(t, err)
	for _, key := range []string{
		capture, strings.TrimSuffix(capture, ".pprof") + ".json",
		"__pyroscope_cluster/other-diagnostics/object", "bucket-index.json",
		"regular/phlaredb/block", "profile-dumps/phlaredb/block", "profile-dumps-other/phlaredb/block",
	} {
		require.NoError(t, storage.Upload(ctx, key, strings.NewReader("opaque")))
	}
	users, err := ListUsers(ctx, storage)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"regular", "profile-dumps", "profile-dumps-other"}, users)
}
