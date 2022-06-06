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
	"compress/gzip"
	"io/ioutil"
)

// A ClientOption configures a connect client.
//
// In addition to any options grouped in the documentation below, remember that
// Options are also valid ClientOptions.
type ClientOption interface {
	applyToClient(*clientConfig)
}

// WithAcceptCompression makes a compression algorithm available to a client.
// Clients ask servers to compress responses using any of the registered
// algorithms. It's safe to use this option liberally: servers will ignore any
// compression algorithms they don't support. To compress requests, pair this
// option with WithSendCompression.
//
// Clients accept gzipped requests by default, using a compressor backed by the
// standard library's gzip package with the default compression level. Use
// WithSendGzip to compress requests with gzip.
func WithAcceptCompression(
	name string,
	newDecompressor func() Decompressor,
	newCompressor func() Compressor,
) ClientOption {
	return &compressionOption{
		Name:            name,
		CompressionPool: newCompressionPool(newDecompressor, newCompressor),
	}
}

// WithClientOptions composes multiple ClientOptions into one.
func WithClientOptions(options ...ClientOption) ClientOption {
	return &clientOptionsOption{options}
}

// WithGRPC configures clients to use the HTTP/2 gRPC protocol.
func WithGRPC() ClientOption {
	return &grpcOption{web: false}
}

// WithGRPCWeb configures clients to use the gRPC-Web protocol.
func WithGRPCWeb() ClientOption {
	return &grpcOption{web: true}
}

// WithProtoJSON configures a client to send JSON-encoded data instead of
// binary Protobuf. It uses the standard Protobuf JSON mapping as implemented
// by google.golang.org/protobuf/encoding/protojson: fields are named using
// lowerCamelCase, zero values are omitted, missing required fields are errors,
// enums are emitted as strings, etc.
func WithProtoJSON() ClientOption {
	return WithCodec(&protoJSONCodec{})
}

// WithSendCompression configures the client to use the specified algorithm to
// compress request messages. If the algorithm has not been registered using
// WithAcceptCompression, the client will return errors at runtime.
//
// Because some servers don't support compression, clients default to sending
// uncompressed requests.
func WithSendCompression(name string) ClientOption {
	return &sendCompressionOption{Name: name}
}

// WithSendGzip configures the client to gzip requests. Since clients have
// access to a gzip compressor by default, WithSendGzip doesn't require
// WithSendCompresion.
//
// Some servers don't support gzip, so clients default to sending uncompressed
// requests.
func WithSendGzip() ClientOption {
	return WithSendCompression(compressionGzip)
}

// A HandlerOption configures a Handler.
//
// In addition to any options grouped in the documentation below, remember that
// all Options are also HandlerOptions.
type HandlerOption interface {
	applyToHandler(*handlerConfig)
}

// WithCompression configures handlers to support a compression algorithm.
// Clients may send messages compressed with that algorithm and/or request
// compressed responses. The Compressors and Decompressors produced by the
// supplied constructors must use the same algorithm. Internally, Connect pools
// compressors and decompressors.
//
// By default, handlers support gzip using the standard library's gzip package
// at the default compression level.
//
// Calling WithCompression with an empty name or nil constructors is a no-op.
func WithCompression(
	name string,
	newDecompressor func() Decompressor,
	newCompressor func() Compressor,
) HandlerOption {
	return &compressionOption{
		Name:            name,
		CompressionPool: newCompressionPool(newDecompressor, newCompressor),
	}
}

// WithHandlerOptions composes multiple HandlerOptions into one.
func WithHandlerOptions(options ...HandlerOption) HandlerOption {
	return &handlerOptionsOption{options}
}

// Option implements both ClientOption and HandlerOption, so it can be applied
// both client-side and server-side.
type Option interface {
	ClientOption
	HandlerOption
}

// WithCodec registers a serialization method with a client or handler.
// Handlers may have multiple codecs registered, and use whichever the client
// chooses. Clients may only have a single codec.
//
// By default, handlers and clients support binary Protocol Buffer data using
// google.golang.org/protobuf/proto. Handlers also support JSON by default,
// using the standard Protobuf JSON mapping. Users with more specialized needs
// may override the default codecs by registering a new codec under the "proto"
// or "json" names. When supplying a custom "proto" codec, keep in mind that
// some unexported, protocol-specific messages are serialized using Protobuf -
// take care to fall back to the standard Protobuf implementation if
// necessary.
//
// Registering a codec with an empty name is a no-op.
func WithCodec(codec Codec) Option {
	return &codecOption{Codec: codec}
}

// WithCompressMinBytes sets a minimum size threshold for compression:
// regardless of compressor configuration, messages smaller than the configured
// minimum are sent uncompressed.
//
// The default minimum is zero. Setting a minimum compression threshold may
// improve overall performance, because the CPU cost of compressing very small
// messages usually isn't worth the small reduction in network I/O.
func WithCompressMinBytes(min int) Option {
	return &compressMinBytesOption{Min: min}
}

