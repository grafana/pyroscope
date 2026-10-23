package connpool

import (
	"testing"

	"github.com/grafana/dskit/ring"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestConnFactory_Close(t *testing.T) {
	options := func(ring.InstanceDesc) []grpc.DialOption {
		return []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	}
	inst := ring.InstanceDesc{Addr: "localhost:0"}

	tests := []struct {
		name     string
		withHook bool
		want     []string
	}{
		{name: "onClose receives the instance address", withHook: true, want: []string{"localhost:0"}},
		{name: "nil onClose is safe", withHook: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var closed []string
			var onClose func(addr string)
			if tc.withHook {
				onClose = func(addr string) { closed = append(closed, addr) }
			}
			c, err := NewConnPoolFactory(options, onClose).FromInstance(inst)
			require.NoError(t, err)
			require.Empty(t, closed)
			require.NoError(t, c.Close())
			require.Equal(t, tc.want, closed)
		})
	}
}
