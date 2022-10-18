package client

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/thanos-io/objstore"

	phlareobjstore "github.com/grafana/phlare/pkg/objstore"
	"github.com/grafana/phlare/pkg/objstore/client/parquet"
	"github.com/grafana/phlare/pkg/objstore/providers/azure"
	"github.com/grafana/phlare/pkg/objstore/providers/filesystem"
	"github.com/grafana/phlare/pkg/objstore/providers/gcs"
	"github.com/grafana/phlare/pkg/objstore/providers/s3"
	"github.com/grafana/phlare/pkg/objstore/providers/swift"
	phlarecontext "github.com/grafana/phlare/pkg/phlare/context"
)

// NewBucket creates a new bucket client based on the configured backend
func NewBucket(ctx context.Context, cfg Config, name string) (phlareobjstore.Bucket, error) {
	var (
		backendClient objstore.Bucket
		err           error
	)
	logger := phlarecontext.Logger(ctx)
	reg := phlarecontext.Registry(ctx)

	switch cfg.Backend {
	case S3:
		backendClient, err = s3.NewBucketClient(cfg.S3, name, logger)
	case GCS:
		backendClient, err = gcs.NewBucketClient(ctx, cfg.GCS, name, logger)
	case Azure:
		backendClient, err = azure.NewBucketClient(cfg.Azure, name, logger)
	case Swift:
		backendClient, err = swift.NewBucketClient(cfg.Swift, name, logger)
	case Filesystem:
		backendClient, err = filesystem.NewBucket(cfg.Filesystem.Directory)
	default:
		return nil, ErrUnsupportedStorageBackend
	}

	if err != nil {
		return nil, err
	}

	// Wrap the client with any provided middleware
	for _, wrap := range cfg.Middlewares {
		backendClient, err = wrap(backendClient)
		if err != nil {
			return nil, err
		}
	}

	return ReaderAtBucket(cfg.StoragePrefix, backendClient, reg)
}

type readerAtBucket struct {
	objstore.InstrumentedBucket
	bkt phlareobjstore.Bucket
}

type readerAt struct {
	objstore.InstrumentedBucket
	size int64
	name string
	ctx  context.Context
}

func ReaderAtBucket(prefix string, b objstore.Bucket, reg prometheus.Registerer) (phlareobjstore.InstrumentedBucket, error) {
	// Prefer to use custom phlareobjstore.BucketReaderAt implenentation if possible.
	if b, ok := b.(phlareobjstore.Bucket); ok {
		if prefix != "" {
			b = phlareobjstore.BucketWithPrefix(b, prefix)
		}

		return &readerAtBucket{
			InstrumentedBucket: objstore.NewTracingBucket(objstore.BucketWithMetrics(b.Name(), b, reg)),
			bkt:                b,
		}, nil
	}
	if prefix != "" {
		b = objstore.NewPrefixedBucket(b, prefix)
	}
	return &readerAtBucket{
		InstrumentedBucket: objstore.NewTracingBucket(objstore.BucketWithMetrics(b.Name(), b, reg)),
	}, nil
}

func (b *readerAtBucket) ReaderAt(ctx context.Context, name string) (r phlareobjstore.ReaderAt, err error) {
	if b.bkt != nil {
		// use phlareobjstore.BucketReaderAt if possible.
		r, err := b.bkt.ReaderAt(ctx, name)
		if err != nil {
			return nil, err
		}
		return parquet.NewReaderAt(r), nil
	}
	// fallback to using Attributes and GetRange
	attr, err := b.InstrumentedBucket.Attributes(ctx, name)
	if err != nil {
		return nil, err
	}
	return parquet.NewReaderAt(&readerAt{
		InstrumentedBucket: b.InstrumentedBucket,
		size:               attr.Size,
		name:               name,
		ctx:                ctx,
	}), nil
}

func (b *readerAtBucket) WithExpectedErrs(expectedFunc objstore.IsOpFailureExpectedFunc) phlareobjstore.Bucket {
	if ib, ok := b.InstrumentedBucket.WithExpectedErrs(expectedFunc).(objstore.InstrumentedBucket); ok {
		return &readerAtBucket{
			InstrumentedBucket: ib,
			bkt:                b.bkt,
		}
	}
	return b
}

func (b *readerAtBucket) ReaderWithExpectedErrs(expectedFunc objstore.IsOpFailureExpectedFunc) phlareobjstore.BucketReader {
	return b.WithExpectedErrs(expectedFunc)
}

func (b *readerAt) Close() error {
	return nil
}

func (b *readerAt) Size() int64 {
	return b.size
}

func (b *readerAt) ReadAt(p []byte, off int64) (n int, err error) {
	rc, err := b.InstrumentedBucket.GetRange(b.ctx, b.name, off, int64(len(p)))
	if err != nil {
		return -1, err
	}
	defer rc.Close()
	return rc.Read(p)
}
