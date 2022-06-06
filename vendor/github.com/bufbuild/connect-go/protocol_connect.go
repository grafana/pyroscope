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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	errorv1 "github.com/bufbuild/connect-go/internal/gen/connect/error/v1"
)

const (
	connectUnaryHeaderCompression           = "Content-Encoding"
	connectUnaryHeaderAcceptCompression     = "Accept-Encoding"
	connectUnaryTrailerPrefix               = "Trailer-"
	connectStreamingHeaderCompression       = "Connect-Encoding"
	connectStreamingHeaderAcceptCompression = "Connect-Accept-Encoding"
	connectHeaderTimeout                    = "Connect-Timeout-Ms"

	connectFlagEnvelopeEndStream = 0b00000010

	connectUnaryContentTypePrefix     = "application/"
	connectUnaryContentTypeJSON       = connectUnaryContentTypePrefix + "json"
	connectStreamingContentTypePrefix = "application/connect+"
)

type protocolConnect struct{}

// NewHandler implements protocol, so it must return an interface.
func (*protocolConnect) NewHandler(params *protocolHandlerParams) protocolHandler {
	contentTypes := make(map[string]struct{})
	for _, name := range params.Codecs.Names() {
		if params.Spec.StreamType == StreamTypeUnary {
			contentTypes[connectUnaryContentTypePrefix+name] = struct{}{}
			continue
		}
		contentTypes[connectStreamingContentTypePrefix+name] = struct{}{}
	}
	return &connectHandler{
		protocolHandlerParams: *params,
		accept:                contentTypes,
	}
}

// NewClient implements protocol, so it must return an interface.
func (*protocolConnect) NewClient(params *protocolClientParams) (protocolClient, error) {
	if err := validateRequestURL(params.URL); err != nil {
		return nil, err
	}
	return &connectClient{protocolClientParams: *params}, nil
}

type connectHandler struct {
	protocolHandlerParams

	accept map[string]struct{}
}

func (h *connectHandler) ContentTypes() map[string]struct{} {
	return h.accept
}

func (*connectHandler) SetTimeout(request *http.Request) (context.Context, context.CancelFunc, error) {
	timeout := request.Header.Get(connectHeaderTimeout)
	if timeout == "" {
		return request.Context(), nil, nil
	}
	if len(timeout) > 10 {
		return nil, nil, errorf(CodeInvalidArgument, "parse timeout: %q has >10 digits", timeout)
	}
	millis, err := strconv.ParseInt(timeout, 10 /* base */, 64 /* bitsize */)
	if err != nil {
		return nil, nil, errorf(CodeInvalidArgument, "parse timeout: %w", err)
	}
	ctx, cancel := context.WithTimeout(
		request.Context(),
		time.Duration(millis)*time.Millisecond,
	)
	return ctx, cancel, nil
}

