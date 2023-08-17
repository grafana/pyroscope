// Copyright 2021-2023 Buf Technologies, Inc.
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
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/textproto"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	statusv1 "github.com/bufbuild/connect-go/internal/gen/connectext/grpc/status/v1"
)

const (
	grpcHeaderCompression       = "Grpc-Encoding"
	grpcHeaderAcceptCompression = "Grpc-Accept-Encoding"
	grpcHeaderTimeout           = "Grpc-Timeout"
	grpcHeaderStatus            = "Grpc-Status"
	grpcHeaderMessage           = "Grpc-Message"
	grpcHeaderDetails           = "Grpc-Status-Details-Bin"

	grpcFlagEnvelopeTrailer = 0b10000000

	grpcTimeoutMaxHours = math.MaxInt64 / int64(time.Hour) // how many hours fit into a time.Duration?
	grpcMaxTimeoutChars = 8                                // from gRPC protocol

	grpcContentTypeDefault    = "application/grpc"
	grpcWebContentTypeDefault = "application/grpc-web"
	grpcContentTypePrefix     = grpcContentTypeDefault + "+"
	grpcWebContentTypePrefix  = grpcWebContentTypeDefault + "+"
)

var (
	grpcTimeoutUnits = []struct {
		size time.Duration
		char byte
	}{
		{time.Nanosecond, 'n'},
		{time.Microsecond, 'u'},
		{time.Millisecond, 'm'},
		{time.Second, 'S'},
		{time.Minute, 'M'},
		{time.Hour, 'H'},
	}
	grpcTimeoutUnitLookup = make(map[byte]time.Duration)
	grpcAllowedMethods    = map[string]struct{}{
		http.MethodPost: {},
	}
	errTrailersWithoutGRPCStatus = fmt.Errorf("gRPC protocol error: no %s trailer", grpcHeaderStatus)

	// defaultGrpcUserAgent follows
	// https://github.com/grpc/grpc/blob/master/doc/PROTOCOL-HTTP2.md#user-agents:
	//
	//	While the protocol does not require a user-agent to function it is recommended
	//	that clients provide a structured user-agent string that provides a basic
	//	description of the calling library, version & platform to facilitate issue diagnosis
	//	in heterogeneous environments. The following structure is recommended to library developers:
	//
	//	User-Agent → "grpc-" Language ?("-" Variant) "/" Version ?( " ("  *(AdditionalProperty ";") ")" )
	defaultGrpcUserAgent = fmt.Sprintf("grpc-go-connect/%s (%s)", Version, runtime.Version())
)

func init() {
	for _, pair := range grpcTimeoutUnits {
		grpcTimeoutUnitLookup[pair.char] = pair.size
	}
}

type protocolGRPC struct {
	web bool
}

// NewHandler implements protocol, so it must return an interface.
func (g *protocolGRPC) NewHandler(params *protocolHandlerParams) protocolHandler {
	bare, prefix := grpcContentTypeDefault, grpcContentTypePrefix
	if g.web {
		bare, prefix = grpcWebContentTypeDefault, grpcWebContentTypePrefix
	}
	contentTypes := make(map[string]struct{})
	for _, name := range params.Codecs.Names() {
		contentTypes[canonicalizeContentType(prefix+name)] = struct{}{}
	}
	if params.Codecs.Get(codecNameProto) != nil {
		contentTypes[bare] = struct{}{}
	}
	return &grpcHandler{
		protocolHandlerParams: *params,
		web:                   g.web,
		accept:                contentTypes,
	}
}

// NewClient implements protocol, so it must return an interface.
func (g *protocolGRPC) NewClient(params *protocolClientParams) (protocolClient, error) {
	peer := newPeerFromURL(params.URL, ProtocolGRPC)
	if g.web {
		peer = newPeerFromURL(params.URL, ProtocolGRPCWeb)
	}
	return &grpcClient{
		protocolClientParams: *params,
		web:                  g.web,
		peer:                 peer,
	}, nil
}

type grpcHandler struct {
	protocolHandlerParams

	web    bool
	accept map[string]struct{}
}

func (g *grpcHandler) Methods() map[string]struct{} {
	return grpcAllowedMethods
}

func (g *grpcHandler) ContentTypes() map[string]struct{} {
	return g.accept
}

