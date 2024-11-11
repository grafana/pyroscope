package block

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/grafana/dskit/multierror"
	"golang.org/x/sync/errgroup"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/util/bufferpool"
	"github.com/grafana/pyroscope/pkg/util/refctr"
)

type Section uint32

const (
	// Table of contents sections.
	_ Section = iota
	SectionProfiles
	SectionTSDB
	SectionSymbols
)

var allSections = []Section{
	SectionProfiles,
	SectionTSDB,
	SectionSymbols,
}

var (
	// Version-specific.
	sectionNames   = [...][]string{1: {"invalid", "profiles", "tsdb", "symbols"}}
	sectionIndices = [...][]int{1: {-1, 0, 1, 2}}
)

func (sc Section) open(ctx context.Context, s *Dataset) (err error) {
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

func NewObject(storage objstore.Bucket, meta *metastorev1.BlockMeta, opts ...ObjectOption) *Object {
	o := &Object{
		storage: storage,
		meta:    meta,
		path:    ObjectPath(meta),
		memSize: defaultObjectSizeLoadInMemory,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

func ObjectPath(md *metastorev1.BlockMeta) string {
	topLevel := DirPathBlock
	tenantDirName := md.TenantId
	if md.CompactionLevel == 0 {
		topLevel = DirPathSegment
		tenantDirName = DirNameAnonTenant
	}
	var b strings.Builder
	b.WriteString(topLevel)
	b.WriteString(strconv.Itoa(int(md.Shard)))
	b.WriteByte('/')
	b.WriteString(tenantDirName)
	b.WriteByte('/')
	b.WriteString(md.Id)
	b.WriteByte('/')
	b.WriteString(FileNameDataObject)
	return b.String()
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

func (obj *Object) Meta() *metastorev1.BlockMeta { return obj.meta }

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
