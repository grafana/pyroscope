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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

const (
	headerContentType = "Content-Type"
	headerUserAgent   = "User-Agent"

	discardLimit = 1024 * 1024 * 4 // 4MiB
)

var errNoTimeout = errors.New("no timeout")

// A Protocol defines the HTTP semantics to use when sending and receiving
// messages. It ties together codecs, compressors, and net/http to produce
// Senders and Receivers.
//
// For example, connect supports the gRPC protocol using this abstraction. Among
// many other things, the protocol implementation is responsible for
// translating timeouts from Go contexts to HTTP and vice versa. For gRPC, it
// converts timeouts to and from strings (for example, 10*time.Second <->
// "10S"), and puts those strings into the "Grpc-Timeout" HTTP header. Other
// protocols might encode durations differently, put them into a different HTTP
// header, or ignore them entirely.
//
// We don't have any short-term plans to export this interface; it's just here
// to separate the protocol-specific portions of connect from the
// protocol-agnostic plumbing.
type protocol interface {
	NewHandler(*protocolHandlerParams) protocolHandler
	NewClient(*protocolClientParams) (protocolClient, error)
}

// HandlerParams are the arguments provided to a Protocol's NewHandler
// method, bundled into a struct to allow backward-compatible argument
// additions. Protocol implementations should take care to use the supplied
// Spec rather than constructing their own, since new fields may have been
// added.
type protocolHandlerParams struct {
	Spec             Spec
	Codecs           readOnlyCodecs
	CompressionPools readOnlyCompressionPools
	CompressMinBytes int
	BufferPool       *bufferPool
}

// Handler is the server side of a protocol. HTTP handlers typically support
// multiple protocols, codecs, and compressors.
type protocolHandler interface {
	// ContentTypes is the set of HTTP Content-Types that the protocol can
	// handle.
	ContentTypes() map[string]struct{}

	// ParseTimeout runs before NewStream. Implementations may inspect the HTTP
	// request, parse any timeout set by the client, and return a modified
	// context and cancellation function.
	//
	// If the client didn't send a timeout, SetTimeout should return the
	// request's context, a nil cancellation function, and a nil error.
	SetTimeout(*http.Request) (context.Context, context.CancelFunc, error)

	// NewStream constructs a Sender and Receiver for the message exchange.
	//
	// Implementations may decide whether the returned error should be sent to
	// the client. (For example, it's helpful to send the client a list of
	// supported compressors if they use an unknown compressor.) If the
	// implementation returns a non-nil Sender, its Close method will be called.
	// If the implementation returns a nil Sender, the error won't be sent to the
	// client.
	//
	// In either case, any returned error is passed through the full interceptor
	// stack. If the implementation returns a nil Sender and/or Receiver, the
	// interceptors receive no-op implementations.
	NewStream(http.ResponseWriter, *http.Request) (Sender, Receiver, error)
}

// ClientParams are the arguments provided to a Protocol's NewClient method,
// bundled into a struct to allow backward-compatible argument additions.
// Protocol implementations should take care to use the supplied Spec rather
// than constructing their own, since new fields may have been added.
type protocolClientParams struct {
	CompressionName  string
	CompressionPools readOnlyCompressionPools
	Codec            Codec
	CompressMinBytes int
	HTTPClient       HTTPClient
	URL              string
	BufferPool       *bufferPool
	// The gRPC family of protocols always needs access to a Protobuf codec to
	// marshal and unmarshal errors.
	Protobuf Codec
}

// Client is the client side of a protocol. HTTP clients typically use a single
// protocol, codec, and compressor to send requests.
type protocolClient interface {
	// WriteRequestHeader writes any protocol-specific request headers.
	WriteRequestHeader(StreamType, http.Header)

	// NewStream constructs a Sender and Receiver for the message exchange.
	//
	// Implementations should assume that the supplied HTTP headers have already
	// been populated by WriteRequestHeader. When constructing a stream for a
	// unary call, implementations may assume that the Sender's Send and Close
	// methods return before the Receiver's Receive or Close methods are called.
	NewStream(context.Context, Spec, http.Header) (Sender, Receiver)
}

// errorTranslatingSender wraps a Sender to ensure that we always return coded
// errors to clients and write coded errors to the network.
//
// This is used in protocol implementations.
type errorTranslatingSender struct {
	Sender

	toWire   func(error) error
	fromWire func(error) error
}

func (s *errorTranslatingSender) Send(msg any) error {
	return s.fromWire(s.Sender.Send(msg))
}

func (s *errorTranslatingSender) Close(err error) error {
	sendErr := s.Sender.Close(s.toWire(err))
	return s.fromWire(sendErr)
}

// errorTranslatingReceiver wraps a Receiver to make sure that we always return
// coded errors from clients.
//
// This is used in protocol implementations.
type errorTranslatingReceiver struct {
	Receiver

	fromWire func(error) error
}