func (*grpcHandler) SetTimeout(request *http.Request) (context.Context, context.CancelFunc, error) {
	timeout, err := grpcParseTimeout(getHeaderCanonical(request.Header, grpcHeaderTimeout))
	if err != nil && !errors.Is(err, errNoTimeout) {
		// Errors here indicate that the client sent an invalid timeout header, so
		// the error text is safe to send back.
		return nil, nil, NewError(CodeInvalidArgument, err)
	} else if err != nil {
		// err wraps errNoTimeout, nothing to do.
		return request.Context(), nil, nil //nolint:nilerr
	}
	ctx, cancel := context.WithTimeout(request.Context(), timeout)
	return ctx, cancel, nil
}

func (g *grpcHandler) CanHandlePayload(request *http.Request, contentType string) bool {
	_, ok := g.accept[contentType]
	return ok
}

func (g *grpcHandler) NewConn(
	responseWriter http.ResponseWriter,
	request *http.Request,
) (handlerConnCloser, bool) {
	// We need to parse metadata before entering the interceptor stack; we'll
	// send the error to the client later on.
	requestCompression, responseCompression, failed := negotiateCompression(
		g.CompressionPools,
		getHeaderCanonical(request.Header, grpcHeaderCompression),
		getHeaderCanonical(request.Header, grpcHeaderAcceptCompression),
	)
	if failed == nil {
		failed = checkServerStreamsCanFlush(g.Spec, responseWriter)
	}

	// Write any remaining headers here:
	// (1) any writes to the stream will implicitly send the headers, so we
	// should get all of gRPC's required response headers ready.
	// (2) interceptors should be able to see these headers.
	//
	// Since we know that these header keys are already in canonical form, we can
	// skip the normalization in Header.Set.
	header := responseWriter.Header()
	header[headerContentType] = []string{getHeaderCanonical(request.Header, headerContentType)}
	header[grpcHeaderAcceptCompression] = []string{g.CompressionPools.CommaSeparatedNames()}
	if responseCompression != compressionIdentity {
		header[grpcHeaderCompression] = []string{responseCompression}
	}

	codecName := grpcCodecFromContentType(g.web, getHeaderCanonical(request.Header, headerContentType))
	codec := g.Codecs.Get(codecName) // handler.go guarantees this is not nil
	protocolName := ProtocolGRPC
	if g.web {
		protocolName = ProtocolGRPCWeb
	}
	conn := wrapHandlerConnWithCodedErrors(&grpcHandlerConn{
		spec: g.Spec,
		peer: Peer{
			Addr:     request.RemoteAddr,
			Protocol: protocolName,
		},
		web:        g.web,
		bufferPool: g.BufferPool,
		protobuf:   g.Codecs.Protobuf(), // for errors
		marshaler: grpcMarshaler{
			envelopeWriter: envelopeWriter{
				writer:           responseWriter,
				compressionPool:  g.CompressionPools.Get(responseCompression),
				codec:            codec,
				compressMinBytes: g.CompressMinBytes,
				bufferPool:       g.BufferPool,
				sendMaxBytes:     g.SendMaxBytes,
			},
		},
		responseWriter:  responseWriter,
		responseHeader:  make(http.Header),
		responseTrailer: make(http.Header),
		request:         request,
		unmarshaler: grpcUnmarshaler{
			envelopeReader: envelopeReader{
				reader:          request.Body,
				codec:           codec,
				compressionPool: g.CompressionPools.Get(requestCompression),
				bufferPool:      g.BufferPool,
				readMaxBytes:    g.ReadMaxBytes,
			},
			web: g.web,
		},
	})
	if failed != nil {
		// Negotiation failed, so we can't establish a stream.
		_ = conn.Close(failed)
		return nil, false
	}
	return conn, true
}

type grpcClient struct {
	protocolClientParams

	web  bool
	peer Peer
}

func (g *grpcClient) Peer() Peer {
	return g.peer
}

