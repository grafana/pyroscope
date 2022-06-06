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

// Package connect is a slim RPC framework built on Protocol Buffers and
// net/http. In addition to supporting its own protocol, Connect handlers and
// clients are wire-compatible with gRPC and gRPC-Web, including streaming.
//
// This documentation is intended to explain each type and function in
// isolation. For walkthroughs, FAQs, and other narrative docs, see
// https://connect.build. For a working demonstration service, see
// https://github.com/bufbuild/connect-demo.
package connect

import (
	"errors"
	"io"
	"net/http"
)

// Version is the semantic version of the connect module.
const Version = "0.1.0"

// These constants are used in compile-time handshakes with connect's generated
// code.
const (
	IsAtLeastVersion0_0_1 = true
	IsAtLeastVersion0_1_0 = true
)

// StreamType describes whether the client, server, neither, or both is
// streaming.
type StreamType uint8

const (
	StreamTypeUnary  StreamType = 0b00
	StreamTypeClient            = 0b01
	StreamTypeServer            = 0b10
	StreamTypeBidi              = StreamTypeClient | StreamTypeServer
)

// Sender is the writable side of a bidirectional stream of messages. Sender
// implementations do not need to be safe for concurrent use.
//
// Sender implementations provided by this module guarantee that all returned
// errors can be cast to *Error using errors.As. The Close method of Sender
// implementations provided by this module automatically adds the appropriate
// codes when passed context.DeadlineExceeded or context.Canceled.
//
// Like the standard library's http.ResponseWriter, both client- and
// handler-side Senders write headers to the network with the first call to
// Send. Any subsequent mutations to the headers are effectively no-ops.
//
// Handler-side Senders may mutate trailers until calling Close, when the
// trailers are written to the network. Clients may not send trailers, since
// the gRPC, gRPC-Web, and Connect protocols all forbid it.
//
// Once servers return an error, they're not interested in receiving additional
// messages and clients should stop sending them. Client-side Senders indicate
// this by returning a wrapped io.EOF from Send. Clients should check for this
// condition with the standard library's errors.Is and call the receiver's
// Receive method to unmarshal the error.
type Sender interface {
	Send(any) error
	Close(error) error

	Spec() Spec
	Header() http.Header
	Trailer() (http.Header, bool)
}

// Receiver is the readable side of a bidirectional stream of messages.
// Receiver implementations do not need to be safe for concurrent use.
//
// Receiver implementations provided by this module guarantee that all returned
// errors can be cast to *Error using errors.As.
//
// Only client-side Receivers may read trailers.
type Receiver interface {
	Receive(any) error
	Close() error

	Spec() Spec
	Header() http.Header
	// Trailers are populated only after Receive returns an error.
	Trailer() (http.Header, bool)
}

// Request is a wrapper around a generated request message. It provides
// access to metadata like headers and the RPC specification, as well as
// strongly-typed access to the message itself.
type Request[T any] struct {
	Msg *T

	spec   Spec
	header http.Header
}

// NewRequest wraps a generated request message.
func NewRequest[T any](message *T) *Request[T] {
	return &Request[T]{
		Msg: message,
		// Initialized lazily so we don't allocate unnecessarily.
		header: nil,
	}
}

// Any returns the concrete request message as an empty interface, so that
// *Request implements the AnyRequest interface.
func (r *Request[_]) Any() any {
	return r.Msg
}

// Spec returns a description of this RPC.
func (r *Request[_]) Spec() Spec {
	return r.spec
}

// Header returns the HTTP headers for this request.
func (r *Request[_]) Header() http.Header {
	if r.header == nil {
		r.header = make(http.Header)
	}
	return r.header
}

// internalOnly implements AnyRequest.
func (e *Request[_]) internalOnly() {}

// AnyRequest is the common method set of all Requests, regardless of type
// parameter. It's used in unary interceptors.
//
// To preserve our ability to add methods to this interface without breaking
// backward compatibility, only types defined in this package can implement
// AnyRequest.
type AnyRequest interface {
	Any() any
	Spec() Spec
	Header() http.Header

	internalOnly()
}

// Response is a wrapper around a generated response message. It provides
// access to metadata like headers and trailers, as well as strongly-typed
// access to the message itself.
type Response[T any] struct {
	Msg *T

	header  http.Header
	trailer http.Header
}

// NewResponse wraps a generated response message.
func NewResponse[T any](message *T) *Response[T] {
	return &Response[T]{
		Msg: message,
		// Initialized lazily so we don't allocate unnecessarily.
		header:  nil,
		trailer: nil,
	}
}

// Any returns the concrete response message as an empty interface, so that
// *Response implements the AnyResponse interface.
func (r *Response[_]) Any() any {
	return r.Msg
}

// Header returns the HTTP headers for this response.
func (r *Response[_]) Header() http.Header {
	if r.header == nil {
		r.header = make(http.Header)
	}
	return r.header
}

// Trailer returns the trailers for this response. Depending on the underlying
// RPC protocol, trailers may be sent as HTTP trailers or a protocol-specific
// block of in-body metadata.
func (r *Response[_]) Trailer() http.Header {
	if r.trailer == nil {
		r.trailer = make(http.Header)
	}
	return r.trailer
}

// internalOnly implements AnyResponse.
func (e *Response[_]) internalOnly() {}

// AnyResponse is the common method set of all Responses, regardless of type
// parameter. It's used in unary interceptors.
//
// To preserve our ability to add methods to this interface without breaking
// backward compatibility, only types defined in this package can implement
// AnyRequest.
type AnyResponse interface {
	Any() any
	Header() http.Header
	Trailer() http.Header

	internalOnly()
}

// HTTPClient is the interface connect expects HTTP clients to implement. The
// standard library's *http.Client implements HTTPClient.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// Spec is a description of a client call or a handler invocation.
type Spec struct {
	StreamType StreamType
	Procedure  string // for example, "/acme.foo.v1.FooService/Bar"
	IsClient   bool   // otherwise we're in a handler
}

// receiveUnaryRequest unmarshals a message from a Receiver, then envelopes
// the message and attaches the Receiver's headers and RPC specification.
func receiveUnaryRequest[T any](receiver Receiver) (*Request[T], error) {
	var msg T
	if err := receiver.Receive(&msg); err != nil {
		return nil, err
	}
	return &Request[T]{
		Msg:    &msg,
		spec:   receiver.Spec(),
		header: receiver.Header(),
	}, nil
}

func receiveUnaryRequestMetadata[T any](r Receiver) *Request[T] {
	return &Request[T]{
		Msg:    new(T),
		spec:   r.Spec(),
		header: r.Header(),
	}
}

// receiveUnaryResponse unmarshals a message from a Receiver, then envelopes
// the message and attaches the Receiver's headers and trailers. It attempts to
// consume the Receiver and isn't appropriate when receiving multiple messages.
func receiveUnaryResponse[T any](receiver Receiver) (*Response[T], error) {
	var msg T
	if err := receiver.Receive(&msg); err != nil {
		return nil, err
	}
	// In a well-formed stream, the response message may be followed by a block
	// of in-stream trailers or HTTP trailers. To ensure that we receive the
	// trailers, try to read another message from the stream.
	if err := receiver.Receive(new(T)); err == nil {
		return nil, NewError(CodeUnknown, errors.New("unary stream has multiple messages"))
	} else if err != nil && !errors.Is(err, io.EOF) {
		return nil, NewError(CodeUnknown, err)
	}
	response := &Response[T]{
		Msg:    &msg,
		header: receiver.Header(),
	}
	if trailer, ok := receiver.Trailer(); ok {
		response.trailer = trailer
	}
	return response, nil
}
