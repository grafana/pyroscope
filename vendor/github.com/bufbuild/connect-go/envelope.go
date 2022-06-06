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
	"encoding/binary"
	"errors"
	"io"
)

// flagEnvelopeCompressed indicates that the data is compressed. It has the
// same meaning in the gRPC-Web, gRPC-HTTP2, and Connect protocols.
const flagEnvelopeCompressed = 0b00000001

var errSpecialEnvelope = errorf(
	CodeUnknown,
	"final message has protocol-specific flags: %w",
	// User code checks for end of stream with errors.Is(err, io.EOF).
	io.EOF,
)

// envelope is a block of arbitrary bytes wrapped in gRPC and Connect's framing
// protocol.
//
// Each message is preceded by a 5-byte prefix. The first byte is a uint8 used
// as a set of bitwise flags, and the remainder is a uint32 indicating the
// message length. gRPC and Connect interpret the bitwise flags differently, so
// envelope leaves their interpretation up to the caller.
type envelope struct {
	Data  *bytes.Buffer
	Flags uint8
}

func (e *envelope) IsSet(flag uint8) bool {
	return e.Flags&flag == flag
}

type envelopeWriter struct {
	writer           io.Writer
	codec            Codec
	compressMinBytes int
	compressionPool  *compressionPool
	bufferPool       *bufferPool
}

func (w *envelopeWriter) Marshal(message any) *Error {
	raw, err := w.codec.Marshal(message)
	if err != nil {
		return errorf(CodeInternal, "marshal message: %w", err)
	}
	// We can't avoid allocating the byte slice, so we may as well reuse it once
	// we're done with it.
	buffer := bytes.NewBuffer(raw)
	defer w.bufferPool.Put(buffer)
	envelope := &envelope{Data: buffer}
	return w.Write(envelope)
}

// Write writes the enveloped message, compressing as necessary. It doesn't
// retain any references to the supplied envelope or its underlying data.
func (w *envelopeWriter) Write(env *envelope) *Error {
	if env.IsSet(flagEnvelopeCompressed) ||
		w.compressionPool == nil ||
		env.Data.Len() < w.compressMinBytes {
		return w.write(env)
	}
	data := w.bufferPool.Get()
	defer w.bufferPool.Put(data)
	if err := w.compressionPool.Compress(data, env.Data); err != nil {
		return err
	}
	return w.write(&envelope{
		Data:  data,
		Flags: env.Flags | flagEnvelopeCompressed,
	})
}

func (w *envelopeWriter) write(env *envelope) *Error {
	prefix := [5]byte{}
	prefix[0] = env.Flags
	binary.BigEndian.PutUint32(prefix[1:5], uint32(env.Data.Len()))
	if _, err := w.writer.Write(prefix[:]); err != nil {
		if connectErr, ok := asError(err); ok {
			return connectErr
		}
		return errorf(CodeUnknown, "write envelope: %w", err)
	}
	if _, err := io.Copy(w.writer, env.Data); err != nil {
		return errorf(CodeUnknown, "write message: %w", err)
	}
	return nil
}

type envelopeReader struct {
	reader          io.Reader
	codec           Codec
	last            envelope
	compressionPool *compressionPool
	bufferPool      *bufferPool
}

func (r *envelopeReader) Unmarshal(message any) *Error {
	buffer := r.bufferPool.Get()
	defer r.bufferPool.Put(buffer)

	env := &envelope{Data: buffer}
	err := r.Read(env)
	switch {
	case err == nil &&
		(env.Flags == 0 || env.Flags == flagEnvelopeCompressed) &&
		env.Data.Len() == 0:
		// This is a standard message (because none of the top 7 bits are set) and
		// there's no data, so the zero value of the message is correct.
		return nil
	case err != nil && errors.Is(err, io.EOF):
		// The stream has ended. Propagate the EOF to the caller.
		return err
	case err != nil:
		// Something's wrong.
		return err
	}

	data := env.Data
	if data.Len() > 0 && env.IsSet(flagEnvelopeCompressed) {
		if r.compressionPool == nil {
			return errorf(
				CodeInvalidArgument,
				"gRPC protocol error: sent compressed message without Grpc-Encoding header",
			)
		}
		decompressed := r.bufferPool.Get()
		defer r.bufferPool.Put(decompressed)
		if err := r.compressionPool.Decompress(decompressed, data); err != nil {
			return err
		}
		data = decompressed
	}

	if env.Flags != 0 && env.Flags != flagEnvelopeCompressed {
		// One of the protocol-specific flags are set, so this is the end of the
		// stream. Save the message for protocol-specific code to process and
		// return a sentinel error. Since we've deferred functions to return env's
		// underlying buffer to a pool, we need to keep a copy.
		r.last = envelope{
			Data:  r.bufferPool.Get(),
			Flags: env.Flags,
		}
		// Don't return last to the pool! We're going to reference the data
		// elsewhere.
		if _, err := r.last.Data.ReadFrom(data); err != nil {
			return errorf(CodeUnknown, "copy final envelope: %w", err)
		}
		return errSpecialEnvelope
	}

	if err := r.codec.Unmarshal(data.Bytes(), message); err != nil {
		return errorf(CodeInvalidArgument, "unmarshal into %T: %w", message, err)
	}
	return nil
}

func (r *envelopeReader) Read(env *envelope) *Error {
	prefixes := [5]byte{}
	prefixBytesRead, err := r.reader.Read(prefixes[:])

	switch {
	case (err == nil || errors.Is(err, io.EOF)) &&
		prefixBytesRead == 5 &&
		isSizeZeroPrefix(prefixes):
		// Successfully read prefix and expect no additional data.
		env.Flags = prefixes[0]
		return nil
	case err != nil && errors.Is(err, io.EOF) && prefixBytesRead == 0:
		// The stream ended cleanly. That's expected, but we need to propagate them
		// to the user so that they know that the stream has ended. We shouldn't
		// add any alarming text about protocol errors, though.
		return NewError(CodeUnknown, err)
	case err != nil || prefixBytesRead < 5:
		// Something else has gone wrong - the stream didn't end cleanly.
		if connectErr, ok := asError(err); ok {
			return connectErr
		}
		return errorf(
			CodeInvalidArgument,
			"protocol error: incomplete envelope: %w", err,
		)
	}
	size := int(binary.BigEndian.Uint32(prefixes[1:5]))
	if size < 0 {
		return errorf(CodeInvalidArgument, "message size %d overflowed uint32", size)
	}
	if size > 0 {
		env.Data.Grow(size)
		// At layer 7, we don't know exactly what's happening down in L4. Large
		// length-prefixed messages may arrive in chunks, so we may need to read
		// the request body past EOF. We also need to take care that we don't retry
		// forever if the message is malformed.
		remaining := int64(size)
		for remaining > 0 {
			bytesRead, err := io.CopyN(env.Data, r.reader, remaining)
			if err != nil && !errors.Is(err, io.EOF) {
				return errorf(CodeUnknown, "read enveloped message: %w", err)
			}
			if errors.Is(err, io.EOF) && bytesRead == 0 {
				// We've gotten zero-length chunk of data. Message is likely malformed,
				// don't wait for additional chunks.
				return errorf(
					CodeInvalidArgument,
					"protocol error: promised %d bytes in enveloped message, got %d bytes",
					size,
					int64(size)-remaining,
				)
			}
			remaining -= bytesRead
		}
	}
	env.Flags = prefixes[0]
	return nil
}

func isSizeZeroPrefix(prefix [5]byte) bool {
	for i := 1; i < 5; i++ {
		if prefix[i] != 0 {
			return false
		}
	}
	return true
}
