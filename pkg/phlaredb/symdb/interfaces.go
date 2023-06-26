package symdb

import (
	"context"

	schemasv1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
)

// Mapping is a binary that is part of the program during the profile
// collection. https://github.com/google/pprof/blob/main/proto/README.md
//
// In the package, Mapping represents all the version of a binary.

type MappingWriter interface {
	// StacktraceAppender provides exclusive write access
	// to the stack traces of the mapping.
	//
	// StacktraceAppender.Release must be called in order
	// to dispose the object and release the lock.
	// Released resolver must not be used.
	StacktraceAppender() StacktraceAppender
}

type MappingReader interface {
	// StacktraceResolver provides non-exclusive read
	// access to the stack traces of the mapping.
	//
	// StacktraceResolver.Release must be called in order
	// to dispose the object and release the lock.
	// Released resolver must not be used.
	StacktraceResolver() StacktraceResolver
}

type StacktraceAppender interface {
	// AppendStacktrace appends the stack traces into the mapping,
	// and writes the allocated identifiers into dst:
	// len(dst) must be equal to len(s).
	// The leaf is at locations[0].
	AppendStacktrace(dst []uint32, s []*schemasv1.Stacktrace)
	Release()
}

type StacktraceResolver interface {
	// ResolveStacktraces resolves locations for each stack trace
	// and inserts it to the StacktraceInserter provided.
	//
	// The stacktraces must be ordered in the ascending order.
	// If a stacktrace can't be resolved, dst receives an empty
	// array of locations.
	//
	// Stacktraces slice might be modified during the call.
	ResolveStacktraces(ctx context.Context, dst StacktraceInserter, stacktraces []uint32) error
	Release()
}

// StacktraceInserter accepts resolved locations for a given stack trace.
// The leaf is at locations[0].
//
// Locations slice must not be retained by implementation.
// It is guaranteed, that for a given stacktrace ID
// InsertStacktrace is called not more than once.
type StacktraceInserter interface {
	InsertStacktrace(stacktraceID uint32, locations []int32)
}

type StacktraceInserterFn func(stacktraceID uint32, locations []int32)

func (fn StacktraceInserterFn) InsertStacktrace(stacktraceID uint32, locations []int32) {
	fn(stacktraceID, locations)
}
