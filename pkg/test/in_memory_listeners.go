package test

import (
	"context"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

// Create in-memory listeners at given addresses.
// Also returns gRPC dial option for a client to connect to the appropriate in-memory listener
// for a given address.
func CreateInMemoryListeners(addresses []string) (map[string]*bufconn.Listener, grpc.DialOption) {
	listeners := make(map[string]*bufconn.Listener)
	for _, a := range addresses {
		el := bufconn.Listen(256 << 10)
		listeners[a] = el
	}
	dialer := func(_ context.Context, address string) (net.Conn, error) {
		el := listeners[address]
		if el != nil {
			return el.Dial()
		}
		return net.Dial("tcp", address)
	}
	return listeners, grpc.WithContextDialer(dialer)
}
