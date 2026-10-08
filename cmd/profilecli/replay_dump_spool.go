package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/go-kit/log/level"
	"github.com/prometheus/prometheus/model/labels"
	"golang.org/x/sync/errgroup"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	phlareobj "github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/symdb"
)

// Bound simultaneous object-store reads and resident symbol partitions.
const replayDumpConcurrency = 4

type replayRecordWriter interface {
	WriteRecord(replayRecord) error
}

// replaySymbols owns one reference per partition for the lifetime of a dataset.
// Per-profile resolvers borrow these references, rather than fetching and
// releasing (and consequently evicting) the entire partition for every row.
// Only a single block worker accesses this cache.
type replaySymbols struct {
	source     symdb.SymbolsReader
	partitions map[uint64]symdb.PartitionReader
}

type borrowedReplayPartition struct{ symdb.PartitionReader }

func (borrowedReplayPartition) Release() {}

func (s *replaySymbols) Partition(ctx context.Context, id uint64) (symdb.PartitionReader, error) {
	p, ok := s.partitions[id]
	if !ok {
		var err error
		p, err = s.source.Partition(ctx, id)
		if err != nil {
			return nil, err
		}
		s.partitions[id] = p
	}
	return borrowedReplayPartition{p}, nil
}

func (s *replaySymbols) Close() {
	for _, p := range s.partitions {
		p.Release()
	}
}

type replayRecordOffset struct {
	timestamp int64
	offset    int64
	size      int64
	block     int
}

type replayCountingWriter struct {
	io.Writer
	n int64
}

func (w *replayCountingWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	w.n += int64(n)
	return n, err
}

type replaySpool struct {
	writer  *replayWriter
	counter *replayCountingWriter
	block   int
	records []replayRecordOffset
}

func (s *replaySpool) position() int64 {
	return s.counter.n + int64(s.writer.w.Buffered())
}

func (s *replaySpool) WriteRecord(rec replayRecord) error {
	offset := s.position()
	if err := s.writer.WriteRecord(rec); err != nil {
		return err
	}
	s.records = append(s.records, replayRecordOffset{
		timestamp: rec.TimestampNanos, offset: offset,
		size: s.position() - offset, block: s.block,
	})
	return nil
}

// dumpBlocks spools payloads to disk in parallel. Only fixed-size record
// offsets are retained in memory; assembly never decodes or recompresses pprof.
func dumpBlocks(ctx context.Context, bucket phlareobj.Bucket, blocks []*metastorev1.BlockMeta,
	matchers []*labels.Matcher, startNanos, endNanos int64, dst *replayWriter, tempDir string,
) (int, error) {
	dir, err := os.MkdirTemp(tempDir, ".replay-blocks-*")
	if err != nil {
		return 0, fmt.Errorf("failed to create block spool directory: %w", err)
	}
	defer os.RemoveAll(dir)

	paths := make([]string, len(blocks))
	indexes := make([][]replayRecordOffset, len(blocks))
	g, workerCtx := errgroup.WithContext(ctx)
	g.SetLimit(replayDumpConcurrency)
	for i, md := range blocks {
		g.Go(func() error {
			if err := workerCtx.Err(); err != nil {
				return err
			}
			paths[i] = filepath.Join(dir, fmt.Sprintf("%d.replay", i))
			index, err := spoolReplayBlock(workerCtx, bucket, md, matchers, startNanos, endNanos, paths[i], i)
			if err != nil {
				return fmt.Errorf("failed to dump block %s: %w", md.Id, err)
			}
			indexes[i] = index
			level.Debug(logger).Log("msg", "dumped block", "block", md.Id, "profiles", len(index))
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return 0, err
	}
	var records []replayRecordOffset
	for _, index := range indexes {
		records = append(records, index...)
	}
	// Stable ordering also makes equal timestamps deterministic, regardless of
	// the order in which workers finish.
	slices.SortStableFunc(records, func(a, b replayRecordOffset) int {
		if a.timestamp < b.timestamp {
			return -1
		}
		if a.timestamp > b.timestamp {
			return 1
		}
		return 0
	})
	if err := assembleReplay(ctx, paths, records, dst); err != nil {
		return 0, fmt.Errorf("failed to assemble replay: %w", err)
	}
	return len(records), nil
}

func spoolReplayBlock(ctx context.Context, bucket phlareobj.Bucket, md *metastorev1.BlockMeta,
	matchers []*labels.Matcher, startNanos, endNanos int64, path string, blockIndex int,
) ([]replayRecordOffset, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	counter := &replayCountingWriter{Writer: f}
	writer, err := newReplayWriter(counter, replayHeader{})
	if err != nil {
		return nil, err
	}
	spool := &replaySpool{writer: writer, counter: counter, block: blockIndex}
	if _, err := dumpBlock(ctx, bucket, md, matchers, startNanos, endNanos, spool); err != nil {
		return nil, err
	}
	if err := writer.Flush(); err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	return spool.records, nil
}

func assembleReplay(ctx context.Context, paths []string, records []replayRecordOffset, dst *replayWriter) error {
	// A small LRU bounds descriptors even for queries spanning thousands of
	// blocks, while keeping overlapping blocks open across adjacent records.
	type openFile struct {
		file *os.File
		used int
	}
	files := make(map[int]openFile)
	defer func() {
		for _, f := range files {
			_ = f.file.Close()
		}
	}()
	buf := make([]byte, 64<<10)
	for i, rec := range records {
		if err := ctx.Err(); err != nil {
			return err
		}
		f, ok := files[rec.block]
		if !ok {
			if len(files) == 32 {
				oldest, age := -1, i
				for id, cached := range files {
					if cached.used < age {
						oldest, age = id, cached.used
					}
				}
				if err := files[oldest].file.Close(); err != nil {
					return err
				}
				delete(files, oldest)
			}
			var err error
			f.file, err = os.Open(paths[rec.block])
			if err != nil {
				return err
			}
		}
		f.used = i
		files[rec.block] = f
		n, err := io.CopyBuffer(dst.w, io.NewSectionReader(f.file, rec.offset, rec.size), buf)
		if err != nil {
			return err
		}
		if n != rec.size {
			return io.ErrUnexpectedEOF
		}
	}
	return ctx.Err()
}
