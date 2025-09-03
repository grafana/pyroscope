package block

import (
	"context"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/grafana/dskit/multierror"
	"github.com/oklog/ulid/v2"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/block/metadata"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/util/bufferpool"
	"github.com/grafana/pyroscope/pkg/util/refctr"
)

// TODO Next:
//  - Separate storages for segments and compacted blocks.
//  - Local cache? Useful for all-in-one deployments.
//  - Distributed cache.

// Object represents a block or a segment in the object storage.
type Object struct {
	path    string
	meta    *metastorev1.BlockMeta
	storage objstore.BucketReader
	local   *objstore.ReadOnlyFile

	refs refctr.Counter
	buf  *bufferpool.Buffer
	err  error

	memSize     int
	downloadDir string
}

type ObjectOption func(*Object)

func WithObjectPath(path string) ObjectOption {
	return func(obj *Object) {
		obj.path = path
	}
}

func WithObjectMaxSizeLoadInMemory(size int) ObjectOption {
	return func(obj *Object) {
		obj.memSize = size
	}
}

func WithObjectDownload(dir string) ObjectOption {
	return func(obj *Object) {
		obj.downloadDir = dir
	}
}

func NewObjectFromPath(ctx context.Context, storage objstore.Bucket, path string, opts ...ObjectOption) (*Object, error) {
	attrs, err := storage.Attributes(ctx, path)
	if err != nil {
		return nil, err
	}

	defaultSize := int64(1 << 8) // 18)
	offset := attrs.Size - defaultSize
	if offset < 0 {
		offset = 0
	}
	size := attrs.Size - offset

	buf := bufferpool.GetBuffer(int(size))
	if err := objstore.ReadRange(ctx, buf, path, storage, offset, size); err != nil {
		return nil, err
	}
	if size < 8 {
		return nil, errors.New("invalid object too small")
	}

	metaSize := int64(binary.BigEndian.Uint32(buf.B[len(buf.B)-8:len(buf.B)-4])) + 8
	if metaSize > size {
		offset = attrs.Size - metaSize

		bufNew := bufferpool.GetBuffer(int(metaSize))
		if err := objstore.ReadRange(ctx, bufNew, path, storage, offset, metaSize-size); err != nil {
			return nil, err
		}
		bufNew.B = append(bufNew.B, buf.B...)
		buf = bufNew

	}

	var meta metastorev1.BlockMeta
	if err := metadata.Decode(buf.B, &meta); err != nil {
		return nil, err
	}
	meta.Size = uint64(attrs.Size)

	opts = append(opts, WithObjectPath(path))
	return NewObject(storage, &meta, opts...), nil
}

