package log

import (
	"context"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/trace"
)

func Logger(ctx context.Context) logrus.FieldLogger {
	logger := logrus.New()
	if spanCtx := trace.SpanContextFromContext(ctx); spanCtx.IsValid() {
		return logger.WithFields(logrus.Fields{
			"trace_id": spanCtx.TraceID().String(),
			"span_id":  spanCtx.SpanID().String(),
		})
	}
	return logger
}
