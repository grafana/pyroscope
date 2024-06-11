package connectapi

import (
	"connectrpc.com/connect"
)

func DefaultClientOptions() []connect.ClientOption {
	return []connect.ClientOption{
		connect.WithCodec(ProtoCodec),
		WithGzipClient(),
	}
}

func DefaultHandlerOptions() []connect.HandlerOption {
	return []connect.HandlerOption{
		connect.WithCodec(ProtoCodec),
		WithGzipHandler(),
	}
}