func (g *grpcClient) WriteRequestHeader(_ StreamType, header http.Header) {
	// We know these header keys are in canonical form, so we can bypass all the
	// checks in Header.Set.
	if getHeaderCanonical(header, headerUserAgent) == "" {
		header[headerUserAgent] = []string{defaultGrpcUserAgent}
	}
	header[headerContentType] = []string{grpcContentTypeFromCodecName(g.web, g.Codec.Name())}
	// gRPC handles compression on a per-message basis, so we don't want to
	// compress the whole stream. By default, http.Client will ask the server
	// to gzip the stream if we don't set Accept-Encoding.
	header["Accept-Encoding"] = []string{compressionIdentity}
	if g.CompressionName != "" && g.CompressionName != compressionIdentity {
		header[grpcHeaderCompression] = []string{g.CompressionName}
	}
	if acceptCompression := g.CompressionPools.CommaSeparatedNames(); acceptCompression != "" {
		header[grpcHeaderAcceptCompression] = []string{acceptCompression}
	}
	if !g.web {
		// The gRPC-HTTP2 specification requires this - it flushes out proxies that
		// don't support HTTP trailers.
		header["Te"] = []string{"trailers"}
	}
}

func (g *grpcClient) NewConn(
	ctx context.Context,
	spec Spec,
	header http.Header,
) streamingClientConn {
	if deadline, ok := ctx.Deadline(); ok {
		if encodedDeadline, err := grpcEncodeTimeout(time.Until(deadline)); err == nil {
			// Tests verify that the error in encodeTimeout is unreachable, so we
			// don't need to handle the error case.
			header[grpcHeaderTimeout] = []string{encodedDeadline}
		}
	}
	duplexCall := newDuplexHTTPCall(
		ctx,
		g.HTTPClient,
		g.URL,
		spec,
		header,
	)
	conn := &grpcClientConn{
		spec:             spec,
		peer:             g.Peer(),
		duplexCall:       duplexCall,
		compressionPools: g.CompressionPools,
		bufferPool:       g.BufferPool,
		protobuf:         g.Protobuf,
		marshaler: grpcMarshaler{
			envelopeWriter: envelopeWriter{
				writer:           duplexCall,
				compressionPool:  g.CompressionPools.Get(g.CompressionName),
				codec:            g.Codec,
				compressMinBytes: g.CompressMinBytes,
				bufferPool:       g.BufferPool,
				sendMaxBytes:     g.SendMaxBytes,
			},
		},
		unmarshaler: grpcUnmarshaler{
			envelopeReader: envelopeReader{
				reader:       duplexCall,
				codec:        g.Codec,
				bufferPool:   g.BufferPool,
				readMaxBytes: g.ReadMaxBytes,
			},
		},
		responseHeader:  make(http.Header),
		responseTrailer: make(http.Header),
	}
	duplexCall.SetValidateResponse(conn.validateResponse)
	if g.web {
		conn.unmarshaler.web = true
		conn.readTrailers = func(unmarshaler *grpcUnmarshaler, _ *duplexHTTPCall) http.Header {
			return unmarshaler.WebTrailer()
		}
	} else {
		conn.readTrailers = func(_ *grpcUnmarshaler, call *duplexHTTPCall) http.Header {
			// To access HTTP trailers, we need to read the body to EOF.
			_ = discard(call)
			return call.ResponseTrailer()
		}
	}
	return wrapClientConnWithCodedErrors(conn)
}

// grpcClientConn works for both gRPC and gRPC-Web.
type grpcClientConn struct {
	spec             Spec
	peer             Peer
	duplexCall       *duplexHTTPCall
	compressionPools readOnlyCompressionPools
	bufferPool       *bufferPool
	protobuf         Codec // for errors
	marshaler        grpcMarshaler
	unmarshaler      grpcUnmarshaler
	responseHeader   http.Header
	responseTrailer  http.Header
	readTrailers     func(*grpcUnmarshaler, *duplexHTTPCall) http.Header
}

func (cc *grpcClientConn) Spec() Spec {
	return cc.spec
}

func (cc *grpcClientConn) Peer() Peer {
	return cc.peer
}

func (cc *grpcClientConn) Send(msg any) error {
	if err := cc.marshaler.Marshal(msg); err != nil {
		return err
	}
	return nil // must be a literal nil: nil *Error is a non-nil error
}

func (cc *grpcClientConn) RequestHeader() http.Header {
	return cc.duplexCall.Header()
}

func (cc *grpcClientConn) CloseRequest() error {
	return cc.duplexCall.CloseWrite()
}

