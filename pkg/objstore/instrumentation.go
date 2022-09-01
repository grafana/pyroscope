package objstore

import (
	"context"

	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/thanos-io/objstore"
	"github.com/thanos-io/objstore/tracing"
)

// InstrumentedBucket is a Bucket with optional instrumentation control on reader.
type InstrumentedBucket interface {
	Bucket

	// WithExpectedErrs allows to specify a filter that marks certain errors as expected, so it will not increment
	// thanos_objstore_bucket_operation_failures_total metric.
	WithExpectedErrs(objstore.IsOpFailureExpectedFunc) Bucket

	// ReaderWithExpectedErrs allows to specify a filter that marks certain errors as expected, so it will not increment
	// thanos_objstore_bucket_operation_failures_total metric.
	// TODO(bwplotka): Remove this when moved to Go 1.14 and replace with InstrumentedBucketReader.
	ReaderWithExpectedErrs(objstore.IsOpFailureExpectedFunc) BucketReader
}

// TracingBucket includes bucket operations in the traces.
type TracingBucket struct {
	objstore.InstrumentedBucket
	bkt Bucket
}

func NewTracingBucket(bkt Bucket) InstrumentedBucket {
	return TracingBucket{
		InstrumentedBucket: objstore.NewTracingBucket(bkt),
		bkt:                bkt,
	}
}

func (t TracingBucket) ReaderAt(ctx context.Context, name string) (r ReaderAt, err error) {
	tracing.DoWithSpan(ctx, "ReaderAt", func(spanCtx context.Context, span opentracing.Span) {
		span.LogKV("name", name)
		r, err = t.bkt.ReaderAt(spanCtx, name)
		if err != nil {
			span.LogKV("err", err)
		}
	})
	return
}

func (t TracingBucket) WithExpectedErrs(expectedFunc objstore.IsOpFailureExpectedFunc) Bucket {
	if ib, ok := t.InstrumentedBucket.WithExpectedErrs(expectedFunc).(objstore.InstrumentedBucket); ok {
		return &TracingBucket{
			InstrumentedBucket: ib,
			bkt:                t.bkt,
		}
	}
	return t
}

func (t TracingBucket) ReaderWithExpectedErrs(expectedFunc objstore.IsOpFailureExpectedFunc) BucketReader {
	return t.WithExpectedErrs(expectedFunc)
}

// TracingBucket includes bucket operations in the traces.
type MetricBucket struct {
	objstore.InstrumentedBucket
	bkt Bucket
}

func NewMetricBucket(bkt Bucket, reg prometheus.Registerer) InstrumentedBucket {
	name := bkt.Name()
	return MetricBucket{
		InstrumentedBucket: objstore.BucketWithMetrics(name, bkt, reg),
		bkt:                bkt,
	}
}

func (t MetricBucket) ReaderAt(ctx context.Context, name string) (r ReaderAt, err error) {
	return t.bkt.ReaderAt(ctx, name)
}

func (t MetricBucket) WithExpectedErrs(expectedFunc objstore.IsOpFailureExpectedFunc) Bucket {
	if ib, ok := t.InstrumentedBucket.WithExpectedErrs(expectedFunc).(objstore.InstrumentedBucket); ok {
		return &MetricBucket{
			InstrumentedBucket: ib,
			bkt:                t.bkt,
		}
	}
	return t
}

func (t MetricBucket) ReaderWithExpectedErrs(expectedFunc objstore.IsOpFailureExpectedFunc) BucketReader {
	return t.WithExpectedErrs(expectedFunc)
}
