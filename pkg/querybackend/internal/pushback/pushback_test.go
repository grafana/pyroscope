package pushback

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestMark(t *testing.T) {
	original := status.Error(codes.ResourceExhausted, "response too large")
	marked := Mark(original)

	require.ErrorIs(t, marked, original)
	require.True(t, IsMarked(marked))
	require.Equal(t, marked, Mark(marked), "marking twice must not nest")
	require.Nil(t, Mark(nil))

	// Without the status, the caller sees codes.Unknown instead of the failure.
	require.Equal(t, codes.ResourceExhausted, status.Code(marked))
	wrapped := fmt.Errorf("merge query: %w", marked)
	require.True(t, IsMarked(wrapped))
	require.Equal(t, codes.ResourceExhausted, status.Code(wrapped))

	require.False(t, IsMarked(original))
	require.False(t, IsMarked(nil))
}

func TestIsNoRetry(t *testing.T) {
	tests := []struct {
		name string
		md   metadata.MD
		want bool
	}{
		{name: "missing"},
		{name: "retry now", md: metadata.Pairs(metadataKey, "0")},
		{name: "retry later", md: metadata.Pairs(metadataKey, "100")},
		{name: "do not retry", md: metadata.Pairs(metadataKey, noRetry), want: true},
		{name: "unparsable", md: metadata.Pairs(metadataKey, "soon"), want: true},
		// grpc-go does not retry on multiple values, whatever they say.
		{name: "multiple values", md: metadata.Pairs(metadataKey, "0", metadataKey, "0"), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsNoRetry(tt.md))
		})
	}
}

type fakeStream struct{ trailer metadata.MD }

func (*fakeStream) Method() string               { return "" }
func (*fakeStream) SetHeader(metadata.MD) error  { return nil }
func (*fakeStream) SendHeader(metadata.MD) error { return nil }
func (s *fakeStream) SetTrailer(md metadata.MD) error {
	s.trailer = metadata.Join(s.trailer, md)
	return nil
}

func TestSetNoRetry(t *testing.T) {
	stream := &fakeStream{}
	SetNoRetry(grpc.NewContextWithServerTransportStream(context.Background(), stream))

	// The wire format grpc-go reads; only a negative value stops a retry.
	require.Equal(t, []string{"-1"}, stream.trailer.Get("grpc-retry-pushback-ms"))
	require.True(t, IsNoRetry(stream.trailer))

	// Outside a gRPC handler there is no client to inform.
	require.NotPanics(t, func() { SetNoRetry(context.Background()) })
}