func (h *connectHandler) NewStream(
	responseWriter http.ResponseWriter,
	request *http.Request,
) (Sender, Receiver, error) {
	// We need to parse metadata before entering the interceptor stack; we'll
	// send the error to the client later on.
	var contentEncoding, acceptEncoding string
	if h.Spec.StreamType == StreamTypeUnary {
		contentEncoding = request.Header.Get(connectUnaryHeaderCompression)
		acceptEncoding = request.Header.Get(connectUnaryHeaderAcceptCompression)
	} else {
		contentEncoding = request.Header.Get(connectStreamingHeaderCompression)
		acceptEncoding = request.Header.Get(connectStreamingHeaderAcceptCompression)
	}
	requestCompression, responseCompression, failed := negotiateCompression(
		h.CompressionPools,
		contentEncoding,
		acceptEncoding,
	)

	// Write any remaining headers here:
	// (1) any writes to the stream will implicitly send the headers, so we
	// should get all of gRPC's required response headers ready.
	// (2) interceptors should be able to see these headers.
	//
	// Since we know that these header keys are already in canonical form, we can
	// skip the normalization in Header.Set.
	header := responseWriter.Header()
	header[headerContentType] = []string{request.Header.Get(headerContentType)}
	acceptCompressionHeader := connectUnaryHeaderAcceptCompression
	if h.Spec.StreamType != StreamTypeUnary {
		acceptCompressionHeader = connectStreamingHeaderAcceptCompression
		// We only write the request encoding header here for streaming calls,
		// since the streaming envelope lets us choose whether to compress each
		// message individually. For unary, we won't know whether we're compressing
		// the request until we see how large the payload is.
		if responseCompression != compressionIdentity {
			header[connectStreamingHeaderCompression] = []string{responseCompression}
		}
	}
	header[acceptCompressionHeader] = []string{h.CompressionPools.CommaSeparatedNames()}

	codecName := connectCodecFromContentType(
		h.Spec.StreamType,
		request.Header.Get(headerContentType),
	)
	codec := h.Codecs.Get(codecName) // handler.go guarantees this is not nil
	var sender Sender = &connectUnaryHandlerSender{
		spec:           h.Spec,
		responseWriter: responseWriter,
		trailer:        make(http.Header),
		marshaler: connectUnaryMarshaler{
			writer:           responseWriter,
			codec:            codec,
			compressMinBytes: h.CompressMinBytes,
			compressionName:  responseCompression,
			compressionPool:  h.CompressionPools.Get(responseCompression),
			bufferPool:       h.BufferPool,
			header:           responseWriter.Header(),
		},
	}
	var receiver Receiver = &connectUnaryHandlerReceiver{
		spec:    h.Spec,
		request: request,
		unmarshaler: connectUnaryUnmarshaler{
			reader:          request.Body,
			codec:           codec,
			compressionPool: h.CompressionPools.Get(requestCompression),
			bufferPool:      h.BufferPool,
		},
	}
	if h.Spec.StreamType != StreamTypeUnary {
		sender = &connectStreamingHandlerSender{
			spec:    h.Spec,
			writer:  responseWriter,
			trailer: make(http.Header),
			marshaler: connectStreamingMarshaler{
				envelopeWriter: envelopeWriter{
					writer:           responseWriter,
					codec:            codec,
					compressMinBytes: h.CompressMinBytes,
					compressionPool:  h.CompressionPools.Get(responseCompression),
					bufferPool:       h.BufferPool,
				},
			},
		}
		receiver = &connectStreamingHandlerReceiver{
			spec:    h.Spec,
			request: request,
			unmarshaler: connectStreamingUnmarshaler{
				envelopeReader: envelopeReader{
					reader:          request.Body,
					codec:           codec,
					compressionPool: h.CompressionPools.Get(requestCompression),
					bufferPool:      h.BufferPool,
				},
			},
		}
	}
	sender, receiver = wrapHandlerStreamWithCodedErrors(sender, receiver)
	// We can't return failed as-is: a nil *Error is non-nil when returned as an
	// error interface.
	if failed != nil {
		// Negotiation failed, so we can't establish a stream. To make the
		// request's HTTP trailers visible to interceptors, we should try to read
		// the body to EOF.
		_ = discard(request.Body)
		return sender, receiver, failed
	}
	return sender, receiver, nil
}

type connectClient struct {
	protocolClientParams
}

