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
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// An ErrorWriter writes errors to an [http.ResponseWriter] in the format
// expected by an RPC client. This is especially useful in server-side net/http
// middleware, where you may wish to handle requests from RPC and non-RPC
// clients with the same code.
//
// ErrorWriters are safe to use concurrently.
type ErrorWriter struct {
	bufferPool                   *bufferPool
	protobuf                     Codec
	allContentTypes              map[string]struct{}
	grpcContentTypes             map[string]struct{}
	grpcWebContentTypes          map[string]struct{}
	unaryConnectContentTypes     map[string]struct{}
	streamingConnectContentTypes map[string]struct{}
}

// NewErrorWriter constructs an ErrorWriter. To properly recognize supported
// RPC Content-Types in net/http middleware, you must pass the same
// HandlerOptions to NewErrorWriter and any wrapped Connect handlers.
func NewErrorWriter(opts ...HandlerOption) *ErrorWriter {
	config := newHandlerConfig("", opts)
	writer := &ErrorWriter{
		bufferPool:                   config.BufferPool,
		protobuf:                     newReadOnlyCodecs(config.Codecs).Protobuf(),
		allContentTypes:              make(map[string]struct{}),
		grpcContentTypes:             make(map[string]struct{}),
		grpcWebContentTypes:          make(map[string]struct{}),
		unaryConnectContentTypes:     make(map[string]struct{}),
		streamingConnectContentTypes: make(map[string]struct{}),
	}
	for name := range config.Codecs {
		unary := connectContentTypeFromCodecName(StreamTypeUnary, name)
		writer.allContentTypes[unary] = struct{}{}
		writer.unaryConnectContentTypes[unary] = struct{}{}
		streaming := connectContentTypeFromCodecName(StreamTypeBidi, name)
		writer.streamingConnectContentTypes[streaming] = struct{}{}
		writer.allContentTypes[streaming] = struct{}{}
	}
	if config.HandleGRPC {
		writer.grpcContentTypes[grpcContentTypeDefault] = struct{}{}
		writer.allContentTypes[grpcContentTypeDefault] = struct{}{}
		for name := range config.Codecs {
			ct := grpcContentTypeFromCodecName(false /* web */, name)
			writer.grpcContentTypes[ct] = struct{}{}
			writer.allContentTypes[ct] = struct{}{}
		}
	}
	if config.HandleGRPCWeb {
		writer.grpcWebContentTypes[grpcWebContentTypeDefault] = struct{}{}
		writer.allContentTypes[grpcWebContentTypeDefault] = struct{}{}
		for name := range config.Codecs {
			ct := grpcContentTypeFromCodecName(true /* web */, name)
			writer.grpcWebContentTypes[ct] = struct{}{}
			writer.allContentTypes[ct] = struct{}{}
		}
	}
	return writer
}

// IsSupported checks whether a request is using one of the ErrorWriter's
// supported RPC protocols.
func (w *ErrorWriter) IsSupported(request *http.Request) bool {
	ctype := canonicalizeContentType(getHeaderCanonical(request.Header, headerContentType))
	_, ok := w.allContentTypes[ctype]
	return ok
}

// Write an error, using the format appropriate for the RPC protocol in use.
// Callers should first use IsSupported to verify that the request is using one
// of the ErrorWriter's supported RPC protocols.
//
// Write does not read or close the request body.
func (w *ErrorWriter) Write(response http.ResponseWriter, request *http.Request, err error) error {
	ctype := canonicalizeContentType(getHeaderCanonical(request.Header, headerContentType))
	if _, ok := w.unaryConnectContentTypes[ctype]; ok {
		// Unary errors are always JSON.
		setHeaderCanonical(response.Header(), headerContentType, connectUnaryContentTypeJSON)
		return w.writeConnectUnary(response, err)
	}
	if _, ok := w.streamingConnectContentTypes[ctype]; ok {
		setHeaderCanonical(response.Header(), headerContentType, ctype)
		return w.writeConnectStreaming(response, err)
	}
	if _, ok := w.grpcContentTypes[ctype]; ok {
		setHeaderCanonical(response.Header(), headerContentType, ctype)
		return w.writeGRPC(response, err)
	}
	if _, ok := w.grpcWebContentTypes[ctype]; ok {
		setHeaderCanonical(response.Header(), headerContentType, ctype)
		return w.writeGRPCWeb(response, err)
	}
	return fmt.Errorf("unsupported Content-Type %q", ctype)
}

func (w *ErrorWriter) writeConnectUnary(response http.ResponseWriter, err error) error {
	if connectErr, ok := asError(err); ok {
		mergeHeaders(response.Header(), connectErr.meta)
	}
	response.WriteHeader(connectCodeToHTTP(CodeOf(err)))
	data, marshalErr := json.Marshal(newConnectWireError(err))
	if marshalErr != nil {
		return fmt.Errorf("marshal error: %w", marshalErr)
	}
	_, writeErr := response.Write(data)
	return writeErr
}

func (w *ErrorWriter) writeConnectStreaming(response http.ResponseWriter, err error) error {
	response.WriteHeader(http.StatusOK)
	marshaler := &connectStreamingMarshaler{
		envelopeWriter: envelopeWriter{
			writer:     response,
			bufferPool: w.bufferPool,
		},
	}
	// MarshalEndStream returns *Error: check return value to avoid typed nils.
	if marshalErr := marshaler.MarshalEndStream(err, make(http.Header)); marshalErr != nil {
		return marshalErr
	}
	return nil
}

func (w *ErrorWriter) writeGRPC(response http.ResponseWriter, err error) error {
	trailers := make(http.Header, 2) // need space for at least code & message
	grpcErrorToTrailer(trailers, w.protobuf, err)
	// To make net/http reliably send trailers without a body, we must set the
	// Trailers header rather than using http.TrailerPrefix. See
	// https://github.com/golang/go/issues/54723.
	keys := make([]string, 0, len(trailers))
	for k := range trailers {
		keys = append(keys, k)
	}
	setHeaderCanonical(response.Header(), headerTrailer, strings.Join(keys, ","))
	response.WriteHeader(http.StatusOK)
	mergeHeaders(response.Header(), trailers)
	return nil
}

func (w *ErrorWriter) writeGRPCWeb(response http.ResponseWriter, err error) error {
	// This is a trailers-only response. To match the behavior of Envoy and
	// protocol_grpc.go, put the trailers in the HTTP headers.
	grpcErrorToTrailer(response.Header(), w.protobuf, err)
	response.WriteHeader(http.StatusOK)
	return nil
}
