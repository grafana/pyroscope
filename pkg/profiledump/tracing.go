package profiledump

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// CaptureSpan traces preparation after selection/rate admission through enqueue
// or drop. It neither forces sampling nor measures upload.
func CaptureSpan(tracer trace.Tracer) func(context.Context) (context.Context, func(Outcome)) {
	return func(ctx context.Context) (context.Context, func(Outcome)) {
		ctx, span := tracer.Start(ctx, "Profile.Capture")
		return ctx, func(o Outcome) {
			span.SetAttributes(o.SpanAttributes()...)
			span.End()
		}
	}
}

func (o Outcome) SpanAttributes() []attribute.KeyValue {
	result := "dropped"
	if o.Enqueued {
		result = "enqueued"
	}
	return []attribute.KeyValue{
		attribute.String("capture.id", o.CaptureID),
		attribute.String("capture.object_key", o.ObjectKey),
		attribute.String("capture.format", string(o.Format)),
		attribute.String("capture.source", metricSource(o.SourceProtocol)),
		attribute.String("capture.distributor_id", o.DistributorID),
		attribute.Int64("capture.payload_size", o.PayloadSize),
		attribute.Int64("capture.object_size", o.Size),
		attribute.String("capture.result", result),
		attribute.String("capture.policy_fingerprint", o.PolicyFingerprint),
		attribute.String("capture.drop_reason", string(o.Reason)),
	}
}
