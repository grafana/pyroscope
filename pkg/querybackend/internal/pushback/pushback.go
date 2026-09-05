// Package pushback carries the gRPC retry pushback signal between a
// query-backend server and its clients.
//
// The client retries RESOURCE_EXHAUSTED, which the server returns when shedding
// load. gRPC returns the same code when a response exceeds the send limit, which
// no retry can fix, so the verdict travels out of band, in the trailer gRPC
// reserves for it (gRFC A6).
package pushback

import (
	"context"
	"errors"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	metadataKey = "grpc-retry-pushback-ms"
	noRetry     = "-1"
)

var noRetryTrailer = metadata.Pairs(metadataKey, noRetry)

type noRetryError struct{ err error }

func (e *noRetryError) Error() string              { return e.err.Error() }
func (e *noRetryError) Unwrap() error              { return e.err }
func (e *noRetryError) GRPCStatus() *status.Status { return status.Convert(e.err) }
func (e *noRetryError) noRetry()                   {}

// Mark records that retrying err cannot succeed, preserving its gRPC status.
func Mark(err error) error {
	if err == nil || IsMarked(err) {
		return err
	}
	return &noRetryError{err: err}
}

// IsMarked reports whether err, or any error it wraps, is marked.
func IsMarked(err error) bool {
	var marker interface{ noRetry() }
	return errors.As(err, &marker)
}

// SetNoRetry tells the client that repeating this request cannot succeed.
// Errors are dropped: a lost signal costs a retry, never a response.
func SetNoRetry(ctx context.Context) {
	_ = grpc.SetTrailer(ctx, noRetryTrailer)
}

// IsNoRetry reports whether md carries a pushback value that stops gRPC from
// retrying: a single negative or unparsable value, or any count but one.
func IsNoRetry(md metadata.MD) bool {
	values := md.Get(metadataKey)
	switch len(values) {
	case 0:
		return false
	case 1:
		ms, err := strconv.Atoi(values[0])
		return err != nil || ms < 0
	default:
		return true
	}
}