func (cc *grpcClientConn) Receive(msg any) error {
	cc.duplexCall.BlockUntilResponseReady()
	err := cc.unmarshaler.Unmarshal(msg)
	if err == nil {
		return nil
	}
	if getHeaderCanonical(cc.responseHeader, grpcHeaderStatus) != "" {
		// We got what gRPC calls a trailers-only response, which puts the trailing
		// metadata (including errors) into HTTP headers. validateResponse has
		// already extracted the error.
		return err
	}
	// See if the server sent an explicit error in the HTTP or gRPC-Web trailers.
	mergeHeaders(
		cc.responseTrailer,
		cc.readTrailers(&cc.unmarshaler, cc.duplexCall),
	)
	serverErr := grpcErrorFromTrailer(cc.protobuf, cc.responseTrailer)
	if serverErr != nil && (errors.Is(err, io.EOF) || !errors.Is(serverErr, errTrailersWithoutGRPCStatus)) {
		// We've either:
		//   - Cleanly read until the end of the response body and *not* received
		//   gRPC status trailers, which is a protocol error, or
		//   - Received an explicit error from the server.
		//
		// This is expected from a protocol perspective, but receiving trailers
		// means that we're _not_ getting a message. For users to realize that
		// the stream has ended, Receive must return an error.
		serverErr.meta = cc.responseHeader.Clone()
		mergeHeaders(serverErr.meta, cc.responseTrailer)
		cc.duplexCall.SetError(serverErr)
		return serverErr
	}
	// This was probably an error converting the bytes to a message or an error
	// reading from the network. We're going to return it to the
	// user, but we also want to setResponseError so Send errors out.
	cc.duplexCall.SetError(err)
	return err
}

func (cc *grpcClientConn) ResponseHeader() http.Header {
	cc.duplexCall.BlockUntilResponseReady()
	return cc.responseHeader
}

func (cc *grpcClientConn) ResponseTrailer() http.Header {
	cc.duplexCall.BlockUntilResponseReady()
	return cc.responseTrailer
}

func (cc *grpcClientConn) CloseResponse() error {
	return cc.duplexCall.CloseRead()
}

func (cc *grpcClientConn) onRequestSend(fn func(*http.Request)) {
	cc.duplexCall.onRequestSend = fn
}

func (cc *grpcClientConn) validateResponse(response *http.Response) *Error {
	if err := grpcValidateResponse(
		response,
		cc.responseHeader,
		cc.responseTrailer,
		cc.compressionPools,
		cc.protobuf,
	); err != nil {
		return err
	}
	compression := getHeaderCanonical(response.Header, grpcHeaderCompression)
	cc.unmarshaler.envelopeReader.compressionPool = cc.compressionPools.Get(compression)
	return nil
}

type grpcHandlerConn struct {
	spec            Spec
	peer            Peer
	web             bool
	bufferPool      *bufferPool
	protobuf        Codec // for errors
	marshaler       grpcMarshaler
	responseWriter  http.ResponseWriter
	responseHeader  http.Header
	responseTrailer http.Header
	wroteToBody     bool
	request         *http.Request
	unmarshaler     grpcUnmarshaler
}

func (hc *grpcHandlerConn) Spec() Spec {
	return hc.spec
}

func (hc *grpcHandlerConn) Peer() Peer {
	return hc.peer
}

func (hc *grpcHandlerConn) Receive(msg any) error {
	if err := hc.unmarshaler.Unmarshal(msg); err != nil {
		return err // already coded
	}
	return nil // must be a literal nil: nil *Error is a non-nil error
}

func (hc *grpcHandlerConn) RequestHeader() http.Header {
	return hc.request.Header
}

func (hc *grpcHandlerConn) Send(msg any) error {
	defer flushResponseWriter(hc.responseWriter)
	if !hc.wroteToBody {
		mergeHeaders(hc.responseWriter.Header(), hc.responseHeader)
		hc.wroteToBody = true
	}
	if err := hc.marshaler.Marshal(msg); err != nil {
		return err
	}
	return nil // must be a literal nil: nil *Error is a non-nil error
}

func (hc *grpcHandlerConn) ResponseHeader() http.Header {
	return hc.responseHeader
}

func (hc *grpcHandlerConn) ResponseTrailer() http.Header {
	return hc.responseTrailer
}

