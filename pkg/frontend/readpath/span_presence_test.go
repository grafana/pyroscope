package readpath

import (
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/mock"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

func (s *routerTestSuite) TestSpanPresenceV2() {
	s.overrides.On("ReadPathOverrides", "tenant-a").Return(Config{EnableQueryBackend: true})
	req := connect.NewRequest(&querierv1.SelectSpanPresenceRequest{Start: 10000, End: 20000, SpanSelector: []string{"0000000000000001"}})
	expected := connect.NewResponse(&querierv1.SelectSpanPresenceResponse{})
	s.newFrontend.On("SelectSpanPresence", mock.Anything, req).Return(expected, nil).Once()
	resp, err := s.router.SelectSpanPresence(s.ctx, req)
	s.Require().NoError(err)
	s.Equal(expected, resp)
}

func (s *routerTestSuite) TestSpanPresenceRejectsV1AndMixed() {
	for _, tc := range []struct {
		name string
		cfg  Config
	}{
		{name: "v1"},
		{name: "mixed", cfg: Config{EnableQueryBackend: true, EnableQueryBackendFrom: QueryBackendFrom{Time: time.Unix(15, 0)}}},
	} {
		s.Run(tc.name, func() {
			s.overrides.On("ReadPathOverrides", "tenant-a").Return(tc.cfg).Once()
			_, err := s.router.SelectSpanPresence(s.ctx, connect.NewRequest(&querierv1.SelectSpanPresenceRequest{Start: 10000, End: 20000, SpanSelector: []string{"0000000000000001"}}))
			s.Require().Error(err)
			s.Equal(connect.CodeUnimplemented, connect.CodeOf(err))
			s.oldFrontend.AssertNotCalled(s.T(), "SelectSpanPresence", mock.Anything, mock.Anything)
			s.newFrontend.AssertNotCalled(s.T(), "SelectSpanPresence", mock.Anything, mock.Anything)
		})
	}
}
