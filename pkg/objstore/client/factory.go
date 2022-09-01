package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v2"

	"github.com/thanos-io/objstore"
	"github.com/thanos-io/objstore/client"
	"github.com/thanos-io/objstore/providers/azure"
	"github.com/thanos-io/objstore/providers/bos"
	"github.com/thanos-io/objstore/providers/cos"
	"github.com/thanos-io/objstore/providers/gcs"
	"github.com/thanos-io/objstore/providers/oci"
	"github.com/thanos-io/objstore/providers/oss"
	"github.com/thanos-io/objstore/providers/s3"
	"github.com/thanos-io/objstore/providers/swift"

	fireobjstore "github.com/grafana/fire/pkg/objstore"
	"github.com/grafana/fire/pkg/objstore/client/parquet"
	"github.com/grafana/fire/pkg/objstore/providers/filesystem"
)

// NewBucket initializes and returns new object storage clients.
// NOTE: confContentYaml can contain secrets.
func NewBucket(logger log.Logger, confContentYaml []byte, reg prometheus.Registerer, component string) (fireobjstore.InstrumentedBucket, error) {
	level.Info(logger).Log("msg", "loading bucket configuration")
	bucketConf := &client.BucketConfig{}
	if err := yaml.UnmarshalStrict(confContentYaml, bucketConf); err != nil {
		return nil, errors.Wrap(err, "parsing config YAML file")
	}

	config, err := yaml.Marshal(bucketConf.Config)
	if err != nil {
		return nil, errors.Wrap(err, "marshal content of bucket configuration")
	}

	var bucket objstore.Bucket
	switch strings.ToUpper(string(bucketConf.Type)) {
	case string(client.GCS):
		bucket, err = gcs.NewBucket(context.Background(), logger, config, component)
	case string(client.S3):
		bucket, err = s3.NewBucket(logger, config, component)
	case string(client.AZURE):
		bucket, err = azure.NewBucket(logger, config, component)
	case string(client.SWIFT):
		bucket, err = swift.NewContainer(logger, config)
	case string(client.COS):
		bucket, err = cos.NewBucket(logger, config, component)
	case string(client.ALIYUNOSS):
		bucket, err = oss.NewBucket(logger, config, component)
	case string(client.FILESYSTEM):
		bucket, err = filesystem.NewBucketFromConfig(config)
	case string(client.BOS):
		bucket, err = bos.NewBucket(logger, config, component)
	case string(client.OCI):
		bucket, err = oci.NewBucket(logger, config)
	default:
		return nil, errors.Errorf("bucket with type %s is not supported", bucketConf.Type)
	}
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("create %s client", bucketConf.Type))
	}
	return ReaderAtBucket(bucketConf.Prefix, bucket, reg)
}

type readerAtBucket struct {
	objstore.InstrumentedBucket
	bkt fireobjstore.Bucket
}

type readerAt struct {
	objstore.InstrumentedBucket
	size int64
	name string
	ctx  context.Context
}

func ReaderAtBucket(prefix string, b objstore.Bucket, reg prometheus.Registerer) (fireobjstore.InstrumentedBucket, error) {
	// Prefer to use custom fireobjstore.BucketReaderAt implenentation if possible.
	if b, ok := b.(fireobjstore.Bucket); ok {
		if prefix != "" {
			b = fireobjstore.BucketWithPrefix(b, prefix)
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

func (b *readerAtBucket) ReaderAt(ctx context.Context, name string) (r fireobjstore.ReaderAt, err error) {
	if b.bkt != nil {
		// use fireobjstore.BucketReaderAt if possible.
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

func (b *readerAtBucket) WithExpectedErrs(expectedFunc objstore.IsOpFailureExpectedFunc) fireobjstore.Bucket {
	if ib, ok := b.InstrumentedBucket.WithExpectedErrs(expectedFunc).(objstore.InstrumentedBucket); ok {
		return &readerAtBucket{
			InstrumentedBucket: ib,
			bkt:                b.bkt,
		}
	}
	return b
}

func (b *readerAtBucket) ReaderWithExpectedErrs(expectedFunc objstore.IsOpFailureExpectedFunc) fireobjstore.BucketReader {
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
