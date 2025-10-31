package fsm

import (
	"context"

	"github.com/opentracing/opentracing-go"
	"go.etcd.io/bbolt"
)

// tracingTx wraps a BoltDB transaction to automatically trace transaction lifecycle.
// It holds a span that encompasses the entire transaction, providing visibility into
// transaction timing without requiring manual instrumentation.
//
// The span should be created by the caller and will be finished when the transaction
// is committed or rolled back.
type tracingTx struct {
	*bbolt.Tx
	span    opentracing.Span
	spanCtx context.Context // Context with the span, for child operations
}

// newTracingTx creates a tracing transaction wrapper.
// The span parameter can be nil if no tracing is desired (e.g., on follower nodes).
func newTracingTx(tx *bbolt.Tx, span opentracing.Span, spanCtx context.Context) *tracingTx {
	return &tracingTx{
		Tx:      tx,
		span:    span,
		spanCtx: spanCtx,
	}
}

// Commit commits the transaction and finishes the span.
func (t *tracingTx) Commit() error {
	if t.span != nil {
		defer t.span.Finish()
		t.span.LogKV("operation", "commit")
	}
	return t.Tx.Commit()
}

// Rollback rolls back the transaction and finishes the span.
func (t *tracingTx) Rollback() error {
	if t.span != nil {
		defer t.span.Finish()
		t.span.LogKV("operation", "rollback")
	}
	return t.Tx.Rollback()
}
