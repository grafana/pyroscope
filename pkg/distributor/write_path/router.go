package writepath

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"

	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	distributormodel "github.com/grafana/pyroscope/pkg/distributor/model"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/util"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
)

// TODO:
//  1. Service
//  2. Overrides
//  3. Integrate into distributor
//  4. Tests
//  5. Add TenantID to the distributormodel.PushRequest

type SendFunc func(context.Context, *distributormodel.PushRequest, string) error

type SegmentWriterClient interface {
	Push(context.Context, *segmentwriterv1.PushRequest) (*segmentwriterv1.PushResponse, error)
}

type Router struct {
	inflight sync.WaitGroup

	config Config
	logger log.Logger

	sendToIngester SendFunc
	segmentWriter  SegmentWriterClient

	durationHistogram *prometheus.HistogramVec
}

func NewRouter(
	limits Config,
	logger log.Logger,
	reg prometheus.Registerer,
	ingester SendFunc,
	segwriter SegmentWriterClient,
) *Router {
	durationHistogram := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "pyroscope",
		Name:      "_write_path_downstream_request_duration_seconds",
		Buckets:   prometheus.ExponentialBucketsRange(0.001, 10, 30),
	}, []string{"route", "primary", "status"})
	reg.MustRegister(durationHistogram)
	return &Router{
		config:            limits,
		logger:            logger,
		sendToIngester:    ingester,
		segmentWriter:     segwriter,
		durationHistogram: durationHistogram,
	}
}

func (m *Router) Send(
	ctx context.Context,
	req *distributormodel.PushRequest,
	tenantID string,
) error {
	switch path := m.config.WritePath(); path {
	case IngesterPath:
		return m.send(m.ingesterRoute())(ctx, req, tenantID)
	case SegmentWriterPath:
		return m.send(m.segwriterRoute(true))(ctx, req, tenantID)
	case CombinedPath:
		return m.sendToBoth(ctx, req, tenantID)
	}
	return ErrInvalidWritePath
}

func (m *Router) ingesterRoute() *route {
	return &route{
		path:    IngesterPath,
		send:    m.sendToIngester,
		primary: true,
	}
}

func (m *Router) segwriterRoute(primary bool) *route {
	return &route{
		path:    SegmentWriterPath,
		primary: primary,
		send: func(
			ctx context.Context,
			req *distributormodel.PushRequest,
			tenantID string,
		) error {
			// Prepare the requests: we're trying to avoid allocating extra
			// memory for serialized profiles by reusing the source request
			// capacities, iff the request won't be sent to ingester.
			requests := convertRequest(tenantID, req, !primary)
			return m.sendRequestsToSegmentWriter(ctx, requests)
		},
	}
}

func (m *Router) sendToBoth(
	ctx context.Context,
	req *distributormodel.PushRequest,
	tenantID string,
) error {
	r := rand.Float64() // [0.0, 1.0)
	ingesterWeight := m.config.IngesterWeight()
	segwriterWeight := m.config.SegmentWriterWeight()
	shouldIngester := ingesterWeight > 0.0 && ingesterWeight >= r
	shouldSegwriter := segwriterWeight > 0.0 && segwriterWeight >= r

	// Client sees errors and latency of the primary write
	// path, secondary write path does not affect the client.
	var ingester, segwriter *route
	if shouldIngester {
		// If the request is sent to ingester (regardless of anything),
		// the response is returned to the client immediately after the old
		// write path returns. Failure of the new write path should be logged
		// and counted in metrics but NOT returned to the client.
		ingester = m.ingesterRoute()
		if !shouldSegwriter {
			return m.send(ingester)(ctx, req, tenantID)
		}
	}
	if shouldSegwriter {
		segwriter = m.segwriterRoute(!shouldIngester)
		if segwriter.primary {
			// If the request is sent to segment-writer exclusively:
			// the response returns to the client when the new write path
			// returns.
			// Failure of the new write is returned to the client.
			// Failure of the old write path is NOT returned to the client.
			return m.send(segwriter)(ctx, req, tenantID)
		}
	}
	if ingester == nil && segwriter == nil {
		return ErrInvalidWritePath
	}

	// If we ended up here, ingester is the primary route.
	c := m.sendAsync(ctx, req, tenantID, ingester)
	m.sendAsync(ctx, req, tenantID, segwriter)
	var err error
	select {
	case err = <-c:
	case <-ctx.Done():
		err = ctx.Err()
	}
	return err
}

type route struct {
	path    WritePath // IngesterPath | SegmentWriterPath
	send    SendFunc
	primary bool
}

func (r *route) metricDims(err error) []string {
	dims := make([]string, 3)
	dims[0] = string(r.path)
	if r.primary {
		dims[1] = "1"
	} else {
		dims[1] = "0"
	}
	code, _ := httputil.ClientHTTPStatusAndError(err)
	dims[2] = strconv.Itoa(code)
	return dims
}

func (m *Router) sendAsync(
	ctx context.Context,
	req *distributormodel.PushRequest,
	tenantID string,
	r *route,
) <-chan error {
	c := make(chan error, 1)
	m.inflight.Add(1)
	go func() {
		defer m.inflight.Done()
		c <- m.send(r)(ctx, req, tenantID)
	}()
	return c
}

func (m *Router) send(r *route) SendFunc {
	return func(
		ctx context.Context,
		req *distributormodel.PushRequest,
		tenantID string,
	) (err error) {
		start := time.Now()
		defer func() {
			if p := recover(); p != nil {
				err = util.PanicError(p)
			}
			m.durationHistogram.
				WithLabelValues(r.metricDims(err)...).
				Observe(time.Since(start).Seconds())
		}()
		return r.send(ctx, req, tenantID)
	}
}

func (m *Router) sendRequestsToSegmentWriter(
	ctx context.Context,
	requests []*segmentwriterv1.PushRequest,
) error {
	// In all known cases, we only have a single profile.
	if len(requests) == 1 {
		_, err := m.segmentWriter.Push(ctx, requests[0])
		return err
	}
	// Fallback: it's very unlikely that we will end up here.
	g, ctx := errgroup.WithContext(ctx)
	for _, r := range requests {
		request := r
		g.Go(func() error {
			_, err := m.segmentWriter.Push(ctx, request)
			return err
		})
	}
	return g.Wait()
}

func convertRequest(
	tenantID string,
	request *distributormodel.PushRequest,
	copy bool,
) []*segmentwriterv1.PushRequest {
	r := make([]*segmentwriterv1.PushRequest, 0, len(request.Series)*2)
	for _, s := range request.Series {
		for _, p := range s.Samples {
			r = append(r, convertProfile(p, s.Labels, tenantID, copy))
		}
	}
	return r
}

func convertProfile(
	sample *distributormodel.ProfileSample,
	labels []*typesv1.LabelPair,
	tenantID string,
	copy bool,
) *segmentwriterv1.PushRequest {
	var b *bytes.Buffer
	if copy {
		b = bytes.NewBuffer(make([]byte, 0, cap(sample.RawProfile)))
	} else {
		b = bytes.NewBuffer(sample.RawProfile[:0])
	}
	if _, err := sample.Profile.WriteTo(b); err != nil {
		panic(fmt.Sprintf("failed to marshal profile: %v", err))
	}
	profileID := uuid.New()
	return &segmentwriterv1.PushRequest{
		TenantId: tenantID,
		// Note that labels are always copied because
		// the API allows multiple profiles to refer to
		// the same label set.
		Labels:    phlaremodel.Labels(labels).Clone(),
		Profile:   b.Bytes(),
		ProfileId: profileID[:],
	}
}
