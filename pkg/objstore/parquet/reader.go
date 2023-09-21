package parquet

import (
	"context"
	"errors"
	"fmt"
	"sync"

	phlareobjstore "github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
)

// bufferPool is a pool of bytes.Buffers.
var bufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, 0, 32*1024)
		return &buf
	},
}

type optimizedReaderAt struct {
	phlareobjstore.ReaderAtCloser
	meta block.File

	footerCache *[]byte
	footerLock  sync.RWMutex
	footerLen   uint64
}

// NewOptimizedReader returns a reader that optimizes the reading of the parquet file.
func NewOptimizedReader(r phlareobjstore.ReaderAtCloser, meta block.File) phlareobjstore.ReaderAtCloser {
	var footerLen uint64

	// as long as we don't keep the exact footer sizes in the meta estimate it
	if meta.SizeBytes > 0 {
		footerLen = meta.SizeBytes / uint64(10000)
	}

	// set a minimum footer size of 32KiB
	if footerLen < 32*1024 {
		footerLen = 32 * 1024
	}

	// set a maximum footer size of 512KiB
	if footerLen > 512*1024 {
		footerLen = 512 * 1024
	}

	// now check clamp it to the actual size of the whole object
	if footerLen > meta.SizeBytes {
		footerLen = meta.SizeBytes
	}

	return &optimizedReaderAt{
		ReaderAtCloser: r,
		meta:           meta,
		footerLen:      footerLen,
	}
}

// // called by parquet-go in OpenFile() to set offset and length of footer section
// func (r *optimizedReaderAt) SetFooterSection(offset, length int64) {
// 	// todo cache footer section
// }

// // called by parquet-go in OpenFile() to set offset and length of column indexes
// func (r *optimizedReaderAt) SetColumnIndexSection(offset, length int64) {
// 	// todo cache column index section
// }

// // called by parquet-go in OpenFile() to set offset and length of offset index section
// func (r *optimizedReaderAt) SetOffsetIndexSection(offset, length int64) {
// 	// todo cache offset index section
// }

const magic = "PAR1"

// note cache needs to be held to call this method
func (r *optimizedReaderAt) serveFromCache(p []byte, off int64) (int, error) {
	if r.footerCache == nil {
		return 0, errors.New("footerCache is nil")
	}
	// recalculate offset to start at the cache
	off = off - int64(r.meta.SizeBytes) + int64(r.footerLen)
	return copy(p, (*r.footerCache)[int(off):int(off)+len(p)]), nil
}

func (r *optimizedReaderAt) clearFooterCache() {
	r.footerLock.Lock()
	defer r.footerLock.Unlock()
	if r.footerCache != nil {
		bufferPool.Put(r.footerCache)
		r.footerCache = nil
	}

}

func (r *optimizedReaderAt) Close() (err error) {
	r.clearFooterCache()
	return r.ReaderAtCloser.Close()
}

func (r *optimizedReaderAt) ReadAt(p []byte, off int64) (int, error) {
	// handle magic header
	if len(p) == 4 && off == 0 {
		return copy(p, []byte(magic)), nil
	}

	// check if the call falls into the footer
	if off >= int64(r.meta.SizeBytes)-int64(r.footerLen) {
		// check if the cache exists
		r.footerLock.RLock()
		cacheExists := r.footerCache != nil && len(*r.footerCache) == int(r.footerLen)
		if cacheExists {
			defer r.footerLock.RUnlock()
			return r.serveFromCache(p, off)
		}
		r.footerLock.RUnlock()

		// no valid cache found, create one under write lock
		r.footerLock.Lock()
		defer r.footerLock.Unlock()

		// check again if cache has been populated in the meantime
		cacheExists = r.footerCache != nil && len(*r.footerCache) == int(r.footerLen)
		if cacheExists {
			return r.serveFromCache(p, off)
		}

		// populate cache
		if r.footerCache == nil {
			r.footerCache = bufferPool.Get().(*[]byte)
		}
		if cap(*r.footerCache) < int(r.footerLen) {
			// grow the buffer if it is too small
			buf := make([]byte, int(r.footerLen))
			r.footerCache = &buf
		} else {
			// reuse the buffer if it is big enough
			*r.footerCache = (*r.footerCache)[:r.footerLen]
		}

		if n, err := r.ReaderAtCloser.ReadAt(*r.footerCache, int64(r.meta.SizeBytes)-int64(r.footerLen)); err != nil {
			// return to pool
			bufferPool.Put(r.footerCache)
			r.footerCache = nil
			return 0, err
		} else if n != int(r.footerLen) { // check if we got the expected amount of bytes
			// return to pool
			bufferPool.Put(r.footerCache)
			r.footerCache = nil
			return 0, fmt.Errorf("unexpected read length, expected=%d actual=%d", r.footerLen, n)
		}

		return r.serveFromCache(p, off)
	}

	// anything else will just read through the optimizer
	return r.ReaderAtCloser.ReadAt(p, off)
}

// OptimizedBucketReaderAt uses a bucket reader and wraps the optimized reader. Must not be used with non-parquet files.
func OptimizedBucketReaderAt(bucketReader phlareobjstore.BucketReader, ctx context.Context, meta block.File) (phlareobjstore.ReaderAtCloser, error) {
	rc, err := bucketReader.ReaderAt(ctx, meta.RelPath)
	if err != nil {
		return nil, err
	}
	return NewOptimizedReader(rc, meta), nil
}