func (c *connectClient) WriteRequestHeader(streamType StreamType, header http.Header) {
	// We know these header keys are in canonical form, so we can bypass all the
	// checks in Header.Set.
	header[headerUserAgent] = []string{connectUserAgent()}
	header[headerContentType] = []string{
		connectContentTypeFromCodecName(streamType, c.Codec.Name()),
	}
	acceptCompressionHeader := connectUnaryHeaderAcceptCompression
	if streamType != StreamTypeUnary {
		// If we don't set Accept-Encoding, by default http.Client will ask the
		// server to compress the whole stream. Since we're already compressing
		// each message, this is a waste.
		header[connectUnaryHeaderAcceptCompression] = []string{compressionIdentity}
		acceptCompressionHeader = connectStreamingHeaderAcceptCompression
		// We only write the request encoding header here for streaming calls,
		// since the streaming envelope lets us choose whether to compress each
		// message individually. For unary, we won't know whether we're compressing
		// the request until we see how large the payload is.
		if c.CompressionName != "" && c.CompressionName != compressionIdentity {
			header[connectStreamingHeaderCompression] = []string{c.CompressionName}
		}
	}
	if acceptCompression := c.CompressionPools.CommaSeparatedNames(); acceptCompression != "" {
		header[acceptCompressionHeader] = []string{acceptCompression}
	}
}

func (c *connectClient) NewStream(
	ctx context.Context,
	spec Spec,
	header http.Header,
) (Sender, Receiver) {
	if deadline, ok := ctx.Deadline(); ok {
		millis := int64(time.Until(deadline) / time.Millisecond)
		if millis > 0 {
			encoded := strconv.FormatInt(millis, 10 /* base */)
			if len(encoded) <= 10 {
				header[connectHeaderTimeout] = []string{encoded}
			} // else effectively unbounded
		}
	}
	duplexCall := newDuplexHTTPCall(ctx, c.HTTPClient, c.URL, spec, header)
	var sender Sender
	var receiver Receiver
	if spec.StreamType == StreamTypeUnary {
		unarySender := &connectClientSender{
			spec:       spec,
			duplexCall: duplexCall,
			marshaler: &connectUnaryMarshaler{
				writer:           duplexCall,
				codec:            c.Codec,
				compressMinBytes: c.CompressMinBytes,
				compressionName:  c.CompressionName,
				compressionPool:  c.CompressionPools.Get(c.CompressionName),
				bufferPool:       c.BufferPool,
				header:           duplexCall.Header(),
			},
		}
		sender = unarySender
		unaryReceiver := &connectUnaryClientReceiver{
			spec:             spec,
			duplexCall:       duplexCall,
			compressionPools: c.CompressionPools,
			bufferPool:       c.BufferPool,
			header:           make(http.Header),
			trailer:          make(http.Header),
			unmarshaler: connectUnaryUnmarshaler{
				reader:     duplexCall,
				codec:      c.Codec,
				bufferPool: c.BufferPool,
			},
		}
		receiver = unaryReceiver
		duplexCall.SetValidateResponse(unaryReceiver.validateResponse)
	} else {
		streamingSender := &connectClientSender{
			spec:       spec,
			duplexCall: duplexCall,
			marshaler: &connectStreamingMarshaler{
				envelopeWriter: envelopeWriter{
					writer:           duplexCall,
					codec:            c.Codec,
					compressMinBytes: c.CompressMinBytes,
					compressionPool:  c.CompressionPools.Get(c.CompressionName),
					bufferPool:       c.BufferPool,
				},
			},
		}
		sender = streamingSender
		streamingReceiver := &connectStreamingClientReceiver{
			spec:             spec,
			bufferPool:       c.BufferPool,
			compressionPools: c.CompressionPools,
			codec:            c.Codec,
			header:           make(http.Header),
			trailer:          make(http.Header),
			duplexCall:       duplexCall,
			unmarshaler: connectStreamingUnmarshaler{
				envelopeReader: envelopeReader{
					reader:     duplexCall,
					codec:      c.Codec,
					bufferPool: c.BufferPool,
				},
			},
		}
		receiver = streamingReceiver
		duplexCall.SetValidateResponse(streamingReceiver.validateResponse)
	}
	return wrapClientStreamWithCodedErrors(sender, receiver)
}

// connectClientSender works equally well for unary and streaming, since it can
// use either marshaler.
type connectClientSender struct {
	spec       Spec
	duplexCall *duplexHTTPCall
	marshaler  interface{ Marshal(any) *Error }
}

