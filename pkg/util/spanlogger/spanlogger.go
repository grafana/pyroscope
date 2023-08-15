// SPDX-License-Identifier: AGPL-3.0-only

package spanlogger

import (
	"context"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/spanlogger"
	"github.com/grafana/dskit/tenant"
)

const (
	// TenantIDsTagName is the tenant IDs tag name.
	TenantIDsTagName = spanlogger.TenantIDsTagName
)

// defaultTenantResolver is used to include tenant IDs in spans automatically from the context.
var defaultTenantResolver = tenant.NewMultiResolver()

// SpanLogger unifies tracing and logging, to reduce repetition.
type SpanLogger = spanlogger.SpanLogger

// NewWithLogger is like New but allows to pass a logger.
func NewWithLogger(ctx context.Context, logger log.Logger, method string, kvps ...interface{}) (*SpanLogger, context.Context) {
	return spanlogger.New(ctx, logger, method, defaultTenantResolver, kvps...)
}

// FromContext returns a SpanLogger using the current parent span.
// If there is no parent span, the SpanLogger will only log to the logger
// within the context. If the context doesn't have a logger, the fallback
// logger is used.
func FromContext(ctx context.Context, fallback log.Logger) *SpanLogger {
	return spanlogger.FromContext(ctx, fallback, defaultTenantResolver)
}
