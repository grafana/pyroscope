// Copyright 2021-2022 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package connect

import (
	"context"
	"net/http"
)

// A Handler is the server-side implementation of a single RPC defined by a
// Protocol Buffers service.
//
// By default, Handlers support the Connect, gRPC, and gRPC-Web protocols with
// the binary Protobuf and JSON codecs. They support gzip compression using the
// standard library's compress/gzip.
type Handler struct {
	spec             Spec
	interceptor      Interceptor
	implementation   func(context.Context, Sender, Receiver, error /* client-visible */)
	protocolHandlers []protocolHandler
	acceptPost       string // Accept-Post header
}

// NewUnaryHandler constructs a Handler for a request-response procedure.
func NewUnaryHandler[Req, Res any](
	procedure string,
	unary func(context.Context, *Request[Req]) (*Response[Res], error),
	options ...HandlerOption,
) *Handler {
	config := newHandlerConfig(procedure, options)
	// Given a (possibly failed) stream, how should we call the unary function?
	implementation := func(ctx context.Context, sender Sender, receiver Receiver, clientVisibleError error) {
		defer receiver.Close()

		var request *Request[Req]
		if clientVisibleError != nil {
			// The protocol implementation failed to establish a stream. To make the
			// resulting error visible to the interceptor stack, we still want to
			// call the wrapped unary Func. To do that safely, we need a useful
			// Message struct. (Note that we do *not* actually calling the handler's
			// implementation.)
			request = receiveUnaryRequestMetadata[Req](receiver)
		} else {
			var err error
			request, err = receiveUnaryRequest[Req](receiver)
			if err != nil {
				// Interceptors should see this error too. Just as above, they need a
				// useful Message.
				clientVisibleError = err
				request = receiveUnaryRequestMetadata[Req](receiver)
			}
		}

		untyped := UnaryFunc(func(ctx context.Context, request AnyRequest) (AnyResponse, error) {
			if clientVisibleError != nil {
				// We've already encountered an error, short-circuit before calling the
				// handler's implementation.
				return nil, clientVisibleError
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			typed, ok := request.(*Request[Req])
			if !ok {
				return nil, errorf(CodeInternal, "unexpected handler request type %T", request)
			}
			res, err := unary(ctx, typed)
			if err != nil {
				return nil, err
			}
			return res, nil
		})
		if interceptor := config.Interceptor; interceptor != nil {
			untyped = interceptor.WrapUnary(untyped)
		}

		response, err := untyped(ctx, request)
		if err != nil {
			_ = sender.Close(err)
			return
		}
		mergeHeaders(sender.Header(), response.Header())
		if trailers, ok := sender.Trailer(); ok {
			mergeHeaders(trailers, response.Trailer())
		}
		_ = sender.Close(sender.Send(response.Any()))
	}

	protocolHandlers := config.newProtocolHandlers(StreamTypeUnary)
	return &Handler{
		spec:             config.newSpec(StreamTypeUnary),
		interceptor:      nil, // already applied
		implementation:   implementation,
		protocolHandlers: protocolHandlers,
		acceptPost:       sortedAcceptPostValue(protocolHandlers),
	}
}

// NewClientStreamHandler constructs a Handler for a client streaming procedure.
func NewClientStreamHandler[Req, Res any](
	procedure string,
	implementation func(context.Context, *ClientStream[Req]) (*Response[Res], error),
	options ...HandlerOption,
) *Handler {
	return newStreamHandler(
		procedure,
		StreamTypeClient,
		func(ctx context.Context, sender Sender, receiver Receiver) {
			stream := &ClientStream[Req]{receiver: receiver}
			res, err := implementation(ctx, stream)
			if err != nil {
				_ = receiver.Close()
				_ = sender.Close(err)
				return
			}
			if err := receiver.Close(); err != nil {
				_ = sender.Close(err)
				return
			}
			mergeHeaders(sender.Header(), res.header)
			if trailer, ok := sender.Trailer(); ok {
				mergeHeaders(trailer, res.trailer)
			}
			_ = sender.Close(sender.Send(res.Msg))
		},
		options...,
	)
}

// NewServerStreamHandler constructs a Handler for a server streaming procedure.
func NewServerStreamHandler[Req, Res any](
	procedure string,
	implementation func(context.Context, *Request[Req], *ServerStream[Res]) error,
	options ...HandlerOption,
) *Handler {
	return newStreamHandler(
		procedure,
		StreamTypeServer,
		func(ctx context.Context, sender Sender, receiver Receiver) {
			stream := &ServerStream[Res]{sender: sender}
			request, err := receiveUnaryRequest[Req](receiver)
			if err != nil {
				_ = receiver.Close()
				_ = sender.Close(err)
				return
			}
			if err := receiver.Close(); err != nil {
				_ = sender.Close(err)
				return
			}
			err = implementation(ctx, request, stream)
			_ = sender.Close(err)
		},
		options...,
	)
}

// NewBidiStreamHandler constructs a Handler for a bidirectional streaming procedure.
func NewBidiStreamHandler[Req, Res any](
	procedure string,
	implementation func(context.Context, *BidiStream[Req, Res]) error,
	options ...HandlerOption,
) *Handler {
	return newStreamHandler(
		procedure,
		StreamTypeBidi,
		func(ctx context.Context, sender Sender, receiver Receiver) {
			stream := &BidiStream[Req, Res]{sender: sender, receiver: receiver}
			err := implementation(ctx, stream)
			_ = receiver.Close()
			_ = sender.Close(err)
		},
		options...,
	)
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(responseWriter http.ResponseWriter, request *http.Request) {
	// We don't need to defer functions  to close the request body or read to
	// EOF: the stream we construct later on already does that, and we only
	// return early when dealing with misbehaving clients. In those cases, it's
	// okay if we can't re-use the connection.
	isBidi := (h.spec.StreamType & StreamTypeBidi) == StreamTypeBidi
	if isBidi && request.ProtoMajor < 2 {
		responseWriter.WriteHeader(http.StatusHTTPVersionNotSupported)
		return
	}

	// The gRPC-HTTP2, gRPC-Web, and Connect protocols are all POST-only.
	if request.Method != http.MethodPost {
		// grpc-go returns a 500 here, but interoperability with non-gRPC HTTP
		// clients is better if we return a 405.
		responseWriter.Header().Set("Allow", http.MethodPost)
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	contentType := request.Header.Get("Content-Type")
	for _, protocolHandler := range h.protocolHandlers {
		if _, ok := protocolHandler.ContentTypes()[contentType]; !ok {
			continue
		}
		ctx, cancel, timeoutErr := protocolHandler.SetTimeout(request)
		if timeoutErr != nil {
			ctx = request.Context()
		}
		if cancel != nil {
			defer cancel()
		}
		if ic := h.interceptor; ic != nil {
			ctx = ic.WrapStreamContext(ctx)
		}
		// Most errors returned from protocolHandler.NewStream are caused by
		// invalid requests. For example, the client may have specified an invalid
		// timeout or an unavailable codec. We'd like those errors to be visible to
		// the interceptor chain, so we're going to capture them here and pass them
		// to the implementation.
		sender, receiver, clientVisibleError := protocolHandler.NewStream(
			responseWriter,
			request.WithContext(ctx),
		)
		if timeoutErr != nil {
			clientVisibleError = timeoutErr
		}
		// If NewStream or SetTimeout errored and the protocol doesn't want the
		// error sent to the client, sender and/or receiver may be nil. We still
		// want the error to be seen by interceptors, so we provide no-op Sender
		// and Receiver implementations.
		if clientVisibleError != nil && sender == nil {
			sender = newNopSender(h.spec, responseWriter.Header(), make(http.Header))
		}
		if clientVisibleError != nil && receiver == nil {
			receiver = newNopReceiver(h.spec, request.Header, request.Trailer)
		}
		if interceptor := h.interceptor; interceptor != nil {
			// Unary interceptors were handled in NewUnaryHandler.
			sender = interceptor.WrapStreamSender(ctx, sender)
			receiver = interceptor.WrapStreamReceiver(ctx, receiver)
		}
		h.implementation(ctx, sender, receiver, clientVisibleError)
		return
	}
	responseWriter.Header().Set("Accept-Post", h.acceptPost)
	responseWriter.WriteHeader(http.StatusUnsupportedMediaType)
}

type handlerConfig struct {
	CompressionPools map[string]*compressionPool
	Codecs           map[string]Codec
	CompressMinBytes int
	Interceptor      Interceptor
	Procedure        string
	HandleGRPC       bool
	HandleGRPCWeb    bool
	BufferPool       *bufferPool
}

func newHandlerConfig(procedure string, options []HandlerOption) *handlerConfig {
	protoPath := extractProtoPath(procedure)
	config := handlerConfig{
		Procedure:        protoPath,
		CompressionPools: make(map[string]*compressionPool),
		Codecs:           make(map[string]Codec),
		HandleGRPC:       true,
		HandleGRPCWeb:    true,
		BufferPool:       newBufferPool(),
	}
	withProtoBinaryCodec().applyToHandler(&config)
	withProtoJSONCodec().applyToHandler(&config)
	withGzip().applyToHandler(&config)
	for _, opt := range options {
		opt.applyToHandler(&config)
	}
	return &config
}

func (c *handlerConfig) newSpec(streamType StreamType) Spec {
	return Spec{
		Procedure:  c.Procedure,
		StreamType: streamType,
	}
}

func (c *handlerConfig) newProtocolHandlers(streamType StreamType) []protocolHandler {
	protocols := []protocol{&protocolConnect{}}
	if c.HandleGRPC {
		protocols = append(protocols, &protocolGRPC{web: false})
	}
	if c.HandleGRPCWeb {
		protocols = append(protocols, &protocolGRPC{web: true})
	}
	handlers := make([]protocolHandler, 0, len(protocols))
	codecs := newReadOnlyCodecs(c.Codecs)
	compressors := newReadOnlyCompressionPools(c.CompressionPools)
	for _, protocol := range protocols {
		handlers = append(handlers, protocol.NewHandler(&protocolHandlerParams{
			Spec:             c.newSpec(streamType),
			Codecs:           codecs,
			CompressionPools: compressors,
			CompressMinBytes: c.CompressMinBytes,
			BufferPool:       c.BufferPool,
		}))
	}
	return handlers
}

func newStreamHandler(
	procedure string,
	streamType StreamType,
	implementation func(context.Context, Sender, Receiver),
	options ...HandlerOption,
) *Handler {
	config := newHandlerConfig(procedure, options)
	protocolHandlers := config.newProtocolHandlers(streamType)
	return &Handler{
		spec:        config.newSpec(streamType),
		interceptor: config.Interceptor,
		implementation: func(ctx context.Context, sender Sender, receiver Receiver, clientVisibleErr error) {
			if clientVisibleErr != nil {
				_ = receiver.Close()
				_ = sender.Close(clientVisibleErr)
				return
			}
			implementation(ctx, sender, receiver)
		},
		protocolHandlers: protocolHandlers,
		acceptPost:       sortedAcceptPostValue(protocolHandlers),
	}
}