func (s *connectClientSender) Spec() Spec {
	return s.spec
}

func (s *connectClientSender) Header() http.Header {
	return s.duplexCall.Header()
}

func (s *connectClientSender) Trailer() (http.Header, bool) {
	return nil, false
}

func (s *connectClientSender) Send(message any) error {
	if err := s.marshaler.Marshal(message); err != nil {
		return err
	}
	return nil // must be a literal nil: nil *Error is a non-nil error
}

func (s *connectClientSender) Close(err error) error {
	return s.duplexCall.CloseWrite()
}

type connectStreamingClientReceiver struct {
	spec             Spec
	bufferPool       *bufferPool
	compressionPools readOnlyCompressionPools
	codec            Codec
	header           http.Header
	trailer          http.Header
	duplexCall       *duplexHTTPCall
	unmarshaler      connectStreamingUnmarshaler
}

func (r *connectStreamingClientReceiver) Spec() Spec {
	return r.spec
}

func (r *connectStreamingClientReceiver) Header() http.Header {
	r.duplexCall.BlockUntilResponseReady()
	return r.header
}

func (r *connectStreamingClientReceiver) Trailer() (http.Header, bool) {
	r.duplexCall.BlockUntilResponseReady()
	return r.trailer, true
}

func (r *connectStreamingClientReceiver) Receive(message any) error {
	r.duplexCall.BlockUntilResponseReady()
	err := r.unmarshaler.Unmarshal(message)
	if err == nil {
		return nil
	}
	// See if the server sent an explicit error in the end-of-stream message.
	mergeHeaders(r.trailer, r.unmarshaler.Trailer())
	if serverErr := r.unmarshaler.EndStreamError(); serverErr != nil {
		// This is expected from a protocol perspective, but receiving an
		// end-of-stream message means that we're _not_ getting a regular message.
		// For users to realize that the stream has ended, Receive must return an
		// error.
		serverErr.meta = r.header.Clone()
		mergeHeaders(serverErr.meta, r.trailer)
		r.duplexCall.SetError(serverErr)
		return serverErr
	}
	// There's no error in the trailers, so this was probably an error
	// converting the bytes to a message, an error reading from the network, or
	// just an EOF. We're going to return it to the user, but we also want to
	// setResponseError so Send errors out.
	r.duplexCall.SetError(err)
	return err
}

func (r *connectStreamingClientReceiver) Close() error {
	return r.duplexCall.CloseRead()
}

// validateResponse is called by duplexHTTPCall in a separate goroutine.
func (r *connectStreamingClientReceiver) validateResponse(response *http.Response) *Error {
	if response.StatusCode != http.StatusOK {
		return errorf(connectHTTPToCode(response.StatusCode), "HTTP status %v", response.Status)
	}
	compression := response.Header.Get(connectStreamingHeaderCompression)
	if compression != "" &&
		compression != compressionIdentity &&
		!r.compressionPools.Contains(compression) {
		return errorf(
			CodeInternal,
			"unknown encoding %q: accepted encodings are %v",
			compression,
			r.compressionPools.CommaSeparatedNames(),
		)
	}
	r.unmarshaler.compressionPool = r.compressionPools.Get(compression)
	mergeHeaders(r.header, response.Header)
	return nil
}

type connectStreamingHandlerSender struct {
	spec      Spec
	marshaler connectStreamingMarshaler
	writer    http.ResponseWriter
	trailer   http.Header
}

func (s *connectStreamingHandlerSender) Send(message any) error {
	defer flushResponseWriter(s.writer)
	if err := s.marshaler.Marshal(message); err != nil {
		return err
	}
	return nil // must be a literal nil: nil *Error is a non-nil error
}

