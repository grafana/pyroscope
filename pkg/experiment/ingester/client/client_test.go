package segmentwriterclient

import (
	"context"
	"flag"
	"io"
	"net"
	"os"
	"testing"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/ring"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	"github.com/grafana/pyroscope/pkg/testhelper"
)

type segwriterServerMock struct {
	segmentwriterv1.UnimplementedSegmentWriterServiceServer
	mock.Mock
}

func (m *segwriterServerMock) Push(
	ctx context.Context,
	req *segmentwriterv1.PushRequest,
) (*segmentwriterv1.PushResponse, error) {
	args := m.Called(ctx, req)
	return args.Get(0).(*segmentwriterv1.PushResponse), args.Error(1)
}

type segwriterClientSuite struct {
	suite.Suite

	listener *bufconn.Listener
	dialer   func(context.Context, string) (net.Conn, error)
	server   *grpc.Server
	service  *segwriterServerMock
	done     chan struct{}

	logger log.Logger
	config grpcclient.Config
	ring   testhelper.MockRing
	client *Client
}

func (s *segwriterClientSuite) SetupTest() {
	s.listener = bufconn.Listen(256 << 10)
	s.dialer = func(context.Context, string) (net.Conn, error) { return s.listener.Dial() }
	s.server = grpc.NewServer()
	s.service = new(segwriterServerMock)
	segmentwriterv1.RegisterSegmentWriterServiceServer(s.server, s.service)

	s.logger = log.NewLogfmtLogger(os.Stdout)
	s.config = grpcclient.Config{}
	s.config.RegisterFlags(flag.NewFlagSet("", flag.PanicOnError))
	instances := []ring.InstanceDesc{
		{Addr: "a", Tokens: make([]uint32, 1)},
		{Addr: "b", Tokens: make([]uint32, 1)},
		{Addr: "c", Tokens: make([]uint32, 1)},
	}
	s.ring = testhelper.NewMockRing(instances, 1)

	var err error
	s.client, err = NewSegmentWriterClient(s.config, s.logger, s.ring, grpc.WithContextDialer(s.dialer))
	s.Require().NoError(err)

	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		s.Require().NoError(s.server.Serve(s.listener))
	}()
}

func (s *segwriterClientSuite) AfterTest() {
	s.service.AssertExpectations(s.T())
}

func (s *segwriterClientSuite) TearDownTest() {
	s.server.GracefulStop()
	<-s.done
}

func TestSegmentWriterClientSuite(t *testing.T) { suite.Run(t, new(segwriterClientSuite)) }

func (s *segwriterClientSuite) Test_Push_HappyPath() {
	s.service.On("Push", mock.Anything, mock.Anything).
		Return(&segmentwriterv1.PushResponse{}, nil).
		Once()

	_, err := s.client.Push(context.Background(), &segmentwriterv1.PushRequest{})
	s.Assert().NoError(err)
}

func (s *segwriterClientSuite) Test_Push_EmptyRing() {
	emptyRing := testhelper.NewMockRing(nil, 1)
	var err error
	s.client, err = NewSegmentWriterClient(s.config, s.logger, emptyRing, grpc.WithContextDialer(s.dialer))
	s.Require().NoError(err)

	_, err = s.client.Push(context.Background(), &segmentwriterv1.PushRequest{})
	s.Assert().Equal(codes.Unavailable.String(), status.Code(err).String())
}

func (s *segwriterClientSuite) Test_Push_ClientError_Cancellation() {
	s.service.On("Push", mock.Anything, mock.Anything).
		Return(new(segmentwriterv1.PushResponse), context.Canceled).
		Once()

	_, err := s.client.Push(context.Background(), &segmentwriterv1.PushRequest{})
	s.Assert().Equal(codes.Canceled.String(), status.Code(err).String())
}

func (s *segwriterClientSuite) Test_Push_ClientError_InvalidArgument() {
	s.service.On("Push", mock.Anything, mock.Anything).
		Return(new(segmentwriterv1.PushResponse), status.Error(codes.InvalidArgument, errServiceUnavailableMsg)).
		Once()

	_, err := s.client.Push(context.Background(), &segmentwriterv1.PushRequest{})
	s.Assert().Equal(codes.InvalidArgument.String(), status.Code(err).String())
}

func (s *segwriterClientSuite) Test_Push_ServerError_NonRetryable() {
	s.service.On("Push", mock.Anything, mock.Anything).
		Return(new(segmentwriterv1.PushResponse), io.EOF).
		Once()

	_, err := s.client.Push(context.Background(), &segmentwriterv1.PushRequest{})
	s.Assert().Equal(codes.Unavailable.String(), status.Code(err).String())
}

func (s *segwriterClientSuite) Test_Push_ServerError_Retry_Unavailable() {
	s.service.On("Push", mock.Anything, mock.Anything).
		Return(new(segmentwriterv1.PushResponse), status.Error(codes.Unavailable, errServiceUnavailableMsg)).
		Once()

	s.service.On("Push", mock.Anything, mock.Anything).
		Return(new(segmentwriterv1.PushResponse), nil).
		Once()

	_, err := s.client.Push(context.Background(), &segmentwriterv1.PushRequest{})
	s.Assert().NoError(err)
}

func (s *segwriterClientSuite) Test_Push_ServerError_Retry_ResourceExhausted() {
	s.service.On("Push", mock.Anything, mock.Anything).
		Return(new(segmentwriterv1.PushResponse), status.Error(codes.ResourceExhausted, errServiceUnavailableMsg)).
		Once()

	s.service.On("Push", mock.Anything, mock.Anything).
		Return(new(segmentwriterv1.PushResponse), nil).
		Once()

	_, err := s.client.Push(context.Background(), &segmentwriterv1.PushRequest{})
	s.Assert().NoError(err)
}

func (s *segwriterClientSuite) Test_Push_DialError() {
	dialer := func(ctx context.Context, s string) (net.Conn, error) {
		return nil, io.EOF
	}
	var err error
	s.client, err = NewSegmentWriterClient(s.config, s.logger, s.ring, grpc.WithContextDialer(dialer))
	s.Require().NoError(err)

	_, err = s.client.Push(context.Background(), &segmentwriterv1.PushRequest{})
	s.Assert().Equal(codes.Unavailable.String(), status.Code(err).String())
}

func (s *segwriterClientSuite) Test_Push_DialError_Retry() {
	var failed bool
	dialer := func(context.Context, string) (net.Conn, error) {
		if failed {
			return nil, net.UnknownNetworkError("network issue")
		}
		failed = true
		return s.listener.Dial()
	}
	var err error
	s.client, err = NewSegmentWriterClient(s.config, s.logger, s.ring, grpc.WithContextDialer(dialer))
	s.Require().NoError(err)

	s.service.On("Push", mock.Anything, mock.Anything).
		Return(new(segmentwriterv1.PushResponse), nil).
		Once()

	_, err = s.client.Push(context.Background(), &segmentwriterv1.PushRequest{})
	s.Assert().NoError(err)
}

func (s *segwriterClientSuite) Test_Push_AllInstancesUnavailable() {
	s.service.On("Push", mock.Anything, mock.Anything).
		Return(new(segmentwriterv1.PushResponse), status.Error(codes.Unavailable, errServiceUnavailableMsg)).
		Times(3)

	_, err := s.client.Push(context.Background(), &segmentwriterv1.PushRequest{})
	s.Assert().Equal(codes.Unavailable.String(), status.Code(err).String())
}
