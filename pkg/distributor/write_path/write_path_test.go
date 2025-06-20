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
	ingester  *mockwritepath.MockIngesterClient
	segwriter *mockwritepath.MockIngesterClient

	request *distributormodel.PushRequest
}

func (s *routerTestSuite) SetupTest() {
	s.logger = log.NewLogfmtLogger(io.Discard)
	s.registry = prometheus.NewRegistry()
	s.ingester = new(mockwritepath.MockIngesterClient)
	s.segwriter = new(mockwritepath.MockIngesterClient)

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
				Annotations: []*typesv1.ProfileAnnotation{
					{Key: "foo", Value: "bar"},
				},
			},
		},
	}

	s.router = NewRouter(
		s.logger,
		s.registry,
		s.ingester,
		s.segwriter,
	)
}

func (s *routerTestSuite) BeforeTest(_, _ string) {
	svc := s.router.Service()
	s.Require().NoError(svc.StartAsync(context.Background()))
	s.Require().NoError(svc.AwaitRunning(context.Background()))
	s.Require().Equal(services.Running, svc.State())
}

func (s *routerTestSuite) AfterTest(_, _ string) {
	svc := s.router.Service()
	svc.StopAsync()
	s.Require().NoError(svc.AwaitTerminated(context.Background()))
	s.Require().Equal(services.Terminated, svc.State())

	s.ingester.AssertExpectations(s.T())
	s.segwriter.AssertExpectations(s.T())
}

func TestRouterSuite(t *testing.T) { suite.Run(t, new(routerTestSuite)) }

func (s *routerTestSuite) Test_IngesterPath() {
	config := Config{
		WritePath: IngesterPath,
	}

	s.ingester.On("Push", mock.Anything, s.request).
		Return(new(connect.Response[pushv1.PushResponse]), nil).
		Once()

	s.Assert().NoError(s.router.Send(context.Background(), s.request, config))
}

func (s *routerTestSuite) Test_SegmentWriterPath() {
	config := Config{
		WritePath: SegmentWriterPath,
	}

	s.segwriter.On("Push", mock.Anything, mock.Anything).
		Return(new(connect.Response[pushv1.PushResponse]), nil).
		Once()

	s.Assert().NoError(s.router.Send(context.Background(), s.request, config))
}

func (s *routerTestSuite) Test_CombinedPath() {
	const (
		N = 100
		w = 10 // Concurrent workers.
		f = 0.5
		d = 0.3 // Allowed delta: note that f is just a probability.
	)

	config := Config{
		WritePath:           CombinedPath,
		IngesterWeight:      1,
		SegmentWriterWeight: f,
	}

	var sentIngester atomic.Uint32
	s.ingester.On("Push", mock.Anything, mock.Anything).
		Run(func(m mock.Arguments) {
			sentIngester.Add(1)
			// Assert that no race condition occurs: we delete series
			// attempting to access it concurrently with segment writer
			// that should convert the distributor request to a segment
			// writer request.
			m.Get(1).(*distributormodel.PushRequest).Series = nil
		}).
		Return(new(connect.Response[pushv1.PushResponse]), nil)

	var sentSegwriter atomic.Uint32
	s.segwriter.On("Push", mock.Anything, mock.Anything).
		Run(func(m mock.Arguments) {
			sentSegwriter.Add(1)
			m.Get(1).(*distributormodel.PushRequest).Series = nil
		}).
		Return(new(connect.Response[pushv1.PushResponse]), nil)

	for i := 0; i < w; i++ {
		for j := 0; j < N; j++ {
			s.Assert().NoError(s.router.Send(context.Background(), s.request.Clone(), config))
		}
	}

	s.router.inflight.Wait()
	expected := N * f * w
	delta := expected * d
	s.Assert().Equal(N*w, int(sentIngester.Load()))
	s.Assert().Greater(int(sentSegwriter.Load()), int(expected-delta))
	s.Assert().Less(int(sentSegwriter.Load()), int(expected+delta))
}

func (s *routerTestSuite) Test_UnspecifiedWriterPath() {
	config := Config{} // Default should route to ingester

	s.ingester.On("Push", mock.Anything, mock.Anything).
		Return(new(connect.Response[pushv1.PushResponse]), nil).
		Once()

	s.Assert().NoError(s.router.Send(context.Background(), s.request, config))
}

func (s *routerTestSuite) Test_CombinedPath_ZeroWeights() {
	config := Config{
		WritePath: CombinedPath,
	}

	s.Assert().NoError(s.router.Send(context.Background(), s.request, config))
}

