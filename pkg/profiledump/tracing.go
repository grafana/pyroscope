package profiledump

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// startCaptureSpan measures admission through enqueue, independently of upload.
func startCaptureSpan(ctx context.Context) (context.Context, func(Outcome)) {
	ctx, span := otel.Tracer("pyroscope").Start(ctx, "Profile.Capture")
	return ctx, func(o Outcome) {
		span.SetAttributes(o.spanAttributes()...)
		span.End()
	}
}

func (o Outcome) spanAttributes() []attribute.KeyValue {
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