func (s *connectStreamingHandlerSender) Close(err error) error {
	defer flushResponseWriter(s.writer)
	if err := s.marshaler.MarshalEndStream(err, s.trailer); err != nil {
		return err
	}
	return nil // must be a literal nil: nil *Error is a non-nil error
}

func (s *connectStreamingHandlerSender) Spec() Spec {
	return s.spec
}

func (s *connectStreamingHandlerSender) Header() http.Header {
	return s.writer.Header()
}

func (s *connectStreamingHandlerSender) Trailer() (http.Header, bool) {
	return s.trailer, true
}

type connectStreamingHandlerReceiver struct {
	spec        Spec
	unmarshaler connectStreamingUnmarshaler
	request     *http.Request
}

func (r *connectStreamingHandlerReceiver) Receive(message any) error {
	if err := r.unmarshaler.Unmarshal(message); err != nil {
		// Clients may not send end-of-stream metadata, so we don't need to handle
		// errSpecialEnvelope.
		return err
	}
	return nil // must be a literal nil: nil *Error is a non-nil error
}

func (r *connectStreamingHandlerReceiver) Close() error {
	// We don't want to copy unread portions of the body to /dev/null here: if
	// the client hasn't closed the request body, we'll block until the server
	// timeout kicks in. This could happen because the client is malicious, but
	// a well-intentioned client may just not expect the server to be returning
	// an error for a streaming RPC. Better to accept that we can't always reuse
	// TCP connections.
	if err := r.request.Body.Close(); err != nil {
		if connectErr, ok := asError(err); ok {
			return connectErr
		}
		return NewError(CodeUnknown, err)
	}
	return nil
}

func (r *connectStreamingHandlerReceiver) Spec() Spec {
	return r.spec
}

func (r *connectStreamingHandlerReceiver) Header() http.Header {
	return r.request.Header
}

func (r *connectStreamingHandlerReceiver) Trailer() (http.Header, bool) {
	return nil, false
}

type connectUnaryClientReceiver struct {
	spec             Spec
	duplexCall       *duplexHTTPCall
	compressionPools readOnlyCompressionPools
	bufferPool       *bufferPool

	header      http.Header
	trailer     http.Header
	unmarshaler connectUnaryUnmarshaler
}

func (r *connectUnaryClientReceiver) Spec() Spec {
	return r.spec
}

func (r *connectUnaryClientReceiver) Header() http.Header {
	r.duplexCall.BlockUntilResponseReady()
	return r.header
}

func (r *connectUnaryClientReceiver) Trailer() (http.Header, bool) {
	r.duplexCall.BlockUntilResponseReady()
	return r.trailer, true
}

func (r *connectUnaryClientReceiver) Receive(message any) error {
	r.duplexCall.BlockUntilResponseReady()
	if err := r.unmarshaler.Unmarshal(message); err != nil {
		return err
	}
	return nil
}

func (r *connectUnaryClientReceiver) Close() error {
	return r.duplexCall.CloseRead()
}

func (r *connectUnaryClientReceiver) validateResponse(response *http.Response) *Error {
	for k, v := range response.Header {
		if !strings.HasPrefix(k, connectUnaryTrailerPrefix) {
			r.header[k] = v
			continue
		}
		r.trailer[strings.TrimPrefix(k, connectUnaryTrailerPrefix)] = v
	}
	compression := response.Header.Get(connectUnaryHeaderCompression)
	if compression != "" &&
		compression != compressionIdentity &&
		!r.compressionPools.Contains(compression) {
		return errorf(
			CodeInternal,
			"unknown encoding %q: accepted encodings are %v",
			compression,
			r.compressionPools.CommaSeparatedNames(),
		)
	}
	if response.StatusCode != http.StatusOK {
		unmarshaler := connectUnaryUnmarshaler{
			reader:          response.Body,
			compressionPool: r.compressionPools.Get(compression),
			bufferPool:      r.bufferPool,
		}
		var serverErr Error
		if err := unmarshaler.UnmarshalFunc(
			(*connectWireError)(&serverErr),
			json.Unmarshal,
		); err == nil {
			serverErr.meta = r.header.Clone()
			mergeHeaders(serverErr.meta, r.trailer)
			return &serverErr
		}
		return NewError(
			connectHTTPToCode(response.StatusCode),
			errors.New(response.Status),
		)
	}
	r.unmarshaler.compressionPool = r.compressionPools.Get(compression)
	return nil
}

