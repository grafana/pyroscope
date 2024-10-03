package read_path

import (
	"context"
	"io"
	"math"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockquerierv1connect"
)

type routerTestSuite struct {
	suite.Suite

	router   *Router
	logger   log.Logger
	registry *prometheus.Registry

	overrides *mockOverrides
	frontend  *mockquerierv1connect.MockQuerierServiceClient
	backend   *mockquerierv1connect.MockQuerierServiceClient

	ctx context.Context
}

type mockOverrides struct{ mock.Mock }

func (m *mockOverrides) ReadPathOverrides(tenantID string) Config {
	args := m.Called(tenantID)
	return args.Get(0).(Config)
}

func (s *routerTestSuite) SetupTest() {
	s.logger = log.NewLogfmtLogger(io.Discard)
	s.registry = prometheus.NewRegistry()
	s.overrides = new(mockOverrides)
	s.frontend = new(mockquerierv1connect.MockQuerierServiceClient)
	s.backend = new(mockquerierv1connect.MockQuerierServiceClient)
	s.router = NewRouter(
		s.logger,
		s.overrides,
		s.frontend,
		s.backend,
	)
	s.ctx = tenant.InjectTenantID(context.Background(), "tenant-a")
}

func (s *routerTestSuite) BeforeTest(_, _ string) {}

func (s *routerTestSuite) AfterTest(_, _ string) {
	s.overrides.AssertExpectations(s.T())
	s.frontend.AssertExpectations(s.T())
	s.backend.AssertExpectations(s.T())
}

func TestRouterSuite(t *testing.T) { suite.Run(t, new(routerTestSuite)) }

func (s *routerTestSuite) Test_FrontendOnly() {
	s.overrides.On("ReadPathOverrides", "tenant-a").Return(Config{EnableQueryBackend: false})

	expected := connect.NewResponse(&typesv1.LabelNamesResponse{Names: []string{"foo", "bar"}})
	s.frontend.On("LabelNames", mock.Anything, mock.Anything).Return(expected, nil).Once()

	resp, err := s.router.LabelNames(s.ctx, connect.NewRequest(&typesv1.LabelNamesRequest{}))
	s.Require().NoError(err)
	s.Assert().Equal(expected, resp)
}

func (s *routerTestSuite) Test_BackendOnly() {
	s.overrides.On("ReadPathOverrides", "tenant-a").Return(Config{EnableQueryBackend: true})

	expected := connect.NewResponse(&typesv1.LabelNamesResponse{Names: []string{"foo", "bar"}})
	s.backend.On("LabelNames", mock.Anything, mock.Anything).Return(expected, nil).Once()

	resp, err := s.router.LabelNames(s.ctx, connect.NewRequest(&typesv1.LabelNamesRequest{}))
	s.Require().NoError(err)
	s.Assert().Equal(expected, resp)
}

func (s *routerTestSuite) Test_Combined() {
	s.overrides.On("ReadPathOverrides", "tenant-a").Return(Config{
		EnableQueryBackend:     true,
		EnableQueryBackendFrom: time.Unix(20, 0),
	})

	req1 := connect.NewRequest(&typesv1.LabelNamesRequest{Start: 10, End: 19999})
	resp1 := connect.NewResponse(&typesv1.LabelNamesResponse{Names: []string{"foo", "bar"}})
	s.frontend.On("LabelNames", mock.Anything, req1).Return(resp1, nil).Once()

	req2 := connect.NewRequest(&typesv1.LabelNamesRequest{Start: 20000, End: math.MaxInt64})
	resp2 := connect.NewResponse(&typesv1.LabelNamesResponse{Names: []string{"baz", "foo", "qux"}})
	s.backend.On("LabelNames", mock.Anything, req2).Return(resp2, nil).Once()

	expected := connect.NewResponse(&typesv1.LabelNamesResponse{Names: []string{"bar", "baz", "foo", "qux"}})
	resp, err := s.router.LabelNames(s.ctx, connect.NewRequest(&typesv1.LabelNamesRequest{
		Start: 10,
		End:   math.MaxInt64,
	}))

	s.Require().NoError(err)
	s.Assert().Equal(expected, resp)
}

