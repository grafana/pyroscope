package queryfrontend

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

func TestSpanPresenceValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		request *querierv1.SelectSpanPresenceRequest
	}{
		{"empty selectors", &querierv1.SelectSpanPresenceRequest{Start: 1, End: 2}},
		{"invalid range", &querierv1.SelectSpanPresenceRequest{Start: 2, End: 1, SpanSelector: []string{"0000000000000001"}}},
		{"invalid span", &querierv1.SelectSpanPresenceRequest{Start: 1, End: 2, SpanSelector: []string{"bad"}}},
		{"invalid trace", &querierv1.SelectSpanPresenceRequest{Start: 1, End: 2, TraceIdSelector: []string{"bad"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var q QueryFrontend
			_, err := q.SelectSpanPresence(context.Background(), connect.NewRequest(tc.request))
			require.Error(t, err)
			require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		})
	}
}