type connectUnaryHandlerSender struct {
	spec           Spec
	responseWriter http.ResponseWriter
	marshaler      connectUnaryMarshaler
	trailer        http.Header
	wroteBody      bool
}

func (s *connectUnaryHandlerSender) Spec() Spec {
	return s.spec
}

func (s *connectUnaryHandlerSender) Header() http.Header {
	return s.responseWriter.Header()
}

func (s *connectUnaryHandlerSender) Trailer() (http.Header, bool) {
	return s.trailer, true
}

func (s *connectUnaryHandlerSender) Send(message any) error {
	s.wroteBody = true
	s.writeHeader(nil /* error */)
	if err := s.marshaler.Marshal(message); err != nil {
		return err
	}
	return nil // must be a literal nil: nil *Error is a non-nil error
}

func (s *connectUnaryHandlerSender) Close(err error) error {
	if !s.wroteBody {
		s.writeHeader(err)
	}
	if err == nil {
		return nil
	}
	// In unary Connect, errors always use application/json.
	s.responseWriter.Header().Set(headerContentType, connectUnaryContentTypeJSON)
	s.responseWriter.WriteHeader(connectCodeToHTTP(CodeOf(err)))
	var wire *connectWireError
	if connectErr, ok := asError(err); ok {
		wire = (*connectWireError)(connectErr)
	} else {
		wire = (*connectWireError)(NewError(CodeUnknown, err))
	}
	data, marshalErr := json.Marshal(wire)
	if marshalErr != nil {
		return errorf(CodeInternal, "marshal error: %w", err)
	}
	_, writeErr := s.responseWriter.Write(data)
	return writeErr
}

func (s *connectUnaryHandlerSender) writeHeader(err error) {
	header := s.responseWriter.Header()
	if err != nil {
		if connectErr, ok := asError(err); ok {
			mergeHeaders(header, connectErr.meta)
		}
	}
	for k, v := range s.trailer {
		header[connectUnaryTrailerPrefix+k] = v
	}
}

type connectUnaryHandlerReceiver struct {
	spec        Spec
	request     *http.Request
	unmarshaler connectUnaryUnmarshaler
}

func (r *connectUnaryHandlerReceiver) Spec() Spec {
	return r.spec
}

func (r *connectUnaryHandlerReceiver) Header() http.Header {
	return r.request.Header
}

func (r *connectUnaryHandlerReceiver) Trailer() (http.Header, bool) {
	return nil, false
}

func (r *connectUnaryHandlerReceiver) Receive(message any) error {
	if err := r.unmarshaler.Unmarshal(message); err != nil {
		return err
	}
	return nil // must be a literal nil: nil *Error is a non-nil error
}

func (r *connectUnaryHandlerReceiver) Close() error {
	return r.request.Body.Close()
}

type connectStreamingMarshaler struct {
	envelopeWriter
}

func (m *connectStreamingMarshaler) MarshalEndStream(err error, trailer http.Header) *Error {
	end := &connectEndStreamMessage{Trailer: trailer}
	if err != nil {
		if connectErr, ok := asError(err); ok {
			mergeHeaders(end.Trailer, connectErr.meta)
			end.Error = (*connectWireError)(connectErr)
		} else {
			end.Error = (*connectWireError)(NewError(CodeUnknown, err))
		}
	}
	data, marshalErr := json.Marshal(end)
	if marshalErr != nil {
		return errorf(CodeInternal, "marshal end stream: %w", marshalErr)
	}
	raw := bytes.NewBuffer(data)
	defer m.envelopeWriter.bufferPool.Put(raw)
	return m.Write(&envelope{
		Data:  raw,
		Flags: connectFlagEnvelopeEndStream,
	})
}

