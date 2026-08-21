package distributor

import (
	"context"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/ring/client"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	distributormodel "github.com/grafana/pyroscope/v2/pkg/distributor/model"
	"github.com/grafana/pyroscope/v2/pkg/distributor/writepath"
	pprof2 "github.com/grafana/pyroscope/v2/pkg/pprof"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/testhelper"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

// blockingSegmentWriter holds every Push until release is closed.
type blockingSegmentWriter struct {
	release chan struct{}
	started chan struct{}
}

func newBlockingSegmentWriter(pushes int) *blockingSegmentWriter {
	return &blockingSegmentWriter{
		release: make(chan struct{}),
		started: make(chan struct{}, pushes),
	}
}

func (s *blockingSegmentWriter) CheckReady(context.Context) error { return nil }

func (s *blockingSegmentWriter) Push(context.Context, *segmentwriterv1.PushRequest) (*segmentwriterv1.PushResponse, error) {
	s.started <- struct{}{}
	<-s.release
	return &segmentwriterv1.PushResponse{}, nil
}

func newInflightDistributor(t *testing.T, reg prometheus.Registerer, maxInflightBytes int64, async bool, sw SegmentWriterClient) *Distributor {
	t.Helper()
	overrides := validation.MockOverrides(func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {
		l := validation.MockDefaultLimits()
		l.WritePathOverrides.WritePath = writepath.SegmentWriterPath
		l.WritePathOverrides.AsyncIngest = async
		tenantLimits["user-1"] = l
	})
	d, err := New(
		Config{DistributorRing: ringConfig, MaxInflightBytes: maxInflightBytes},
		testhelper.NewMockRing([]ring.InstanceDesc{{Addr: "foo"}}, 3),
		&poolFactory{f: func(addr string) (client.PoolClient, error) { return newFakeIngester(t, false), nil }},
		overrides, reg, log.NewNopLogger(), sw,
	)
	require.NoError(t, err)
	return d
}

func newInflightRequest(series int) *distributormodel.PushRequest {
	return &distributormodel.PushRequest{
		RawProfileType: distributormodel.RawProfileTypePPROF,
		Series:         probeProfileSeries(series),
	}
}

func TestPushBatch_MaxInflightBytes(t *testing.T) {
	const series = 4
	size, profiles := inflightBytes(newInflightRequest(series))
	require.Positive(t, size)
	require.Equal(t, int64(series), profiles)

	t.Run("request that fits is accepted", func(t *testing.T) {
		d := newInflightDistributor(t, nil, size, false, &probeSegmentWriter{})
		ctx := tenant.InjectTenantID(context.Background(), "user-1")
		require.NoError(t, d.PushBatch(ctx, newInflightRequest(series)))
		assert.Equal(t, int64(0), d.inflight.Bytes())
	})

	t.Run("request that exceeds the limit is rejected with 503", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		d := newInflightDistributor(t, reg, size-1, false, &probeSegmentWriter{})
		ctx := tenant.InjectTenantID(context.Background(), "user-1")

		// The discard metrics are process-wide, so compare deltas.
		discardedBytes := validation.DiscardedBytes.WithLabelValues(reasonMaxInflightBytes, "user-1")
		discardedProfiles := validation.DiscardedProfiles.WithLabelValues(reasonMaxInflightBytes, "user-1")
		bytesBefore := testutil.ToFloat64(discardedBytes)
		profilesBefore := testutil.ToFloat64(discardedProfiles)

		err := d.PushBatch(ctx, newInflightRequest(series))
		require.Error(t, err)
		assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
		assert.Equal(t, int64(0), d.inflight.Bytes(), "the rejected reservation is released")

		assert.Equal(t, float64(1), testutil.ToFloat64(
			d.metrics.rejectedRequests.WithLabelValues(reasonMaxInflightBytes)))
		assert.Equal(t, float64(size-1), testutil.ToFloat64(d.metrics.inflightBytesLimit))

		// The rejected data is attributed to the tenant that was turned away.
		assert.Equal(t, float64(size), testutil.ToFloat64(discardedBytes)-bytesBefore)
		assert.Equal(t, float64(profiles), testutil.ToFloat64(discardedProfiles)-profilesBefore)
	})

	t.Run("the rejection series exists before the first rejection", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		newInflightDistributor(t, reg, size, false, &probeSegmentWriter{})
		assert.Equal(t, 1, testutil.CollectAndCount(reg, "pyroscope_distributor_rejected_requests_total"))
	})

	t.Run("the high watermark records the peak of a rejected request", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		d := newInflightDistributor(t, reg, size-1, false, &probeSegmentWriter{})
		ctx := tenant.InjectTenantID(context.Background(), "user-1")
		require.Error(t, d.PushBatch(ctx, newInflightRequest(series)))

		bs, err := testutil.CollectAndFormat(reg, expfmt.TypeTextPlain,
			"pyroscope_distributor_inflight_bytes_high_watermark")
		require.NoError(t, err)
		assert.Contains(t, string(bs),
			fmt.Sprintf(`pyroscope_distributor_inflight_bytes_high_watermark{quantile="1"} %d`, size))
	})

	t.Run("the limit is disabled by default", func(t *testing.T) {
		d := newInflightDistributor(t, nil, 0, false, &probeSegmentWriter{})
		ctx := tenant.InjectTenantID(context.Background(), "user-1")
		require.NoError(t, d.PushBatch(ctx, newInflightRequest(series)))
	})
}

func TestPushBatch_MaxInflightBytes_AsyncWritePath(t *testing.T) {
	sw := newBlockingSegmentWriter(1)
	d := newInflightDistributor(t, nil, 0, true, sw)
	ctx := tenant.InjectTenantID(context.Background(), "user-1")

	// The client is served as soon as the request is handed over to the
	// segment writer, but the profile is still held in memory.
	require.NoError(t, d.PushBatch(ctx, newInflightRequest(1)))
	select {
	case <-sw.started:
	case <-time.After(10 * time.Second):
		t.Fatal("segment writer was not called")
	}
	assert.Positive(t, d.inflight.Bytes(), "bytes stay reserved while the async write is in flight")

	close(sw.release)
	assert.Eventually(t, func() bool { return d.inflight.Bytes() == 0 },
		10*time.Second, 10*time.Millisecond, "bytes are released once the async write completes")
}

func TestInflightBytes(t *testing.T) {
	t.Parallel()

	t.Run("nil profiles are skipped", func(t *testing.T) {
		req := &distributormodel.PushRequest{Series: []*distributormodel.ProfileSeries{{}}}
		size, profiles := inflightBytes(req)
		assert.Equal(t, int64(0), size)
		assert.Equal(t, int64(0), profiles)
	})

	t.Run("profiles decoded from bytes report their raw size", func(t *testing.T) {
		profile := collectTestProfileBytes(t)
		decoded, err := pprof2.RawFromBytes(profile)
		require.NoError(t, err)
		req := &distributormodel.PushRequest{
			Series: []*distributormodel.ProfileSeries{{Profile: decoded}},
		}
		size, profiles := inflightBytes(req)
		assert.Equal(t, int64(decoded.RawSize()), size)
		assert.Equal(t, int64(1), profiles)
	})

	t.Run("profiles built in-process fall back to the encoded size", func(t *testing.T) {
		p := newProbeProfile()
		require.Zero(t, p.RawSize())
		req := &distributormodel.PushRequest{
			Series: []*distributormodel.ProfileSeries{{Profile: p}, {Profile: p}},
		}
		size, profiles := inflightBytes(req)
		assert.Equal(t, int64(2*p.SizeVT()), size)
		assert.Equal(t, int64(2), profiles)
	})
}