func (r *errorTranslatingReceiver) Receive(msg any) error {
	if err := r.Receiver.Receive(msg); err != nil {
		return r.fromWire(err)
	}
	return nil
}

func (r *errorTranslatingReceiver) Close() error {
	return r.fromWire(r.Receiver.Close())
}

// wrapHandlerStreamWithCodedErrors ensures that we (1) automatically code
// context-related errors correctly when writing them to the network, and (2)
// return *Errors from all exported APIs.
func wrapHandlerStreamWithCodedErrors(sender Sender, receiver Receiver) (Sender, Receiver) {
	wrappedSender := &errorTranslatingSender{
		Sender:   sender,
		toWire:   wrapIfContextError,
		fromWire: wrapIfUncoded,
	}
	wrappedReceiver := &errorTranslatingReceiver{
		Receiver: receiver,
		fromWire: wrapIfUncoded,
	}
	return wrappedSender, wrappedReceiver
}

// wrapClientStreamWithCodedErrors ensures that we always return *Errors from
// public APIs.
func wrapClientStreamWithCodedErrors(sender Sender, receiver Receiver) (Sender, Receiver) {
	wrappedSender := &errorTranslatingSender{
		Sender:   sender,
		toWire:   func(err error) error { return err }, // no-op
		fromWire: wrapIfUncoded,
	}
	wrappedReceiver := &errorTranslatingReceiver{
		Receiver: receiver,
		fromWire: wrapIfUncoded,
	}
	return wrappedSender, wrappedReceiver
}

func sortedAcceptPostValue(handlers []protocolHandler) string {
	contentTypes := make(map[string]struct{})
	for _, handler := range handlers {
		for contentType := range handler.ContentTypes() {
			contentTypes[contentType] = struct{}{}
		}
	}
	accept := make([]string, 0, len(contentTypes))
	for ct := range contentTypes {
		accept = append(accept, ct)
	}
	sort.Strings(accept)
	return strings.Join(accept, ", ")
}

func isCommaOrSpace(c rune) bool {
	return c == ',' || c == ' '
}

func discard(reader io.Reader) error {
	if lr, ok := reader.(*io.LimitedReader); ok {
		_, err := io.Copy(io.Discard, lr)
		return err
	}
	// We don't want to get stuck throwing data away forever, so limit how much
	// we're willing to do here.
	lr := &io.LimitedReader{R: reader, N: discardLimit}
	_, err := io.Copy(io.Discard, lr)
	return err
}

func validateRequestURL(uri string) *Error {
	_, err := url.ParseRequestURI(uri)
	if err == nil {
		return nil
	}
	if !strings.Contains(uri, "://") {
		// URL doesn't have a scheme, so the user is likely accustomed to
		// grpc-go's APIs.
		err = fmt.Errorf(
			"URL %q missing scheme: use http:// or https:// (unlike grpc-go)",
			uri,
		)
	}
	return NewError(CodeUnavailable, err)
}

// negotiateCompression determines and validates the request compression and
// response compression using the available compressors and protocol-specific
// Content-Encoding and Accept-Encoding headers.
func negotiateCompression( // nolint:nonamedreturns
	availableCompressors readOnlyCompressionPools,
	sent, accept string,
) (requestCompression, responseCompression string, clientVisibleErr *Error) {
	requestCompression = compressionIdentity
	if sent != "" && sent != compressionIdentity {
		// We default to identity, so we only care if the client sends something
		// other than the empty string or compressIdentity.
		if availableCompressors.Contains(sent) {
			requestCompression = sent
		} else {
			// To comply with
			// https://github.com/grpc/grpc/blob/master/doc/compression.md and the
			// Connect protocol, we should return CodeUnimplemented and specify
			// acceptable compression(s) (in addition to setting the a
			// protocol-specific accept-encoding header).
			return "", "", errorf(
				CodeUnimplemented,
				"unknown compression %q: supported encodings are %v",
				sent, availableCompressors.CommaSeparatedNames(),
			)
		}
	}
	// Support asymmetric compression. This logic follows
	// https://github.com/grpc/grpc/blob/master/doc/compression.md and common
	// sense.
	responseCompression = requestCompression
	// If we're not already planning to compress the response, check whether the
	// client requested a compression algorithm we support.
	if responseCompression == compressionIdentity && accept != "" {
		for _, name := range strings.FieldsFunc(accept, isCommaOrSpace) {
			if availableCompressors.Contains(name) {
				// We found a mutually supported compression algorithm. Unlike standard
				// HTTP, there's no preference weighting, so can bail out immediately.
				responseCompression = name
				break
			}
		}
	}
	return requestCompression, responseCompression, nil
}

func flushResponseWriter(w http.ResponseWriter) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