type connectStreamingUnmarshaler struct {
	envelopeReader

	endStreamErr *Error
	trailer      http.Header
}

func (u *connectStreamingUnmarshaler) Unmarshal(message any) *Error {
	err := u.envelopeReader.Unmarshal(message)
	if err == nil {
		return nil
	}
	if !errors.Is(err, errSpecialEnvelope) {
		return err
	}
	env := u.envelopeReader.last
	if !env.IsSet(connectFlagEnvelopeEndStream) {
		return errorf(CodeInternal, "protocol error: invalid envelope flags %d", env.Flags)
	}
	var end connectEndStreamMessage
	if err := json.Unmarshal(env.Data.Bytes(), &end); err != nil {
		return errorf(CodeInternal, "unmarshal end stream message: %w", err)
	}
	u.trailer = end.Trailer
	u.endStreamErr = (*Error)(end.Error)
	return errSpecialEnvelope
}

func (u *connectStreamingUnmarshaler) Trailer() http.Header {
	return u.trailer
}

func (u *connectStreamingUnmarshaler) EndStreamError() *Error {
	return u.endStreamErr
}

type connectUnaryMarshaler struct {
	writer           io.Writer
	codec            Codec
	compressMinBytes int
	compressionName  string
	compressionPool  *compressionPool
	bufferPool       *bufferPool
	header           http.Header
}

func (m *connectUnaryMarshaler) Marshal(message any) *Error {
	data, err := m.codec.Marshal(message)
	if err != nil {
		return errorf(CodeInternal, "marshal message: %w", err)
	}
	// Can't avoid allocating the slice, but we can reuse it.
	uncompressed := bytes.NewBuffer(data)
	defer m.bufferPool.Put(uncompressed)
	if len(data) < m.compressMinBytes || m.compressionPool == nil {
		return m.write(data)
	}
	compressed := m.bufferPool.Get()
	defer m.bufferPool.Put(compressed)
	if err := m.compressionPool.Compress(compressed, uncompressed); err != nil {
		return err
	}
	m.header.Set(connectUnaryHeaderCompression, m.compressionName)
	return m.write(compressed.Bytes())
}

func (m *connectUnaryMarshaler) write(data []byte) *Error {
	if _, err := m.writer.Write(data); err != nil {
		if connectErr, ok := asError(err); ok {
			return connectErr
		}
		return errorf(CodeUnknown, "write message: %w", err)
	}
	return nil
}

type connectUnaryUnmarshaler struct {
	reader          io.Reader
	codec           Codec
	compressionPool *compressionPool
	bufferPool      *bufferPool
	alreadyRead     bool
}

func (u *connectUnaryUnmarshaler) Unmarshal(message any) *Error {
	return u.UnmarshalFunc(message, u.codec.Unmarshal)
}

func (u *connectUnaryUnmarshaler) UnmarshalFunc(message any, unmarshal func([]byte, any) error) *Error {
	if u.alreadyRead {
		return NewError(CodeInternal, io.EOF)
	}
	u.alreadyRead = true
	data := u.bufferPool.Get()
	defer u.bufferPool.Put(data)
	// ReadFrom ignores io.EOF, so any error here is real.
	if _, err := data.ReadFrom(u.reader); err != nil {
		if connectErr, ok := asError(err); ok {
			return connectErr
		}
		return errorf(CodeUnknown, "read message: %w", err)
	}
	if data.Len() > 0 && u.compressionPool != nil {
		decompressed := u.bufferPool.Get()
		defer u.bufferPool.Put(decompressed)
		if err := u.compressionPool.Decompress(decompressed, data); err != nil {
			return err
		}
		data = decompressed
	}
	if err := unmarshal(data.Bytes(), message); err != nil {
		return errorf(CodeInvalidArgument, "unmarshal into %T: %w", message, err)
	}
	return nil
}

