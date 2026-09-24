package bucket

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	thanos "github.com/thanos-io/objstore"

	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/bucketindex"
	"github.com/grafana/pyroscope/v2/pkg/profiledump"
)

func TestCaptureNamespaceDiscovery(t *testing.T) {
	for _, v1Object := range []string{"", "01DTVP434PA9VFXSW2JKB3392D/profiles.parquet", bucketindex.IndexCompressedFilename, TenantDeletionMarkPath} {
		t.Run(v1Object, func(t *testing.T) {
			ctx := context.Background()
			raw := objstore.NewBucket(thanos.NewInMemBucket())
			b := objstore.NewPrefixedBucket(raw, "customer/prefix")
			capture, _, err := profiledump.NewObjectKey("tenant", time.Now(), profiledump.FormatPprof)
			require.NoError(t, err)
			for _, name := range []string{capture, "regular/phlaredb/01DTVP434PA9VFXSW2JKB3392D/profiles.parquet", "profile-debug-dumps-other/phlaredb/block"} {
				require.NoError(t, b.Upload(ctx, name, strings.NewReader("opaque")))
			}
			if v1Object != "" {
				require.NoError(t, b.Upload(ctx, "profile-debug-dumps/phlaredb/"+v1Object, strings.NewReader("{}")))
			}
			users, err := ListUsers(ctx, b)
			require.NoError(t, err)
			want := []string{"regular", "profile-debug-dumps-other"}
			if v1Object != "" {
				want = append(want, "profile-debug-dumps")
			}
			require.ElementsMatch(t, want, users)
			if v1Object == "" {
				scanner := NewTenantsScanner(b, AllTenants, log.NewNopLogger())
				owned, deleted, err := scanner.ScanTenants(ctx)
				require.NoError(t, err)
				require.ElementsMatch(t, want, owned)
				require.Empty(t, deleted)
			}
			// The real V1 metadata fetcher sees only the tenant's phlaredb subtree.
			// A partial native block remains discoverable, captures never become blocks.
			tenant := objstore.NewTenantBucketClient("profile-debug-dumps", b, nil)
			fetcher, err := block.NewMetaFetcher(log.NewNopLogger(), 1, tenant, "", prometheus.NewRegistry(), nil)
			require.NoError(t, err)
			metas, partials, err := fetcher.Fetch(ctx)
			require.NoError(t, err)
			require.Empty(t, metas)
			if strings.HasPrefix(v1Object, "01D") {
				require.Len(t, partials, 1)
			} else {
				require.Empty(t, partials)
			}
			exists, err := b.Exists(ctx, capture)
			require.NoError(t, err)
			require.True(t, exists)
		})
	}
}

type discoveryBucket struct {
	thanos.Bucket
	iter func(context.Context, string, func(string) error) error
}

func (b discoveryBucket) Iter(ctx context.Context, prefix string, f func(string) error, _ ...thanos.IterOption) error {
	return b.iter(ctx, prefix, f)
}

func TestCaptureNamespaceProbeFailureAndCancellation(t *testing.T) {
	failure := errors.New("list failed")
	for _, canceled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		b := discoveryBucket{Bucket: thanos.NewInMemBucket()}
		b.iter = func(_ context.Context, prefix string, f func(string) error) error {
			if prefix == "" {
				return f("profile-debug-dumps/")
			}
			if canceled {
				cancel()
				return ctx.Err()
			}
			return failure
		}
		_, err := ListUsers(ctx, b)
		if canceled {
			require.ErrorIs(t, err, context.Canceled)
		} else {
			require.ErrorIs(t, err, failure)
		}
		cancel()
	}
}