func (hc *grpcHandlerConn) Close(err error) (retErr error) {
	defer func() {
		// We don't want to copy unread portions of the body to /dev/null here: if
		// the client hasn't closed the request body, we'll block until the server
		// timeout kicks in. This could happen because the client is malicious, but
		// a well-intentioned client may just not expect the server to be returning
		// an error for a streaming RPC. Better to accept that we can't always reuse
		// TCP connections.
		closeErr := hc.request.Body.Close()
		if retErr == nil {
			retErr = closeErr
		}
	}()
	defer flushResponseWriter(hc.responseWriter)
	// If we haven't written the headers yet, do so.
	if !hc.wroteToBody {
		mergeHeaders(hc.responseWriter.Header(), hc.responseHeader)
	}
	// gRPC always sends the error's code, message, details, and metadata as
	// trailing metadata. The Connect protocol doesn't do this, so we don't want
	// to mutate the trailers map that the user sees.
	mergedTrailers := make(
		http.Header,
		len(hc.responseTrailer)+2, // always make space for status & message
	)
	mergeHeaders(mergedTrailers, hc.responseTrailer)
	grpcErrorToTrailer(mergedTrailers, hc.protobuf, err)
	if hc.web && !hc.wroteToBody {
		// We're using gRPC-Web and we haven't yet written to the body. Since we're
		// not sending any response messages, the gRPC specification calls this a
		// "trailers-only" response. Under those circumstances, the gRPC-Web spec
		// says that implementations _may_ send trailing metadata as HTTP headers
		// instead. Envoy is the canonical implementation of the gRPC-Web protocol,
		// so we emulate Envoy's behavior and put the trailing metadata in the HTTP
		// headers.
		mergeHeaders(hc.responseWriter.Header(), mergedTrailers)
		return nil
	}
	if hc.web {
		// We're using gRPC-Web and we've already sent the headers, so we write
		// trailing metadata to the HTTP body.
		if err := hc.marshaler.MarshalWebTrailers(mergedTrailers); err != nil {
			return err
		}
		return nil // must be a literal nil: nil *Error is a non-nil error
	}
	// We're using standard gRPC. Even if we haven't written to the body and
	// we're sending a "trailers-only" response, we must send trailing metadata
	// as HTTP trailers. (If we had frame-level control of the HTTP/2 layer, we
	// could send trailers-only responses as a single HEADER frame and no DATA
	// frames, but net/http doesn't expose APIs that low-level.)
	if !hc.wroteToBody {
		// This block works around a bug in x/net/http2. Until Go 1.20, trailers
		// written using http.TrailerPrefix were only sent if either (1) there's
		// data in the body, or (2) the innermost http.ResponseWriter is flushed.
		// To ensure that we always send a valid gRPC response, even if the user
		// has wrapped the response writer in net/http middleware that doesn't
		// implement http.Flusher, we must pre-declare our HTTP trailers. We can
		// remove this when Go 1.21 ships and we drop support for Go 1.19.
		for key := range mergedTrailers {
			addHeaderCanonical(hc.responseWriter.Header(), headerTrailer, key)
		}
		hc.responseWriter.WriteHeader(http.StatusOK)
		for key, values := range mergedTrailers {
			for _, value := range values {
				// These are potentially user-supplied, so we can't assume they're in
				// canonical form. Don't use addHeaderCanonical.
				hc.responseWriter.Header().Add(key, value)
			}
		}
		return nil
	}
	// In net/http's ResponseWriter API, we send HTTP trailers by writing to the
	// headers map with a special prefix. This prefixing is an implementation
	// detail, so we should hide it and _not_ mutate the user-visible headers.
	//
	// Note that this is _very_ finicky and difficult to test with net/http,
	// since correctness depends on low-level framing details. Breaking this
	// logic breaks Envoy's gRPC-Web translation.
	for key, values := range mergedTrailers {
		for _, value := range values {
			// These are potentially user-supplied, so we can't assume they're in
			// canonical form. Don't use addHeaderCanonical.
			hc.responseWriter.Header().Add(http.TrailerPrefix+key, value)
		}
	}
	return nil
}

type grpcMarshaler struct {
	envelopeWriter
}