func NewObject(storage objstore.Bucket, md *metastorev1.BlockMeta, opts ...ObjectOption) *Object {
	o := &Object{
		storage: storage,
		meta:    md,
		path:    ObjectPath(md),
		memSize: defaultObjectSizeLoadInMemory,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

func ObjectPath(md *metastorev1.BlockMeta) string {
	return BuildObjectPath(metadata.Tenant(md), md.Shard, md.CompactionLevel, md.Id)
}

func BuildObjectDir(tenant string, shard uint32) string {
	topLevel := DirNameBlock
	tenantDirName := tenant
	if tenant == "" {
		topLevel = DirNameSegment
		tenantDirName = DirNameAnonTenant
	}
	var b strings.Builder
	b.WriteString(topLevel)
	b.WriteByte('/')
	b.WriteString(strconv.Itoa(int(shard)))
	b.WriteByte('/')
	b.WriteString(tenantDirName)
	b.WriteByte('/')
	return b.String()
}

func BuildObjectPath(tenant string, shard uint32, level uint32, block string) string {
	topLevel := DirNameBlock
	tenantDirName := tenant
	if level == 0 {
		topLevel = DirNameSegment
		tenantDirName = DirNameAnonTenant
	}
	var b strings.Builder
	b.WriteString(topLevel)
	b.WriteByte('/')
	b.WriteString(strconv.Itoa(int(shard)))
	b.WriteByte('/')
	b.WriteString(tenantDirName)
	b.WriteByte('/')
	b.WriteString(block)
	b.WriteByte('/')
	b.WriteString(FileNameDataObject)
	return b.String()
}

func MetadataDLQObjectPath(md *metastorev1.BlockMeta) string {
	var b strings.Builder
	tenantDirName := DirNameAnonTenant
	if md.CompactionLevel > 0 {
		tenantDirName = metadata.Tenant(md)
	}
	b.WriteString(DirNameDLQ)
	b.WriteByte('/')
	b.WriteString(strconv.Itoa(int(md.Shard)))
	b.WriteByte('/')
	b.WriteString(tenantDirName)
	b.WriteByte('/')
	b.WriteString(md.Id)
	b.WriteByte('/')
	b.WriteString(FileNameMetadataObject)
	return b.String()
}

func ParseBlockIDFromPath(path string) (ulid.ULID, error) {
	tokens := strings.Split(path, "/")
	if len(tokens) < 2 {
		return ulid.ULID{}, fmt.Errorf("invalid path format: %s", path)
	}
	blockID, err := ulid.Parse(tokens[len(tokens)-2])
	if err != nil {
		return ulid.ULID{}, fmt.Errorf("expected ULID: %s: %w", path, err)
	}
	return blockID, nil
}

// Open opens the object, loading the data into memory if it's small enough.
//
// Open may be called multiple times concurrently, but the
// object is only initialized once. While it is possible to open
// the object repeatedly after close, the caller must pass the
// failure reason to the "CloseWithError" call, preventing further
// use, if applicable.
func (obj *Object) Open(ctx context.Context) error {
	return obj.refs.IncErr(func() error {
		return obj.open(ctx)
	})
}

func (obj *Object) open(ctx context.Context) (err error) {
	if obj.err != nil {
		// In case if the object has been already closed with an error,
		// and then released, return the error immediately.
		return obj.err
	}
	if len(obj.meta.Datasets) == 0 {
		return nil
	}
	// Estimate the size of the sections to process, and load the
	// data into memory, if it's small enough.
	if obj.meta.Size > uint64(obj.memSize) {
		// Otherwise, download the object to the local directory,
		// if it's specified, and use the local file.
		if obj.downloadDir != "" {
			return obj.Download(ctx)
		}
		// The object will be read from the storage directly.
		return nil
	}
	obj.buf = bufferpool.GetBuffer(int(obj.meta.Size))
	defer func() {
		if err != nil {
			_ = obj.closeErr(err)
		}
	}()
	if err = objstore.ReadRange(ctx, obj.buf, obj.path, obj.storage, 0, int64(obj.meta.Size)); err != nil {
		return fmt.Errorf("loading object into memory %s: %w", obj.path, err)
	}
	return nil
}

func (obj *Object) Close() error {
	return obj.CloseWithError(nil)
}

// CloseWithError closes the object, releasing all the acquired resources,
// once the last reference is released. If the provided error is not nil,
// the object will be marked as failed, preventing any further use.
func (obj *Object) CloseWithError(err error) (closeErr error) {
	obj.refs.Dec(func() {
		closeErr = obj.closeErr(err)
	})
	return closeErr
}

func (obj *Object) closeErr(err error) (closeErr error) {
	obj.err = err
	if obj.buf != nil {
		bufferpool.Put(obj.buf)
		obj.buf = nil
	}
	if obj.local != nil {
		closeErr = obj.local.Close()
		obj.local = nil
	}
	return closeErr
}

func (obj *Object) Download(ctx context.Context) error {
	dir := filepath.Join(obj.downloadDir, obj.meta.Id)
	local, err := objstore.Download(ctx, obj.path, obj.storage, dir)
	if err != nil {
		return err
	}
	obj.storage = local
	obj.local = local
	return nil
}

func (obj *Object) Metadata() *metastorev1.BlockMeta { return obj.meta }

func (obj *Object) SetMetadata(md *metastorev1.BlockMeta) { obj.meta = md }

// ReadMetadata fetches the full block metadata from the storage.
// It the object does not include the metadata offset, the method
// returns the metadata entry the object was opened with.
func (obj *Object) ReadMetadata(ctx context.Context) (*metastorev1.BlockMeta, error) {
	if obj.meta.MetadataOffset == 0 {
		return obj.meta, nil
	}
	offset := int64(obj.meta.MetadataOffset)
	size := int64(obj.meta.Size) - offset
	buf := bufferpool.GetBuffer(int(size))
	defer bufferpool.Put(buf)
	if err := objstore.ReadRange(ctx, buf, obj.path, obj.storage, offset, size); err != nil {
		return nil, fmt.Errorf("reading block metadata %s: %w", obj.path, err)
	}
	var meta metastorev1.BlockMeta
	if err := metadata.Decode(buf.B, &meta); err != nil {
		return nil, fmt.Errorf("decoding block metadata %s: %w", obj.path, err)
	}
	// Size is not stored in the metadata, so we need to preserve it.
	meta.Size = obj.meta.Size
	return &meta, nil
}

func (obj *Object) IsNotExists(err error) bool {
	return objstore.IsNotExist(obj.storage, err)
}

// ObjectsFromMetas binds block metas to corresponding objects in the storage.
func ObjectsFromMetas(storage objstore.Bucket, blocks []*metastorev1.BlockMeta, options ...ObjectOption) Objects {
	objects := make([]*Object, len(blocks))
	for i, m := range blocks {
		objects[i] = NewObject(storage, m, options...)
	}
	return objects
}

type Objects []*Object

func (s Objects) Open(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	for i := range s {
		i := i
		g.Go(util.RecoverPanic(func() error {
			return s[i].Open(ctx)
		}))
	}
	return g.Wait()
}

func (s Objects) Close() error {
	var m multierror.MultiError
	for i := range s {
		m.Add(s[i].Close())
	}
	return m.Err()
}
