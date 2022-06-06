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

// ClientStream is the handler's view of a client streaming RPC.
//
// It's constructed as part of Handler invocation, but doesn't currently have
// an exported constructor.
type ClientStream[Req any] struct {
	receiver Receiver
	msg      Req
	err      error
}

// RequestHeader returns the headers received from the client.
func (c *ClientStream[Req]) RequestHeader() http.Header {
	return c.receiver.Header()
}

// Receive advances the stream to the next message, which will then be
// available through the Msg method. It returns false when the stream stops,
// either by reaching the end or by encountering an unexpected error. After
// Receive returns false, the Err method will return any unexpected error
// encountered.
func (c *ClientStream[Req]) Receive() bool {
	if c.err != nil {
		return false
	}
	c.err = c.receiver.Receive(&c.msg)
	return c.err == nil
}

// Msg returns the most recent message unmarshaled by a call to Receive. The
// returned message points to data that will be overwritten by the next call to
// Receive.
func (c *ClientStream[Req]) Msg() *Req {
	return &c.msg
}

// Err returns the first non-EOF error that was encountered by Receive.
func (c *ClientStream[Req]) Err() error {
	if c.err == nil || errors.Is(c.err, io.EOF) {
		return nil
	}
	return c.err
}

// ServerStream is the handler's view of a server streaming RPC.
//
// It's constructed as part of Handler invocation, but doesn't currently have
// an exported constructor.
type ServerStream[Res any] struct {
	sender Sender
}

// ResponseHeader returns the response headers. Headers are sent with the first
// call to Send.
func (s *ServerStream[Res]) ResponseHeader() http.Header {
	return s.sender.Header()
}

// ResponseTrailer returns the response trailers. Handlers may write to the
// response trailers at any time before returning.
func (s *ServerStream[Res]) ResponseTrailer() http.Header {
	if trailers, ok := s.sender.Trailer(); ok {
		return trailers
	}
	return make(http.Header)
}

// Send a message to the client. The first call to Send also sends the response
// headers.
func (s *ServerStream[Res]) Send(msg *Res) error {
	return s.sender.Send(msg)
}

// BidiStream is the handler's view of a bidirectional streaming RPC.
//
// It's constructed as part of Handler invocation, but doesn't currently have
// an exported constructor.
type BidiStream[Req, Res any] struct {
	sender   Sender
	receiver Receiver
}

// RequestHeader returns the headers received from the client.
func (b *BidiStream[Req, Res]) RequestHeader() http.Header {
	return b.receiver.Header()
}

// Receive a message. When the client is done sending messages, Receive will
// return an error that wraps io.EOF.
func (b *BidiStream[Req, Res]) Receive() (*Req, error) {
	var req Req
	if err := b.receiver.Receive(&req); err != nil {
		return nil, err
	}
	return &req, nil
}

// ResponseHeader returns the response headers. Headers are sent with the first
// call to Send.
func (b *BidiStream[Req, Res]) ResponseHeader() http.Header {
	return b.sender.Header()
}

// ResponseTrailer returns the response trailers. Handlers may write to the
// response trailers at any time before returning.
func (b *BidiStream[Req, Res]) ResponseTrailer() http.Header {
	if trailers, ok := b.sender.Trailer(); ok {
		return trailers
	}
	return make(http.Header)
}

// Send a message to the client. The first call to Send also sends the response
// headers.
func (b *BidiStream[Req, Res]) Send(msg *Res) error {
	return b.sender.Send(msg)
}
