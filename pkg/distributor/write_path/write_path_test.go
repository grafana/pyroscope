package writepath

import (
	"context"
	"io"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	distributormodel "github.com/grafana/pyroscope/pkg/distributor/model"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockwritepath"
)

type routerTestSuite struct {
	suite.Suite

	router    *Router
	logger    log.Logger
	registry  *prometheus.Registry
	overrides *mockOverrides
	ingester  *mockwritepath.MockIngesterClient
	segwriter *mockwritepath.MockSegmentWriterClient

	request *distributormodel.PushRequest
}

type mockOverrides struct{ mock.Mock }

func (m *mockOverrides) WritePathOverrides(tenantID string) Config {
	args := m.Called(tenantID)
	return args.Get(0).(Config)
}

func (s *routerTestSuite) SetupTest() {
	s.logger = log.NewLogfmtLogger(io.Discard)
	s.registry = prometheus.NewRegistry()
	s.overrides = new(mockOverrides)
	s.ingester = new(mockwritepath.MockIngesterClient)
	s.segwriter = new(mockwritepath.MockSegmentWriterClient)

	profile := &distributormodel.ProfileSample{Profile: &pprof.Profile{}}
	s.request = &distributormodel.PushRequest{
		TenantID: "tenant-a",
		Series: []*distributormodel.ProfileSeries{
			{
				Samples: []*distributormodel.ProfileSample{profile},
				Labels: []*typesv1.LabelPair{
					{Name: "foo", Value: "bar"},
					{Name: "qux", Value: "zoo"},
				},
			},
		},
	}

	s.router = NewRouter(
		s.logger,
		s.registry,
		s.overrides,
		s.ingester,
		s.segwriter,
	)
}

func (s *routerTestSuite) BeforeTest(_, _ string) {
	svc := s.router.Service()
	s.Require().NoError(svc.StartAsync(context.Background()))
	s.Require().NoError(svc.AwaitRunning(context.Background()))
	s.Require().Equal(services.Running, svc.State())

	s.overrides.AssertExpectations(s.T())
	s.ingester.AssertExpectations(s.T())
	s.segwriter.AssertExpectations(s.T())
}

func (s *routerTestSuite) AfterTest(_, _ string) {
	svc := s.router.Service()
	svc.StopAsync()
	s.Require().NoError(svc.AwaitTerminated(context.Background()))
	s.Require().Equal(services.Terminated, svc.State())

	s.overrides.AssertExpectations(s.T())
	s.ingester.AssertExpectations(s.T())
	s.segwriter.AssertExpectations(s.T())
}

func TestRouterSuite(t *testing.T) { suite.Run(t, new(routerTestSuite)) }

func (s *routerTestSuite) Test_IngesterPath() {
	s.overrides.On("WritePathOverrides", "tenant-a").Return(Config{
		WritePath: IngesterPath,
	})

	s.ingester.On("Push", mock.Anything, s.request).
		Return(new(connect.Response[pushv1.PushResponse]), nil).
		Once()

	s.Assert().NoError(s.router.Send(context.Background(), s.request))
}

func (s *routerTestSuite) Test_SegmentWriterPath() {
	s.overrides.On("WritePathOverrides", "tenant-a").Return(Config{
		WritePath: SegmentWriterPath,
	})

	s.segwriter.On("Push", mock.Anything, mock.Anything).
		Return(new(segmentwriterv1.PushResponse), nil).
		Once()

	s.Assert().NoError(s.router.Send(context.Background(), s.request))
}

func (s *routerTestSuite) Test_CombinedPath() {
	const (
		N = 100
		f = 0.5
		d = 0.3 // Allowed delta: note that f is just a probability.
	)

	s.overrides.On("WritePathOverrides", "tenant-a").Return(Config{
		WritePath:           CombinedPath,
		IngesterWeight:      1,
		SegmentWriterWeight: f,
	})

	var sentIngester atomic.Uint32
	s.ingester.On("Push", mock.Anything, mock.Anything).
		Run(func(mock.Arguments) { sentIngester.Add(1) }).
		Return(new(connect.Response[pushv1.PushResponse]), nil)

	var sentSegwriter atomic.Uint32
	s.segwriter.On("Push", mock.Anything, mock.Anything).
		Run(func(mock.Arguments) { sentSegwriter.Add(1) }).
		Return(new(segmentwriterv1.PushResponse), nil)

	for i := 0; i < N; i++ {
		s.Assert().NoError(s.router.Send(context.Background(), s.request))
	}

	s.router.inflight.Wait()
	expected := N * f
	delta := expected * d
	s.Assert().Equal(N, int(sentIngester.Load()))
	s.Assert().Greater(int(sentSegwriter.Load()), int(expected-delta))
	s.Assert().Less(int(sentSegwriter.Load()), int(expected+delta))
}