func (s *routerTestSuite) Test_Combined_BeforeSplit() {
	s.overrides.On("ReadPathOverrides", "tenant-a").Return(Config{
		EnableQueryBackend:     true,
		EnableQueryBackendFrom: time.Unix(20, 0),
	})

	expected := connect.NewResponse(&typesv1.LabelNamesResponse{Names: []string{"foo", "bar"}})
	req := connect.NewRequest(&typesv1.LabelNamesRequest{Start: 10, End: 10000})
	s.frontend.On("LabelNames", mock.Anything, req).Return(expected, nil).Once()

	resp, err := s.router.LabelNames(s.ctx, req)
	s.Require().NoError(err)
	s.Assert().Equal(expected, resp)
}

func (s *routerTestSuite) Test_Combined_AfterSplit() {
	s.overrides.On("ReadPathOverrides", "tenant-a").Return(Config{
		EnableQueryBackend:     true,
		EnableQueryBackendFrom: time.Unix(20, 0),
	})

	expected := connect.NewResponse(&typesv1.LabelNamesResponse{Names: []string{"foo", "bar"}})
	req := connect.NewRequest(&typesv1.LabelNamesRequest{Start: 30000, End: 40000})
	s.backend.On("LabelNames", mock.Anything, req).Return(expected, nil).Once()

	resp, err := s.router.LabelNames(s.ctx, req)
	s.Require().NoError(err)
	s.Assert().Equal(expected, resp)
}

func (s *routerTestSuite) Test_LabelNames() {
	s.overrides.On("ReadPathOverrides", "tenant-a").Return(Config{
		EnableQueryBackend:     true,
		EnableQueryBackendFrom: time.Unix(5, 0),
	})

	req := connect.NewRequest(&typesv1.LabelNamesRequest{Start: 10, End: 10000})
	expected := connect.NewResponse(&typesv1.LabelNamesResponse{Names: []string{"bar", "foo"}})
	s.frontend.On("LabelNames", mock.Anything, mock.Anything).Return(expected, nil).Once()
	s.backend.On("LabelNames", mock.Anything, mock.Anything).Return(expected, nil).Once()

	resp, err := s.router.LabelNames(s.ctx, req)
	s.Require().NoError(err)
	s.Assert().Equal(expected, resp)
}

func (s *routerTestSuite) Test_LabelValues() {
	s.overrides.On("ReadPathOverrides", "tenant-a").Return(Config{
		EnableQueryBackend:     true,
		EnableQueryBackendFrom: time.Unix(5, 0),
	})

	req := connect.NewRequest(&typesv1.LabelValuesRequest{Start: 10, End: 10000})
	expected := connect.NewResponse(&typesv1.LabelValuesResponse{Names: []string{"bar", "foo"}})
	s.frontend.On("LabelValues", mock.Anything, mock.Anything).Return(expected, nil).Once()
	s.backend.On("LabelValues", mock.Anything, mock.Anything).Return(expected, nil).Once()

	resp, err := s.router.LabelValues(s.ctx, req)
	s.Require().NoError(err)
	s.Assert().Equal(expected, resp)
}

func (s *routerTestSuite) Test_Series() {
	s.overrides.On("ReadPathOverrides", "tenant-a").Return(Config{
		EnableQueryBackend:     true,
		EnableQueryBackendFrom: time.Unix(5, 0),
	})

	req := connect.NewRequest(&querierv1.SeriesRequest{Start: 10, End: 10000})
	expected := connect.NewResponse(&querierv1.SeriesResponse{
		LabelsSet: []*typesv1.Labels{
			{Labels: []*typesv1.LabelPair{{Name: "foo", Value: "bar"}}},
		},
	})

	s.frontend.On("Series", mock.Anything, mock.Anything).Return(expected, nil).Once()
	s.backend.On("Series", mock.Anything, mock.Anything).Return(expected, nil).Once()

	resp, err := s.router.Series(s.ctx, req)
	s.Require().NoError(err)
	s.Assert().Equal(expected, resp)
}