type connectWireError Error

func (e *connectWireError) MarshalJSON() ([]byte, error) {
	wire := &errorv1.Error{
		Code:    CodeUnknown.String(),
		Message: (*Error)(e).Error(),
	}
	if connectErr, ok := asError((*Error)(e)); ok {
		wire.Code = connectErr.Code().String()
		wire.Message = connectErr.Message()
		details, err := connectErr.detailsAsAny()
		if err != nil {
			return nil, err
		}
		wire.Details = details
	}
	return (&protoJSONCodec{}).Marshal(wire)
}

func (e *connectWireError) UnmarshalJSON(data []byte) error {
	var wire errorv1.Error
	if err := (&protoJSONCodec{}).Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.Code == "" {
		return nil
	}
	var code Code
	if err := code.UnmarshalText([]byte(wire.Code)); err != nil {
		return err
	}
	e.code = code
	if wire.Message != "" {
		e.err = errors.New(wire.Message)
	}
	if len(wire.Details) > 0 {
		e.details = make([]ErrorDetail, len(wire.Details))
		for i, detail := range wire.Details {
			e.details[i] = detail
		}
	}
	return nil
}

type connectEndStreamMessage struct {
	Error   *connectWireError `json:"error,omitempty"`
	Trailer http.Header       `json:"metadata,omitempty"`
}

func connectCodeToHTTP(code Code) int {
	// Return literals rather than named constants from the HTTP package to make
	// it easier to compare this function to the Connect specification.
	switch code {
	case CodeCanceled:
		return 408
	case CodeUnknown:
		return 500
	case CodeInvalidArgument:
		return 400
	case CodeDeadlineExceeded:
		return 408
	case CodeNotFound:
		return 404
	case CodeAlreadyExists:
		return 409
	case CodePermissionDenied:
		return 403
	case CodeResourceExhausted:
		return 429
	case CodeFailedPrecondition:
		return 412
	case CodeAborted:
		return 409
	case CodeOutOfRange:
		return 400
	case CodeUnimplemented:
		return 404
	case CodeInternal:
		return 500
	case CodeUnavailable:
		return 503
	case CodeDataLoss:
		return 500
	case CodeUnauthenticated:
		return 401
	default:
		return 500 // same as CodeUnknown
	}
}

func connectHTTPToCode(httpCode int) Code {
	// As above, literals are easier to compare to the specificaton (vs named
	// constants).
	switch httpCode {
	case 400:
		return CodeInvalidArgument
	case 401:
		return CodeUnauthenticated
	case 403:
		return CodePermissionDenied
	case 404:
		return CodeUnimplemented
	case 408:
		return CodeDeadlineExceeded
	case 412:
		return CodeFailedPrecondition
	case 413:
		return CodeResourceExhausted
	case 429:
		return CodeUnavailable
	case 431:
		return CodeResourceExhausted
	case 502, 503, 504:
		return CodeUnavailable
	default:
		return CodeUnknown
	}
}

// connectUserAgent returns a User-Agent string similar to those used in gRPC.
func connectUserAgent() string {
	return fmt.Sprintf("connect-go/%s (%s)", Version, runtime.Version())
}

func connectCodecFromContentType(streamType StreamType, contentType string) string {
	if streamType == StreamTypeUnary {
		return strings.TrimPrefix(contentType, connectUnaryContentTypePrefix)
	}
	return strings.TrimPrefix(contentType, connectStreamingContentTypePrefix)
}

func connectContentTypeFromCodecName(streamType StreamType, name string) string {
	if streamType == StreamTypeUnary {
		return connectUnaryContentTypePrefix + name
	}
	return connectStreamingContentTypePrefix + name
}
