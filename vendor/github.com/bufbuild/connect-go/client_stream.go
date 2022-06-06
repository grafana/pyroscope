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
	"errors"
	"io"
	"net/http"
)

// ClientStreamForClient is the client's view of a client streaming RPC.
//
// It's returned from Client.CallClientStream, but doesn't currently have an
// exported constructor function.
type ClientStreamForClient[Req, Res any] struct {
	sender   Sender
	receiver Receiver
	// Error from client construction. If non-nil, return for all calls.
	err error
}

// RequestHeader returns the request headers. Headers are sent to the server with the
// first call to Send.
func (c *ClientStreamForClient[Req, Res]) RequestHeader() http.Header {
	return c.sender.Header()
}

// Send a message to the server. The first call to Send also sends the request
// headers.
//
// If the server returns an error, Send returns an error that wraps io.EOF.
// Clients should check for case using the standard library's errors.Is and
// unmarshal the error using CloseAndReceive.
func (c *ClientStreamForClient[Req, Res]) Send(request *Req) error {
	if c.err != nil {
		return c.err
	}
	return c.sender.Send(request)
}

// CloseAndReceive closes the send side of the stream and waits for the
// response.
func (c *ClientStreamForClient[Req, Res]) CloseAndReceive() (*Response[Res], error) {
	if c.err != nil {
		return nil, c.err
	}
	if err := c.sender.Close(nil); err != nil {
		return nil, err
	}
	response, err := receiveUnaryResponse[Res](c.receiver)
	if err != nil {
		_ = c.receiver.Close()
		return nil, err
	}
	if err := c.receiver.Close(); err != nil {
		return nil, err
	}
	return response, nil
}

// ServerStreamForClient is the client's view of a server streaming RPC.
//
// It's returned from Client.CallServerStream, but doesn't currently have an
// exported constructor function.
type ServerStreamForClient[Res any] struct {
	receiver Receiver
	msg      Res
	err      error
}

// Receive advances the stream to the next message, which will then be
// available through the Msg method. It returns false when the stream stops,
// either by reaching the end or by encountering an unexpected error. After
// Receive returns false, the Err method will return any unexpected error
// encountered.
func (s *ServerStreamForClient[Res]) Receive() bool {
	if s.err != nil {
		return false
	}
	s.err = s.receiver.Receive(&s.msg)
	return s.err == nil
}

// Msg returns the most recent message unmarshaled by a call to Receive. The
// returned message points to data that will be overwritten by the next call to
// Receive.
func (s *ServerStreamForClient[Res]) Msg() *Res {
	return &s.msg
}

// Err returns the first non-EOF error that was encountered by Receive.
func (s *ServerStreamForClient[Res]) Err() error {
	if s.err == nil || errors.Is(s.err, io.EOF) {
		return nil
	}
	return s.err
}

// ResponseHeader returns the headers received from the server. It blocks until
// the first call to Receive returns.
func (s *ServerStreamForClient[Res]) ResponseHeader() http.Header {
	return s.receiver.Header()
}

// ResponseTrailer returns the trailers received from the server. Trailers
// aren't fully populated until Receive() returns an error wrapping io.EOF.
func (s *ServerStreamForClient[Res]) ResponseTrailer() http.Header {
	if trailer, ok := s.receiver.Trailer(); ok {
		return trailer
	}
	return make(http.Header)
}

// Close the receive side of the stream.
func (s *ServerStreamForClient[Res]) Close() error {
	return s.receiver.Close()
}

// BidiStreamForClient is the client's view of a bidirectional streaming RPC.
//
// It's returned from Client.CallBidiStream, but doesn't currently have an
// exported constructor function.
type BidiStreamForClient[Req, Res any] struct {
	sender   Sender
	receiver Receiver
	// Error from client construction. If non-nil, return for all calls.
	err error
}

// RequestHeader returns the request headers. Headers are sent with the first
// call to Send.
func (b *BidiStreamForClient[Req, Res]) RequestHeader() http.Header {
	return b.sender.Header()
}

// Send a message to the server. The first call to Send also sends the request
// headers.
//
// If the server returns an error, Send returns an error that wraps io.EOF.
// Clients should check for EOF using the standard library's errors.Is and
// call Receive to retrieve the error.
func (b *BidiStreamForClient[Req, Res]) Send(msg *Req) error {
	if b.err != nil {
		return b.err
	}
	return b.sender.Send(msg)
}

// CloseSend closes the send side of the stream.
func (b *BidiStreamForClient[Req, Res]) CloseSend() error {
	if b.err != nil {
		return b.err
	}
	return b.sender.Close(nil)
}

// Receive a message. When the server is done sending messages and no other
// errors have occurred, Receive will return an error that wraps io.EOF.
func (b *BidiStreamForClient[Req, Res]) Receive() (*Res, error) {
	if b.err != nil {
		return nil, b.err
	}
	var msg Res
	if err := b.receiver.Receive(&msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// CloseReceive closes the receive side of the stream.
func (b *BidiStreamForClient[Req, Res]) CloseReceive() error {
	if b.err != nil {
		return b.err
	}
	return b.receiver.Close()
}

// ResponseHeader returns the headers received from the server. It blocks until
// the first call to Receive returns.
func (b *BidiStreamForClient[Req, Res]) ResponseHeader() http.Header {
	return b.receiver.Header()
}

// ResponseTrailer returns the trailers received from the server. Trailers
// aren't fully populated until Receive() returns an error wrapping io.EOF.
func (b *BidiStreamForClient[Req, Res]) ResponseTrailer() http.Header {
	if trailer, ok := b.receiver.Trailer(); ok {
		return trailer
	}
	return make(http.Header)
}