func (s *routerTestSuite) Test_TimeSeries_Limit() {
	s.overrides.On("ReadPathOverrides", "tenant-a").Return(Config{
		EnableQueryBackend:     true,
		EnableQueryBackendFrom: time.Unix(5, 0),
	})

	one := int64(1)
	req := connect.NewRequest(&querierv1.SelectSeriesRequest{Start: 10, End: 10000, Limit: &one})
	expected := connect.NewResponse(&querierv1.SelectSeriesResponse{
		Series: []*typesv1.Series{
			{Labels: model.LabelsFromStrings("foo", "baz"), Points: []*typesv1.Point{{Timestamp: 1, Value: 3}}},
		},
	})

	s.frontend.On("SelectSeries",
		mock.Anything, connect.NewRequest(&querierv1.SelectSeriesRequest{Start: 10, End: 4999})).
		Return(connect.NewResponse(&querierv1.SelectSeriesResponse{
			Series: []*typesv1.Series{
				{Labels: model.LabelsFromStrings("foo", "bar"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}}},
				{Labels: model.LabelsFromStrings("foo", "baz"), Points: []*typesv1.Point{{Timestamp: 1, Value: 2}}},
			}}), nil).Once()

	s.backend.On("SelectSeries",
		mock.Anything, connect.NewRequest(&querierv1.SelectSeriesRequest{Start: 5000, End: 10000})).
		Return(connect.NewResponse(&querierv1.SelectSeriesResponse{
			Series: []*typesv1.Series{
				{Labels: model.LabelsFromStrings("foo", "bar"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}}},
				{Labels: model.LabelsFromStrings("foo", "baz"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}}},
			}}), nil).Once()

	resp, err := s.router.SelectSeries(s.ctx, req)
	s.Require().NoError(err)
	s.Assert().Equal(expected, resp)
}

func (s *routerTestSuite) Test_TimeSeries_NoLimit() {
	s.overrides.On("ReadPathOverrides", "tenant-a").Return(Config{
		EnableQueryBackend:     true,
		EnableQueryBackendFrom: time.Unix(5, 0),
	})

	req := connect.NewRequest(&querierv1.SelectSeriesRequest{Start: 10, End: 10000})
	expected := connect.NewResponse(&querierv1.SelectSeriesResponse{
		Series: []*typesv1.Series{
			{Labels: model.LabelsFromStrings("foo", "baz"), Points: []*typesv1.Point{{Timestamp: 1, Value: 3}}},
			{Labels: model.LabelsFromStrings("foo", "bar"), Points: []*typesv1.Point{{Timestamp: 1, Value: 2}}},
		},
	})

	s.frontend.On("SelectSeries",
		mock.Anything, connect.NewRequest(&querierv1.SelectSeriesRequest{Start: 10, End: 4999})).
		Return(connect.NewResponse(&querierv1.SelectSeriesResponse{
			Series: []*typesv1.Series{
				{Labels: model.LabelsFromStrings("foo", "bar"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}}},
				{Labels: model.LabelsFromStrings("foo", "baz"), Points: []*typesv1.Point{{Timestamp: 1, Value: 2}}},
			}}), nil).Once()

	s.backend.On("SelectSeries",
		mock.Anything, connect.NewRequest(&querierv1.SelectSeriesRequest{Start: 5000, End: 10000})).
		Return(connect.NewResponse(&querierv1.SelectSeriesResponse{
			Series: []*typesv1.Series{
				{Labels: model.LabelsFromStrings("foo", "bar"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}}},
				{Labels: model.LabelsFromStrings("foo", "baz"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}}},
			}}), nil).Once()

	resp, err := s.router.SelectSeries(s.ctx, req)
	s.Require().NoError(err)
	s.Assert().Equal(expected, resp)
}
