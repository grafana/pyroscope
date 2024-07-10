package compactionworker

import (
	"context"
	"io"
	"strconv"

	"github.com/parquet-go/parquet-go"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
)

const (
	segmentDirPath    = "segments/"
	blockDirPath      = "blocks/"
	anonTenantDirName = "anon"
)

const (
	// Table of contents sections.
	// Version-specific.

	profileTableSectionIdx = iota
	tsdbSectionIdx
	symdbSectionIdx
)

// Object represents a block or a segment in the object storage.
// TODO: Better naming?
// TODO: Prefetch small objects into memory.
type storageObject struct {
	storage objstore.Bucket
	path    string
	meta    *metastorev1.BlockMeta
}

func newObject(storage objstore.Bucket, meta *metastorev1.BlockMeta) storageObject {
	return storageObject{
		storage: storage,
		path:    objectPath(meta),
		meta:    meta,
	}
}

func objectPath(md *metastorev1.BlockMeta) string {
	topLevel := blockDirPath
	tenantDirName := md.TenantId
	if md.CompactionLevel == 0 {
		topLevel = segmentDirPath
		tenantDirName = anonTenantDirName
	}
	return topLevel + strconv.Itoa(int(md.Shard)) + "/" + tenantDirName + "/" + md.Id + "/block.bin"
}

func (storageObject) sectionOffsetByIndex(svc *metastorev1.TenantService, idx int) int64 {
	return int64(svc.TableOfContents[idx])
}

func (b storageObject) sectionSizeByIndex(svc *metastorev1.TenantService, idx int) int64 {
	off := b.sectionOffsetByIndex(svc, idx)
	var next uint64
	if idx == len(svc.TableOfContents)-1 {
		next = svc.Size + svc.TableOfContents[0]
	} else {
		next = svc.TableOfContents[idx+1]
	}
	return int64(next) - off
}

func (b storageObject) sectionReaderByIndex(
	ctx context.Context,
	svc *metastorev1.TenantService,
	idx int,
) (io.ReadCloser, error) {
	return b.storage.GetRange(ctx, b.path,
		b.sectionOffsetByIndex(svc, idx),
		b.sectionSizeByIndex(svc, idx))
}

func (b storageObject) openTsdb(ctx context.Context, svc *metastorev1.TenantService) (*index.Reader, error) {
	r, err := b.sectionReaderByIndex(ctx, svc, tsdbSectionIdx)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, b.sectionSizeByIndex(svc, tsdbSectionIdx))
	if _, err = io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return index.NewReader(index.RealByteSlice(buf))
}

func (b storageObject) openSymdb(ctx context.Context, svc *metastorev1.TenantService) (*symdb.Reader, error) {
	offset := b.sectionOffsetByIndex(svc, symdbSectionIdx)
	size := b.sectionSizeByIndex(svc, symdbSectionIdx)
	reader := objstore.NewBucketReaderWithOffset(b.storage, offset)
	symbols, err := symdb.OpenObject(ctx, reader, b.path, size,
		symdb.WithPrefetchSize(32<<10))
	if err != nil {
		return nil, err
	}
	return symbols, nil
}

func (b storageObject) openProfileTable(ctx context.Context, svc *metastorev1.TenantService) (*parquet.File, error) {
	offset := b.sectionOffsetByIndex(svc, profileTableSectionIdx)
	size := b.sectionSizeByIndex(svc, profileTableSectionIdx)
	rat := &objstore.ReaderAt{
		Context:        ctx,
		GetRangeReader: b.storage,
		Name:           b.path,
		Offset:         offset,
	}
	return parquet.OpenFile(rat, size) // TODO: options, etc.
}
