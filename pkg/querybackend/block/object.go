package block

import (
	"bytes"
	"context"
	"fmt"
	"strconv"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/util/refctr"
)

// TODO Next:
//  - Buffer pool.
//  - In-memory threshold option.
//  - Store the object size in metadata.
//  - Separate storages for segments and compacted blocks.
//  - Local cache? Useful for all-in-one deployments.
//  - Distributed cache.

const (
	segmentDirPath    = "segments/"
	blockDirPath      = "blocks/"
	anonTenantDirName = "anon"
)

type Section uint32

const (
	// Table of contents sections.
	_ Section = iota
	SectionProfiles
	SectionTSDB
	SectionSymbols
)

var (
	// Version-specific.
	sectionNames   = [...][]string{1: {"invalid", "profiles", "tsdb", "symbols"}}
	sectionIndices = [...][]int{1: {-1, 0, 1, 2}}
)

func (sc Section) open(ctx context.Context, s *TenantService) (err error) {
	switch sc {
	case SectionTSDB:
		return openTSDB(ctx, s)
	case SectionSymbols:
		return openSymbols(ctx, s)
	case SectionProfiles:
		return openProfileTable(ctx, s)
	default:
		panic(fmt.Sprintf("bug: unknown section: %d", sc))
	}
}

const loadInMemorySizeThreshold = 1 << 20

// Object represents a block or a segment in the object storage.
type Object struct {
	storage objstore.Bucket
	path    string
	meta    *metastorev1.BlockMeta

	refs refctr.Counter
	buf  *bytes.Buffer
	err  error
}

type Option func(*Object)

func WithPath(path string) Option {
	return func(obj *Object) {
		obj.path = path
	}
}

func NewObject(storage objstore.Bucket, meta *metastorev1.BlockMeta, opts ...Option) *Object {
	o := &Object{
		storage: storage,
		meta:    meta,
	}
	for _, opt := range opts {
		opt(o)
	}
	if o.path == "" {
		o.path = ObjectPath(meta)
	}
	return o
}

func ObjectPath(md *metastorev1.BlockMeta) string {
	topLevel := blockDirPath
	tenantDirName := md.TenantId
	if md.CompactionLevel == 0 {
		topLevel = segmentDirPath
		tenantDirName = anonTenantDirName
	}
	return topLevel + strconv.Itoa(int(md.Shard)) + "/" + tenantDirName + "/" + md.Id + "/block.bin"
}

// OpenShared opens the object, loading the data into memory
// if it's small enough.
//
// OpenShared may be called multiple times concurrently, but the
// object is only initialized once. While it is possible to open
// the object repeatedly after close, the caller must pass the
// failure reason to the "CloseShared" call, preventing further
// use, if  applicable.
func (obj *Object) OpenShared(ctx context.Context) error {
	obj.err = obj.refs.Inc(func() error {
		return obj.Open(ctx)
	})
	return obj.err
}

func (obj *Object) Open(ctx context.Context) error {
	if obj.err != nil {
		// In case if the object has been already closed with an error,
		// and then released, return the error immediately.
		return obj.err
	}
	// Estimate the size of the sections to process, and load the
	// data into memory, if it's small enough.
	if len(obj.meta.TenantServices) == 0 {
		panic("bug: invalid block meta: at least one section is expected")
	}
	obj.buf = new(bytes.Buffer) // TODO: Take from pool.
	if err := objstore.FetchRange(ctx, obj.buf, obj.path, obj.storage, 0, int64(obj.meta.Size)); err != nil {
		return fmt.Errorf("loading object into memory: %w", err)
	}
	return nil
}

// CloseShared closes the object, releasing all the acquired resources,
// once the last reference is released. If the provided error is not nil,
// the object will be marked as failed, preventing any further use.
func (obj *Object) CloseShared(err error) {
	obj.refs.Dec(func() {
		obj.closeErr(err)
	})
}

func (obj *Object) Close() error {
	obj.closeErr(nil)
	return obj.err
}

func (obj *Object) closeErr(err error) {
	if obj.err == nil {
		obj.err = err
	}
	obj.buf = nil // TODO: Release.
}

func (obj *Object) Meta() *metastorev1.BlockMeta { return obj.meta }
