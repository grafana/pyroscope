package filesystem

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	thanosobjstore "github.com/thanos-io/objstore"
	"github.com/thanos-io/objstore/providers/filesystem"
	"gopkg.in/yaml.v2"

	"github.com/grafana/fire/pkg/objstore"
)

var _ objstore.BucketReader = (*Bucket)(nil)

type Bucket struct {
	thanosobjstore.Bucket
	rootDir string
}

// NewBucketFromConfig returns a new filesystem.Bucket from config.
func NewBucketFromConfig(conf []byte) (*Bucket, error) {
	var c filesystem.Config
	if err := yaml.Unmarshal(conf, &c); err != nil {
		return nil, err
	}
	if c.Directory == "" {
		return nil, errors.New("missing directory for filesystem bucket")
	}
	return NewBucket(c.Directory)
}

// NewBucket returns a new filesystem.Bucket.
func NewBucket(rootDir string) (*Bucket, error) {
	b, err := filesystem.NewBucket(rootDir)
	if err != nil {
		return nil, err
	}
	return &Bucket{Bucket: b, rootDir: rootDir}, nil
}

func (b *Bucket) ReaderAt(ctx context.Context, filename string) (objstore.ReaderAt, error) {
	f, err := os.Open(filepath.Join(b.rootDir, filename))
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}

	return &bucketReaderAt{File: f, size: fi.Size()}, nil
}

type bucketReaderAt struct {
	*os.File
	size int64
}

func (b *bucketReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	// todo cache meta data
	return b.File.ReadAt(p, off)
}

func (b *bucketReaderAt) Size() int64 {
	return b.size
}