func (m *grpcMarshaler) MarshalWebTrailers(trailer http.Header) *Error {
	raw := m.envelopeWriter.bufferPool.Get()
	defer m.envelopeWriter.bufferPool.Put(raw)
	for key, values := range trailer {
		// Per the Go specification, keys inserted during iteration may be produced
		// later in the iteration or may be skipped. For safety, avoid mutating the
		// map if the key is already lower-cased.
		lower := strings.ToLower(key)
		if key == lower {
			continue
		}
		delete(trailer, key)
		trailer[lower] = values
	}
	if err := trailer.Write(raw); err != nil {
		return errorf(CodeInternal, "format trailers: %w", err)
	}
	return m.Write(&envelope{
		Data:  raw,
		Flags: grpcFlagEnvelopeTrailer,
	})
}

type grpcUnmarshaler struct {
	envelopeReader envelopeReader
	web            bool
	webTrailer     http.Header
}

func (u *grpcUnmarshaler) Unmarshal(message any) *Error {
	err := u.envelopeReader.Unmarshal(message)
	if err == nil {
		return nil
	}
	if !errors.Is(err, errSpecialEnvelope) {
		return err
	}
	env := u.envelopeReader.last
	if !u.web || !env.IsSet(grpcFlagEnvelopeTrailer) {
		return errorf(CodeInternal, "protocol error: invalid envelope flags %d", env.Flags)
	}

	// Per the gRPC-Web specification, trailers should be encoded as an HTTP/1
	// headers block _without_ the terminating newline. To make the headers
	// parseable by net/textproto, we need to add the newline.
	if err := env.Data.WriteByte('\n'); err != nil {
		return errorf(CodeInternal, "unmarshal web trailers: %w", err)
	}
	bufferedReader := bufio.NewReader(env.Data)
	mimeReader := textproto.NewReader(bufferedReader)
	mimeHeader, mimeErr := mimeReader.ReadMIMEHeader()
	if mimeErr != nil {
		return errorf(
			CodeInternal,
			"gRPC-Web protocol error: trailers invalid: %w",
			mimeErr,
		)
	}
	u.webTrailer = http.Header(mimeHeader)
	return errSpecialEnvelope
}

func (u *grpcUnmarshaler) WebTrailer() http.Header {
	return u.webTrailer
}

func grpcValidateResponse(
	response *http.Response,
	header, trailer http.Header,
	availableCompressors readOnlyCompressionPools,
	protobuf Codec,
) *Error {
	if response.StatusCode != http.StatusOK {
		return errorf(grpcHTTPToCode(response.StatusCode), "HTTP status %v", response.Status)
	}
	if compression := getHeaderCanonical(response.Header, grpcHeaderCompression); compression != "" &&
		compression != compressionIdentity &&
		!availableCompressors.Contains(compression) {
		// Per https://github.com/grpc/grpc/blob/master/doc/compression.md, we
		// should return CodeInternal and specify acceptable compression(s) (in
		// addition to setting the Grpc-Accept-Encoding header).
		return errorf(
			CodeInternal,
			"unknown encoding %q: accepted encodings are %v",
			compression,
			availableCompressors.CommaSeparatedNames(),
		)
	}
	// When there's no body, gRPC and gRPC-Web servers may send error information
	// in the HTTP headers.
	if err := grpcErrorFromTrailer(
		protobuf,
		response.Header,
	); err != nil && !errors.Is(err, errTrailersWithoutGRPCStatus) {
		// Per the specification, only the HTTP status code and Content-Type should
		// be treated as headers. The rest should be treated as trailing metadata.
		if contentType := getHeaderCanonical(response.Header, headerContentType); contentType != "" {
			setHeaderCanonical(header, headerContentType, contentType)
		}
		mergeHeaders(trailer, response.Header)
		delHeaderCanonical(trailer, headerContentType)
		// Also set the error metadata
		err.meta = header.Clone()
		mergeHeaders(err.meta, trailer)
		return err
	}
	// The response is valid, so we should expose the headers.
	mergeHeaders(header, response.Header)
	return nil
}