func (s *routerTestSuite) Test_UnspecifiedWriterPath() {
	s.overrides.On("WritePathOverrides", "tenant-a").Return(Config{})

	s.ingester.On("Push", mock.Anything, mock.Anything).
		Return(new(connect.Response[pushv1.PushResponse]), nil).
		Once()

	s.Assert().NoError(s.router.Send(context.Background(), s.request))
}

func (s *routerTestSuite) Test_CombinedPath_ZeroWeights() {
	s.overrides.On("WritePathOverrides", "tenant-a").Return(Config{
		WritePath: CombinedPath,
	})

	s.Assert().NoError(s.router.Send(context.Background(), s.request))
}

func (s *routerTestSuite) Test_CombinedPath_IngesterError() {
	s.overrides.On("WritePathOverrides", "tenant-a").Return(Config{
		WritePath: CombinedPath,
		// We ensure that request is sent to both.
		IngesterWeight:      1,
		SegmentWriterWeight: 1,
	})

	s.segwriter.On("Push", mock.Anything, mock.Anything).
		Return(new(segmentwriterv1.PushResponse), nil).
		Once()

	s.ingester.On("Push", mock.Anything, mock.Anything).
		Return(new(connect.Response[pushv1.PushResponse]), context.Canceled).
		Once()

	s.Assert().Error(s.router.Send(context.Background(), s.request), context.Canceled)
}

func (s *routerTestSuite) Test_CombinedPath_SegmentWriterError() {
	s.overrides.On("WritePathOverrides", "tenant-a").Return(Config{
		WritePath: CombinedPath,
		// We ensure that request is sent to both.
		IngesterWeight:      1,
		SegmentWriterWeight: 1,
	})

	s.segwriter.On("Push", mock.Anything, mock.Anything).
		Return(new(segmentwriterv1.PushResponse), context.Canceled).
		Once()

	s.ingester.On("Push", mock.Anything, mock.Anything).
		Return(new(connect.Response[pushv1.PushResponse]), nil).
		Once()

	s.Assert().NoError(s.router.Send(context.Background(), s.request))
}

func (s *routerTestSuite) Test_CombinedPath_Ingester_Exclusive_Error() {
	s.overrides.On("WritePathOverrides", "tenant-a").Return(Config{
		WritePath: CombinedPath,
		// The request is only sent to ingester.
		IngesterWeight:      1,
		SegmentWriterWeight: 0,
	})

	s.ingester.On("Push", mock.Anything, mock.Anything).
		Return(new(connect.Response[pushv1.PushResponse]), context.Canceled).
		Once()

	s.Assert().Error(s.router.Send(context.Background(), s.request), context.Canceled)
}

func (s *routerTestSuite) Test_CombinedPath_SegmentWriter_Exclusive_Error() {
	s.overrides.On("WritePathOverrides", "tenant-a").Return(Config{
		WritePath: CombinedPath,
		// The request is only sent to segment writer.
		IngesterWeight:      0,
		SegmentWriterWeight: 1,
	})

	s.segwriter.On("Push", mock.Anything, mock.Anything).
		Return(new(segmentwriterv1.PushResponse), context.Canceled).
		Once()

	s.Assert().Error(s.router.Send(context.Background(), s.request), context.Canceled)
}

func (s *routerTestSuite) Test_SegmentWriter_MultipleProfiles() {
	s.overrides.On("WritePathOverrides", "tenant-a").Return(Config{
		WritePath:           SegmentWriterPath,
		IngesterWeight:      0,
		SegmentWriterWeight: 1,
	})

	x := s.request.Series[0]
	x.Samples = append(x.Samples, &distributormodel.ProfileSample{Profile: &pprof.Profile{}})

	s.segwriter.On("Push", mock.Anything, mock.Anything).
		Return(new(segmentwriterv1.PushResponse), nil).
		Twice()

	s.Assert().NoError(s.router.Send(context.Background(), s.request))
}