// WithInterceptors configures a client or handler's interceptor stack. Repeated
// WithInterceptors options are applied in order, so
//
//   WithInterceptors(A) + WithInterceptors(B, C) == WithInterceptors(A, B, C)
//
// Unary interceptors compose like an onion. The first interceptor provided is
// the outermost layer of the onion: it acts first on the context and request,
// and last on the response and error.
//
// Stream interceptors also behave like an onion: the first interceptor
// provided is the first to wrap the context and is the outermost wrapper for
// the (Sender, Receiver) pair. It's the first to see sent messages and the
// last to see received messages.
//
// Applied to client and handler, WithInterceptors(A, B, ..., Y, Z) produces:
//
//        client.Send()     client.Receive()
//              |                 ^
//              v                 |
//           A ---               --- A
//           B ---               --- B
//             ...               ...
//           Y ---               --- Y
//           Z ---               --- Z
//              |                 ^
//              v                 |
//           network            network
//              |                 ^
//              v                 |
//           A ---               --- A
//           B ---               --- B
//             ...               ...
//           Y ---               --- Y
//           Z ---               --- Z
//              |                 ^
//              v                 |
//       handler.Receive() handler.Send()
//              |                 ^
//              |                 |
//              -> handler logic --
//
// Note that in clients, the Sender handles the request message(s) and the
// Receiver handles the response message(s). For handlers, it's the reverse.
// Depending on your interceptor's logic, you may need to wrap one side of the
// stream on the clients and the other side on handlers.
func WithInterceptors(interceptors ...Interceptor) Option {
	return &interceptorsOption{interceptors}
}

// WithOptions composes multiple Options into one.
func WithOptions(options ...Option) Option {
	return &optionsOption{options}
}

type clientOptionsOption struct {
	options []ClientOption
}

func (o *clientOptionsOption) applyToClient(config *clientConfig) {
	for _, option := range o.options {
		option.applyToClient(config)
	}
}

type codecOption struct {
	Codec Codec
}

func (o *codecOption) applyToClient(config *clientConfig) {
	if o.Codec == nil || o.Codec.Name() == "" {
		return
	}
	config.Codec = o.Codec
}

func (o *codecOption) applyToHandler(config *handlerConfig) {
	if o.Codec == nil || o.Codec.Name() == "" {
		return
	}
	config.Codecs[o.Codec.Name()] = o.Codec
}

type compressionOption struct {
	Name            string
	CompressionPool *compressionPool
}

func (o *compressionOption) applyToClient(config *clientConfig) {
	o.apply(config.CompressionPools)
}

func (o *compressionOption) applyToHandler(config *handlerConfig) {
	o.apply(config.CompressionPools)
}

func (o *compressionOption) apply(m map[string]*compressionPool) {
	if o.Name == "" || o.CompressionPool == nil {
		return
	}
	m[o.Name] = o.CompressionPool
}

type compressMinBytesOption struct {
	Min int
}

func (o *compressMinBytesOption) applyToClient(config *clientConfig) {
	config.CompressMinBytes = o.Min
}

func (o *compressMinBytesOption) applyToHandler(config *handlerConfig) {
	config.CompressMinBytes = o.Min
}

type handlerOptionsOption struct {
	options []HandlerOption
}

func (o *handlerOptionsOption) applyToHandler(config *handlerConfig) {
	for _, option := range o.options {
		option.applyToHandler(config)
	}
}

type grpcOption struct {
	web bool
}

func (o *grpcOption) applyToClient(config *clientConfig) {
	config.Protocol = &protocolGRPC{web: o.web}
}

type interceptorsOption struct {
	Interceptors []Interceptor
}

func (o *interceptorsOption) applyToClient(config *clientConfig) {
	config.Interceptor = o.chainWith(config.Interceptor)
}

func (o *interceptorsOption) applyToHandler(config *handlerConfig) {
	config.Interceptor = o.chainWith(config.Interceptor)
}

func (o *interceptorsOption) chainWith(current Interceptor) Interceptor {
	if len(o.Interceptors) == 0 {
		return current
	}
	if current == nil && len(o.Interceptors) == 1 {
		return o.Interceptors[0]
	}
	if current == nil && len(o.Interceptors) > 1 {
		return newChain(o.Interceptors)
	}
	return newChain(append([]Interceptor{current}, o.Interceptors...))
}

type optionsOption struct {
	options []Option
}

func (o *optionsOption) applyToClient(config *clientConfig) {
	for _, option := range o.options {
		option.applyToClient(config)
	}
}

func (o *optionsOption) applyToHandler(config *handlerConfig) {
	for _, option := range o.options {
		option.applyToHandler(config)
	}
}

type sendCompressionOption struct {
	Name string
}

func (o *sendCompressionOption) applyToClient(config *clientConfig) {
	config.RequestCompressionName = o.Name
}

func withGzip() Option {
	return &compressionOption{
		Name: compressionGzip,
		CompressionPool: newCompressionPool(
			func() Decompressor { return &gzip.Reader{} },
			func() Compressor { return gzip.NewWriter(ioutil.Discard) },
		),
	}
}

func withProtoBinaryCodec() Option {
	return WithCodec(&protoBinaryCodec{})
}

func withProtoJSONCodec() HandlerOption {
	return WithCodec(&protoJSONCodec{})
}