func grpcHTTPToCode(httpCode int) Code {
	// https://github.com/grpc/grpc/blob/master/doc/http-grpc-status-mapping.md
	// Note that this is not just the inverse of the gRPC-to-HTTP mapping.
	switch httpCode {
	case 400:
		return CodeInternal
	case 401:
		return CodeUnauthenticated
	case 403:
		return CodePermissionDenied
	case 404:
		return CodeUnimplemented
	case 429:
		return CodeUnavailable
	case 502, 503, 504:
		return CodeUnavailable
	default:
		return CodeUnknown
	}
}

// The gRPC wire protocol specifies that errors should be serialized using the
// binary Protobuf format, even if the messages in the request/response stream
// use a different codec. Consequently, this function needs a Protobuf codec to
// unmarshal error information in the headers.
func grpcErrorFromTrailer(protobuf Codec, trailer http.Header) *Error {
	codeHeader := getHeaderCanonical(trailer, grpcHeaderStatus)
	if codeHeader == "" {
		return NewError(CodeInternal, errTrailersWithoutGRPCStatus)
	}
	if codeHeader == "0" {
		return nil
	}

	code, err := strconv.ParseUint(codeHeader, 10 /* base */, 32 /* bitsize */)
	if err != nil {
		return errorf(CodeInternal, "gRPC protocol error: invalid error code %q", codeHeader)
	}
	message := grpcPercentDecode(getHeaderCanonical(trailer, grpcHeaderMessage))
	retErr := NewWireError(Code(code), errors.New(message))

	detailsBinaryEncoded := getHeaderCanonical(trailer, grpcHeaderDetails)
	if len(detailsBinaryEncoded) > 0 {
		detailsBinary, err := DecodeBinaryHeader(detailsBinaryEncoded)
		if err != nil {
			return errorf(CodeInternal, "server returned invalid grpc-status-details-bin trailer: %w", err)
		}
		var status statusv1.Status
		if err := protobuf.Unmarshal(detailsBinary, &status); err != nil {
			return errorf(CodeInternal, "server returned invalid protobuf for error details: %w", err)
		}
		for _, d := range status.Details {
			retErr.details = append(retErr.details, &ErrorDetail{pb: d})
		}
		// Prefer the Protobuf-encoded data to the headers (grpc-go does this too).
		retErr.code = Code(status.Code)
		retErr.err = errors.New(status.Message)
	}

	return retErr
}

func grpcParseTimeout(timeout string) (time.Duration, error) {
	if timeout == "" {
		return 0, errNoTimeout
	}
	unit, ok := grpcTimeoutUnitLookup[timeout[len(timeout)-1]]
	if !ok {
		return 0, fmt.Errorf("gRPC protocol error: timeout %q has invalid unit", timeout)
	}
	num, err := strconv.ParseInt(timeout[:len(timeout)-1], 10 /* base */, 64 /* bitsize */)
	if err != nil || num < 0 {
		return 0, fmt.Errorf("gRPC protocol error: invalid timeout %q", timeout)
	}
	if num > 99999999 { // timeout must be ASCII string of at most 8 digits
		return 0, fmt.Errorf("gRPC protocol error: timeout %q is too long", timeout)
	}
	if unit == time.Hour && num > grpcTimeoutMaxHours {
		// Timeout is effectively unbounded, so ignore it. The grpc-go
		// implementation does the same thing.
		return 0, errNoTimeout
	}
	return time.Duration(num) * unit, nil
}

func grpcEncodeTimeout(timeout time.Duration) (string, error) {
	if timeout <= 0 {
		return "0n", nil
	}
	for _, pair := range grpcTimeoutUnits {
		digits := strconv.FormatInt(int64(timeout/pair.size), 10 /* base */)
		if len(digits) < grpcMaxTimeoutChars {
			return digits + string(pair.char), nil
		}
	}
	// The max time.Duration is smaller than the maximum expressible gRPC
	// timeout, so we can't reach this case.
	return "", errNoTimeout
}

func grpcCodecFromContentType(web bool, contentType string) string {
	if (!web && contentType == grpcContentTypeDefault) || (web && contentType == grpcWebContentTypeDefault) {
		// implicitly protobuf
		return codecNameProto
	}
	prefix := grpcContentTypePrefix
	if web {
		prefix = grpcWebContentTypePrefix
	}
	return strings.TrimPrefix(contentType, prefix)
}

func grpcContentTypeFromCodecName(web bool, name string) string {
	if web {
		return grpcWebContentTypePrefix + name
	}
	return grpcContentTypePrefix + name
}