func (s *routerTestSuite) Test_CombinedPath_IngesterError() {
	config := Config{
		WritePath: CombinedPath,
		// We ensure that request is sent to both.
		IngesterWeight:      1,
		SegmentWriterWeight: 1,
	}

	s.segwriter.On("Push", mock.Anything, mock.Anything).
		Return(new(connect.Response[pushv1.PushResponse]), nil).
		Once()

	s.ingester.On("Push", mock.Anything, mock.Anything).
		Return(new(connect.Response[pushv1.PushResponse]), context.Canceled).
		Once()

	s.Assert().Error(s.router.Send(context.Background(), s.request, config), context.Canceled)
}

func (s *routerTestSuite) Test_CombinedPath_SegmentWriterError() {
	config := Config{
		WritePath: CombinedPath,
		// We ensure that request is sent to both.
		IngesterWeight:      1,
		SegmentWriterWeight: 1,
	}

	s.segwriter.On("Push", mock.Anything, mock.Anything).
		Return(new(connect.Response[pushv1.PushResponse]), context.Canceled).
		Once()

	s.ingester.On("Push", mock.Anything, mock.Anything).
		Return(new(connect.Response[pushv1.PushResponse]), nil).
		Once()

	s.Assert().NoError(s.router.Send(context.Background(), s.request, config))
}

func (s *routerTestSuite) Test_CombinedPath_Ingester_Exclusive_Error() {
	config := Config{
		WritePath: CombinedPath,
		// The request is only sent to ingester.
		IngesterWeight:      1,
		SegmentWriterWeight: 0,
	}

	s.ingester.On("Push", mock.Anything, mock.Anything).
		Return(new(connect.Response[pushv1.PushResponse]), context.Canceled).
		Once()

	s.Assert().Error(s.router.Send(context.Background(), s.request, config), context.Canceled)
}

func (s *routerTestSuite) Test_CombinedPath_SegmentWriter_Exclusive_Error() {
	config := Config{
		WritePath: CombinedPath,
		// The request is only sent to segment writer.
		IngesterWeight:      0,
		SegmentWriterWeight: 1,
	}

	s.segwriter.On("Push", mock.Anything, mock.Anything).
		Return(new(connect.Response[pushv1.PushResponse]), context.Canceled).
		Once()

	s.Assert().Error(s.router.Send(context.Background(), s.request, config), context.Canceled)
}

func (s *routerTestSuite) Test_SegmentWriter_MultipleProfiles() {
	config := Config{
		WritePath:           SegmentWriterPath,
		IngesterWeight:      0,
		SegmentWriterWeight: 1,
	}

	x := s.request.Series[0]
	x.Samples = append(x.Samples, &distributormodel.ProfileSample{Profile: &pprof.Profile{}})

	s.segwriter.On("Push", mock.Anything, mock.Anything).
		Return(new(connect.Response[pushv1.PushResponse]), nil).
		Once()

	s.Assert().NoError(s.router.Send(context.Background(), s.request, config))
}

func (s *routerTestSuite) Test_AsyncIngest_Synchronous() {
	config := Config{
		WritePath:   SegmentWriterPath,
		AsyncIngest: false,
	}

	s.segwriter.On("Push", mock.Anything, mock.Anything).
		Return(new(connect.Response[pushv1.PushResponse]), context.Canceled).
		Once()

	err := s.router.Send(context.Background(), s.request, config)
	s.Assert().Error(err)
}

func (s *routerTestSuite) Test_AsyncIngest_Asynchronous() {
	config := Config{
		WritePath:   SegmentWriterPath,
		AsyncIngest: true,
	}

	s.segwriter.On("Push", mock.Anything, mock.Anything).
		Return(new(connect.Response[pushv1.PushResponse]), context.Canceled).
		Once()

	err := s.router.Send(context.Background(), s.request, config)
	s.Assert().NoError(err)

	s.router.inflight.Wait()
}

func (s *routerTestSuite) Test_AsyncIngest_CombinedPath() {
	config := Config{
		WritePath:           CombinedPath,
		IngesterWeight:      1,
		SegmentWriterWeight: 1,
		AsyncIngest:         true,
	}

	s.ingester.On("Push", mock.Anything, mock.Anything).
		Return(new(connect.Response[pushv1.PushResponse]), context.Canceled).
		Once()

	s.segwriter.On("Push", mock.Anything, mock.Anything).
		Return(new(connect.Response[pushv1.PushResponse]), context.Canceled).
		Once()

	err := s.router.Send(context.Background(), s.request, config)
	s.Assert().Error(err)

	s.router.inflight.Wait()
}
