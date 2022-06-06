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
	"net/http"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"
)

const commonErrorsURL = "https://connect.build/docs/go/common-errors"

// An ErrorDetail is a self-describing Protobuf message attached to an *Error.
// Error details are sent over the network to clients, which can then work with
// strongly-typed data rather than trying to parse a complex error message.
//
// The ErrorDetail interface is implemented by Protobuf's Any type, provided in
// Go by the google.golang.org/protobuf/types/known/anypb package. The
// google.golang.org/genproto/googleapis/rpc/errdetails package contains a
// variety of Protobuf messages commonly wrapped in anypb.Any and used as error
// details.
type ErrorDetail interface {
	proto.Message

	MessageName() protoreflect.FullName
	UnmarshalTo(proto.Message) error
}

// An Error captures four key pieces of information: a Code, an underlying Go
// error, a map of metadata, and an optional collection of arbitrary Protobuf
// messages called "details" (more on those below). Servers send the code, the
// underlying error's Error() output, the metadata, and details over the wire
// to clients. Remember that the underlying error's message will be sent to
// clients - take care not to leak sensitive information from public APIs!
//
// Service implementations and interceptors should return errors that can be
// cast to an *Error (using the standard library's errors.As). If the returned
// error can't be cast to an *Error, connect will use CodeUnknown and the
// returned error's message.
//
// Error details are an optional mechanism for servers, interceptors, and
// proxies to attach arbitrary Protobuf messages to the error code and message.
// They're a clearer and more performant alternative to HTTP header
// microformats. See https://connect.build/docs/go/errors for more details.
type Error struct {
	code    Code
	err     error
	details []ErrorDetail
	meta    http.Header
}

// NewError annotates any Go error with a status code.
func NewError(c Code, underlying error) *Error {
	return &Error{code: c, err: underlying}
}

func (e *Error) Error() string {
	message := e.Message()
	if message == "" {
		return e.code.String()
	}
	return e.code.String() + ": " + message
}

// Message returns the underlying error message. It may be empty if the
// original error was created with a status code and a nil error.
func (e *Error) Message() string {
	if e.err != nil {
		return e.err.Error()
	}
	return ""
}

// Unwrap implements errors.Wrapper, which allows errors.Is and errors.As
// access to the underlying error.
func (e *Error) Unwrap() error {
	return e.err
}

// Code returns the error's status code.
func (e *Error) Code() Code {
	return e.code
}

// Details returns the error's details.
func (e *Error) Details() []ErrorDetail {
	return e.details
}

// AddDetail appends a message to the error's details.
func (e *Error) AddDetail(d ErrorDetail) {
	e.details = append(e.details, d)
}

// Meta allows the error to carry additional information as key-value pairs.
//
// Metadata attached to errors returned by unary handlers is always sent as
// HTTP headers, regardless of the protocol. Metadata attached to errors
// returned by streaming handlers may be sent as HTTP headers, HTTP trailers,
// or a block of in-body metadata, depending on the protocol in use and whether
// or not the handler has already written messages to the stream.
//
// When clients receive errors, the metadata contains the union of the HTTP
// headers and the protocol-specific trailers (either HTTP trailers or in-body
// metadata).
func (e *Error) Meta() http.Header {
	if e.meta == nil {
		e.meta = make(http.Header)
	}
	return e.meta
}

func (e *Error) detailsAsAny() ([]*anypb.Any, error) {
	anys := make([]*anypb.Any, 0, len(e.details))
	for _, detail := range e.details {
		// If the detail is already a protobuf Any, we're golden.
		if anyProtoDetail, ok := detail.(*anypb.Any); ok {
			anys = append(anys, anyProtoDetail)
			continue
		}
		// Otherwise, we convert it to an Any.
		anyProtoDetail, err := anypb.New(detail)
		if err != nil {
			return nil, fmt.Errorf(
				"can't create an *anypb.Any from %v (type %T): %w",
				detail,
				detail,
				err,
			)
		}
		anys = append(anys, anyProtoDetail)
	}
	return anys, nil
}

// errorf calls fmt.Errorf with the supplied template and arguments, then wraps
// the resulting error.
func errorf(c Code, template string, args ...any) *Error {
	return NewError(c, fmt.Errorf(template, args...))
}

// asError uses errors.As to unwrap any error and look for a connect *Error.
func asError(err error) (*Error, bool) {
	var connectErr *Error
	ok := errors.As(err, &connectErr)
	return connectErr, ok
}

// wrapIfUncoded ensures that all errors are wrapped. It leaves already-wrapped
// errors unchanged, uses wrapIfContextError to apply codes to context.Canceled
// and context.DeadlineExceeded, and falls back to wrapping other errors with
// CodeUnknown.
func wrapIfUncoded(err error) error {
	if err == nil {
		return nil
	}
	maybeCodedErr := wrapIfContextError(err)
	if _, ok := asError(maybeCodedErr); ok {
		return maybeCodedErr
	}
	return NewError(CodeUnknown, maybeCodedErr)
}

// wrapIfContextError applies CodeCanceled or CodeDeadlineExceeded to Go's
// context.Canceled and context.DeadlineExceeded errors, but only if they
// haven't already been wrapped.
func wrapIfContextError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := asError(err); ok {
		return err
	}
	if errors.Is(err, context.Canceled) {
		return NewError(CodeCanceled, err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return NewError(CodeDeadlineExceeded, err)
	}
	return err
}

// wrapIfLikelyWithGRPCNotUsedError adds a wrapping error that has a message
// telling the caller that they likely need to use h2c but are using a raw http.Client{}.
//
// This happens when running a gRPC-only server.
// This is fragile and may break over time, and this should be considered a best-effort.
func wrapIfLikelyH2CNotConfiguredError(request *http.Request, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := asError(err); ok {
		return err
	}
	if url := request.URL; url != nil && url.Scheme != "http" {
		// If the scheme is not http, we definitely do not have an h2c error, so just return.
		return err
	}
	// net/http code has been investigated and there is no typing of any of these errors
	// they are all created with fmt.Errorf
	// grpc-go returns the first error 2/3-3/4 of the time, and the second error 1/4-1/3 of the time
	if errString := err.Error(); strings.HasPrefix(errString, `Post "`) &&
		(strings.Contains(errString, `net/http: HTTP/1.x transport connection broken: malformed HTTP response`) ||
			strings.HasSuffix(errString, `write: broken pipe`)) {
		return fmt.Errorf("possible h2c configuration issue when talking to gRPC server, see %s: %w", commonErrorsURL, err)
	}
	return err
}

// wrapIfLikelyWithGRPCNotUsedError adds a wrapping error that has a message
// telling the caller that they likely forgot to use connect.WithGRPC().
//
// This happens when running a gRPC-only server.
// This is fragile and may break over time, and this should be considered a best-effort.
func wrapIfLikelyWithGRPCNotUsedError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := asError(err); ok {
		return err
	}
	// golang.org/x/net code has been investigated and there is no typing of this error
	// it is created with fmt.Errorf
	// http2/transport.go:573:	return nil, fmt.Errorf("http2: Transport: cannot retry err [%v] after Request.Body was written; define Request.GetBody to avoid this error", err)
	if errString := err.Error(); strings.HasPrefix(errString, `Post "`) &&
		strings.Contains(errString, `http2: Transport: cannot retry err`) &&
		strings.HasSuffix(errString, `after Request.Body was written; define Request.GetBody to avoid this error`) {
		return fmt.Errorf("possible missing connect.WithGPRC() client option when talking to gRPC server, see %s: %w", commonErrorsURL, err)
	}
	return err
}