func grpcErrorToTrailer(trailer http.Header, protobuf Codec, err error) {
	if err == nil {
		setHeaderCanonical(trailer, grpcHeaderStatus, "0") // zero is the gRPC OK status
		setHeaderCanonical(trailer, grpcHeaderMessage, "")
		return
	}
	status := grpcStatusFromError(err)
	code := strconv.Itoa(int(status.Code))
	bin, binErr := protobuf.Marshal(status)
	if binErr != nil {
		setHeaderCanonical(
			trailer,
			grpcHeaderStatus,
			strconv.FormatInt(int64(CodeInternal), 10 /* base */),
		)
		setHeaderCanonical(
			trailer,
			grpcHeaderMessage,
			grpcPercentEncode(
				fmt.Sprintf("marshal protobuf status: %v", binErr),
			),
		)
		return
	}
	if connectErr, ok := asError(err); ok {
		mergeHeaders(trailer, connectErr.meta)
	}
	setHeaderCanonical(trailer, grpcHeaderStatus, code)
	setHeaderCanonical(trailer, grpcHeaderMessage, grpcPercentEncode(status.Message))
	setHeaderCanonical(trailer, grpcHeaderDetails, EncodeBinaryHeader(bin))
}

func grpcStatusFromError(err error) *statusv1.Status {
	status := &statusv1.Status{
		Code:    int32(CodeUnknown),
		Message: err.Error(),
	}
	if connectErr, ok := asError(err); ok {
		status.Code = int32(connectErr.Code())
		status.Message = connectErr.Message()
		status.Details = connectErr.detailsAsAny()
	}
	return status
}

// grpcPercentEncode follows RFC 3986 Section 2.1 and the gRPC HTTP/2 spec.
// It's a variant of URL-encoding with fewer reserved characters. It's intended
// to take UTF-8 encoded text and escape non-ASCII bytes so that they're valid
// HTTP/1 headers, while still maximizing readability of the data on the wire.
//
// The grpc-message trailer (used for human-readable error messages) should be
// percent-encoded.
//
// References:
//
//	https://github.com/grpc/grpc/blob/master/doc/PROTOCOL-HTTP2.md#responses
//	https://datatracker.ietf.org/doc/html/rfc3986#section-2.1
func grpcPercentEncode(msg string) string {
	for i := 0; i < len(msg); i++ {
		// Characters that need to be escaped are defined in gRPC's HTTP/2 spec.
		// They're different from the generic set defined in RFC 3986.
		if c := msg[i]; c < ' ' || c > '~' || c == '%' {
			return grpcPercentEncodeSlow(msg, i)
		}
	}
	return msg
}

// msg needs some percent-escaping. Bytes before offset don't require
// percent-encoding, so they can be copied to the output as-is.
func grpcPercentEncodeSlow(msg string, offset int) string {
	var out strings.Builder
	out.Grow(2 * len(msg))
	out.WriteString(msg[:offset])
	for i := offset; i < len(msg); i++ {
		c := msg[i]
		if c < ' ' || c > '~' || c == '%' {
			fmt.Fprintf(&out, "%%%02X", c)
			continue
		}
		out.WriteByte(c)
	}
	return out.String()
}

func grpcPercentDecode(encoded string) string {
	for i := 0; i < len(encoded); i++ {
		if c := encoded[i]; c == '%' && i+2 < len(encoded) {
			return grpcPercentDecodeSlow(encoded, i)
		}
	}
	return encoded
}

// Similar to percentEncodeSlow: encoded is percent-encoded, and needs to be
// decoded byte-by-byte starting at offset.
func grpcPercentDecodeSlow(encoded string, offset int) string {
	var out strings.Builder
	out.Grow(len(encoded))
	out.WriteString(encoded[:offset])
	for i := offset; i < len(encoded); i++ {
		c := encoded[i]
		if c != '%' || i+2 >= len(encoded) {
			out.WriteByte(c)
			continue
		}
		parsed, err := strconv.ParseUint(encoded[i+1:i+3], 16 /* hex */, 8 /* bitsize */)
		if err != nil {
			out.WriteRune(utf8.RuneError)
		} else {
			out.WriteByte(byte(parsed))
		}
		i += 2
	}
	return out.String()
}
