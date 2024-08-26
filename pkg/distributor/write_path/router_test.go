package writepath

import (
	"context"
	"io"
	"testing"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	distributormodel "github.com/grafana/pyroscope/pkg/distributor/model"
	"github.com/grafana/pyroscope/pkg/pprof"
)

type routerTestSuite struct {
	suite.Suite

	router    *Router
	logger    log.Logger
	registry  *prometheus.Registry
	overrides *mockOverrides
	ingester  *mockIngesterClient
	segwriter *mockSegmentWriterClient

	request *distributormodel.PushRequest
}

type mockOverrides struct{ mock.Mock }

func (m *mockOverrides) WritePathOverrides(tenantID string) Config {
	args := m.Called(tenantID)
	return args.Get(0).(Config)
}

type mockSegmentWriterClient struct{ mock.Mock }

func (m *mockSegmentWriterClient) Push(
	ctx context.Context,
	request *segmentwriterv1.PushRequest,
) (*segmentwriterv1.PushResponse, error) {
	args := m.Called(ctx, request)
	return args.Get(0).(*segmentwriterv1.PushResponse), args.Error(1)
}

type mockIngesterClient struct{ mock.Mock }

func (m *mockIngesterClient) Push(
	ctx context.Context,
	request *distributormodel.PushRequest,
) (*connect.Response[pushv1.PushResponse], error) {
	args := m.Called(ctx, request)
	return args.Get(0).(*connect.Response[pushv1.PushResponse]), args.Error(1)
}

func (s *routerTestSuite) SetupTest() {
	s.logger = log.NewLogfmtLogger(io.Discard)
	s.registry = prometheus.NewRegistry()
	s.overrides = new(mockOverrides)
	s.ingester = new(mockIngesterClient)
	s.segwriter = new(mockSegmentWriterClient)

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

func (s *routerTestSuite) AfterTest(_, _ string) {
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
		d = 0.3 // Allowed delta.
	)

	s.overrides.On("WritePathOverrides", "tenant-a").Return(Config{
		WritePath:           CombinedPath,
		IngesterWeight:      1,
		SegmentWriterWeight: f,
	})

	var sentIngester int
	s.ingester.On("Push", mock.Anything, mock.Anything).
		Run(func(mock.Arguments) { sentIngester++ }).
		Return(new(connect.Response[pushv1.PushResponse]), nil)

	var sentSegwriter int
	s.segwriter.On("Push", mock.Anything, mock.Anything).
		Run(func(mock.Arguments) { sentSegwriter++ }).
		Return(new(segmentwriterv1.PushResponse), nil)

	for i := 0; i < N; i++ {
		s.Assert().NoError(s.router.Send(context.Background(), s.request))
	}

	expected := N * f
	delta := expected * d
	s.Assert().Equal(N, sentIngester)
	s.Assert().Greater(sentSegwriter, int(expected-delta))
	s.Assert().Less(sentSegwriter, int(expected+delta))
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
