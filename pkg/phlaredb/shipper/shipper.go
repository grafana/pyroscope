// Copyright (c) The Thanos Authors.
// Licensed under the Apache License 2.0.

// Package shipper detects directories on the local file system and uploads
// them to a block storage.

// TODO: Fix attribution

package shipper

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/runutil"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/thanos-io/objstore"

	"github.com/grafana/phlare/pkg/phlaredb/block"
)

type metrics struct {
	dirSyncs          prometheus.Counter
	dirSyncFailures   prometheus.Counter
	uploads           prometheus.Counter
	uploadFailures    prometheus.Counter
	uploadedCompacted prometheus.Gauge
}

func newMetrics(reg prometheus.Registerer, uploadCompacted bool) *metrics {
	var m metrics

	m.dirSyncs = promauto.With(reg).NewCounter(prometheus.CounterOpts{
		Name: "pyroscope_shipper_dir_syncs_total",
		Help: "Total number of dir syncs",
	})
	m.dirSyncFailures = promauto.With(reg).NewCounter(prometheus.CounterOpts{
		Name: "pyroscope_shipper_dir_sync_failures_total",
		Help: "Total number of failed dir syncs",
	})
	m.uploads = promauto.With(reg).NewCounter(prometheus.CounterOpts{
		Name: "pyroscope_shipper_uploads_total",
		Help: "Total number of uploaded blocks",
	})
	m.uploadFailures = promauto.With(reg).NewCounter(prometheus.CounterOpts{
		Name: "pyroscope_shipper_upload_failures_total",
		Help: "Total number of block upload failures",
	})
	uploadCompactedGaugeOpts := prometheus.GaugeOpts{
		Name: "pyroscope_shipper_upload_compacted_done",
		Help: "If 1 it means shipper uploaded all compacted blocks from the filesystem.",
	}
	if uploadCompacted {
		m.uploadedCompacted = promauto.With(reg).NewGauge(uploadCompactedGaugeOpts)
	} else {
		m.uploadedCompacted = promauto.With(nil).NewGauge(uploadCompactedGaugeOpts)
	}
	return &m
}

// Shipper watches a directory for matching files and directories and uploads
// them to a remote data store.
type Shipper struct {
	logger      log.Logger
	metrics     *metrics
	bucket      objstore.Bucket
	blockLister BlockLister
	source      block.SourceType

	uploadCompacted        bool
	allowOutOfOrderUploads bool
}

// New creates a new shipper that detects new TSDB blocks in dir and uploads them to
// remote if necessary. It attaches the Thanos metadata section in each meta JSON file.
// If uploadCompacted is enabled, it also uploads compacted blocks which are already in filesystem.
func New(
	logger log.Logger,
	r prometheus.Registerer,
	blockLister BlockLister,
	bucket objstore.Bucket,
	source block.SourceType,
	uploadCompacted bool,
	allowOutOfOrderUploads bool,
) *Shipper {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	return &Shipper{
		logger:                 logger,
		blockLister:            blockLister,
		bucket:                 bucket,
		metrics:                newMetrics(r, uploadCompacted),
		source:                 source,
		allowOutOfOrderUploads: allowOutOfOrderUploads,
		uploadCompacted:        uploadCompacted,
	}
}

type BlockLister interface {
	LocalDataPath() string
	BlockMetas(ctx context.Context) ([]*block.Meta, error)
}

// Timestamps returns the minimum timestamp for which data is available and the highest timestamp
// of blocks that were successfully uploaded.
func (s *Shipper) Timestamps() (minTime, maxSyncTime model.Time, err error) {
	ctx := context.Background()
	meta, err := ReadMetaFile(s.blockLister.LocalDataPath())
	if err != nil {
		return 0, 0, errors.Wrap(err, "read shipper meta file")
	}
	// Build a map of blocks we already uploaded.
	hasUploaded := make(map[ulid.ULID]struct{}, len(meta.Uploaded))
	for _, id := range meta.Uploaded {
		hasUploaded[id] = struct{}{}
	}

	minTime = model.Time(math.MaxInt64)
	maxSyncTime = model.Time(math.MinInt64)

	metas, err := s.blockLister.BlockMetas(ctx)
	if err != nil {
		return 0, 0, err
	}
	for _, m := range metas {
		if m.MinTime < minTime {
			minTime = m.MinTime
		}
		if _, ok := hasUploaded[m.ULID]; ok && m.MaxTime > maxSyncTime {
			maxSyncTime = m.MaxTime
		}
	}

	if minTime == math.MaxInt64 {
		// No block yet found. We cannot assume any min block size so propagate 0 minTime.
		minTime = 0
	}
	return minTime, maxSyncTime, nil
}

type lazyOverlapChecker struct {
	synced bool
	logger log.Logger
	bucket objstore.Bucket

	metas       []tsdb.BlockMeta
	lookupMetas map[ulid.ULID]struct{}
}

func newLazyOverlapChecker(logger log.Logger, bucket objstore.Bucket) *lazyOverlapChecker {
	return &lazyOverlapChecker{
		logger: logger,
		bucket: bucket,

		lookupMetas: map[ulid.ULID]struct{}{},
	}
}

func (c *lazyOverlapChecker) sync(ctx context.Context) error {
	if err := c.bucket.Iter(ctx, "", func(path string) error {
		id, ok := block.IsBlockDir(path)
		if !ok {
			return nil
		}

		m, err := block.DownloadMeta(ctx, c.logger, c.bucket, id)
		if err != nil {
			return err
		}

		c.metas = append(c.metas, m.TSDBBlockMeta())
		c.lookupMetas[m.ULID] = struct{}{}
		return nil

	}); err != nil {
		return errors.Wrap(err, "get all block meta.")
	}

	c.synced = true
	return nil
}

func (c *lazyOverlapChecker) IsOverlapping(ctx context.Context, newMeta tsdb.BlockMeta) error {
	if !c.synced {
		level.Info(c.logger).Log("msg", "gathering all existing blocks from the remote bucket for check", "id", newMeta.ULID.String())
		if err := c.sync(ctx); err != nil {
			return err
		}
	}

	// TODO(bwplotka) so confusing! we need to sort it first. Add comment to TSDB code.
	metas := append([]tsdb.BlockMeta{newMeta}, c.metas...)
	sort.Slice(metas, func(i, j int) bool {
		return metas[i].MinTime < metas[j].MinTime
	})
	if o := tsdb.OverlappingBlocks(metas); len(o) > 0 {
		// TODO(bwplotka): Consider checking if overlaps relates to block in concern?
		return errors.Errorf("shipping compacted block %s is blocked; overlap spotted: %s", newMeta.ULID, o.String())
	}
	return nil
}

// Sync performs a single synchronization, which ensures all non-compacted local blocks have been uploaded
// to the object bucket once.
//
// If uploaded.
//
// It is not concurrency-safe, however it is compactor-safe (running concurrently with compactor is ok).
func (s *Shipper) Sync(ctx context.Context) (uploaded int, err error) {
	meta, err := ReadMetaFile(s.blockLister.LocalDataPath())
	if err != nil {
		// If we encounter any error, proceed with an empty meta file and overwrite it later.
		// The meta file is only used to avoid unnecessary bucket.Exists call,
		// which are properly handled by the system if their occur anyway.
		if !os.IsNotExist(err) {
			level.Warn(s.logger).Log("msg", "reading meta file failed, will override it", "err", err)
		}
		meta = &Meta{Version: MetaVersion1}
	}

	// Build a map of blocks we already uploaded.
	hasUploaded := make(map[ulid.ULID]struct{}, len(meta.Uploaded))
	for _, id := range meta.Uploaded {
		hasUploaded[id] = struct{}{}
	}

	// Reset the uploaded slice so we can rebuild it only with blocks that still exist locally.
	meta.Uploaded = nil

	var (
		checker    = newLazyOverlapChecker(s.logger, s.bucket)
		uploadErrs int
	)

	metas, err := s.blockLister.BlockMetas(ctx)
	if err != nil {
		return 0, err
	}
	for _, m := range metas {
		// Do not sync a block if we already uploaded or ignored it. If it's no longer found in the bucket,
		// it was generally removed by the compaction process.
		if _, uploaded := hasUploaded[m.ULID]; uploaded {
			meta.Uploaded = append(meta.Uploaded, m.ULID)
			continue
		}

		// We only ship of the first compacted block level as normal flow.
		if m.Compaction.Level > 1 {
			if !s.uploadCompacted {
				continue
			}
		}

		// Check against bucket if the meta file for this block exists.
		ok, err := s.bucket.Exists(ctx, path.Join(m.ULID.String(), block.MetaFilename))
		if err != nil {
			return 0, errors.Wrap(err, "check exists")
		}
		if ok {
			meta.Uploaded = append(meta.Uploaded, m.ULID)
			continue
		}

		// Skip overlap check if out of order uploads is enabled.
		if m.Compaction.Level > 1 && !s.allowOutOfOrderUploads {
			if err := checker.IsOverlapping(ctx, m.TSDBBlockMeta()); err != nil {
				return 0, errors.Errorf("Found overlap or error during sync, cannot upload compacted block, details: %v", err)
			}
		}

		if err := s.upload(ctx, m); err != nil {
			if !s.allowOutOfOrderUploads {
				return 0, errors.Wrapf(err, "upload %v", m.ULID)
			}

			// No error returned, just log line. This is because we want other blocks to be uploaded even
			// though this one failed. It will be retried on second Sync iteration.
			level.Error(s.logger).Log("msg", "shipping failed", "block", m.ULID, "err", err)
			uploadErrs++
			continue
		}
		meta.Uploaded = append(meta.Uploaded, m.ULID)
		uploaded++
		s.metrics.uploads.Inc()
	}
	if err := WriteMetaFile(s.logger, s.blockLister.LocalDataPath(), meta); err != nil {
		level.Warn(s.logger).Log("msg", "updating meta file failed", "err", err)
	}

	s.metrics.dirSyncs.Inc()
	if uploadErrs > 0 {
		s.metrics.uploadFailures.Add(float64(uploadErrs))
		return uploaded, errors.Errorf("failed to sync %v blocks", uploadErrs)
	}

	if s.uploadCompacted {
		s.metrics.uploadedCompacted.Set(1)
	}
	return uploaded, nil
}

// sync uploads the block if not exists in remote storage.
// TODO(khyatisoneji): Double check if block does not have deletion-mark.json for some reason, otherwise log it or return error.
func (s *Shipper) upload(ctx context.Context, meta *block.Meta) error {
	level.Info(s.logger).Log("msg", "upload new block", "id", meta.ULID)

	updir := filepath.Join(s.blockLister.LocalDataPath(), meta.ULID.String())

	meta.Source = s.source
	if _, err := meta.WriteToFile(s.logger, updir); err != nil {
		return errors.Wrap(err, "write meta file")
	}
	return block.Upload(ctx, s.logger, s.bucket, updir)
}

// Meta defines the format thanos.shipper.json file that the shipper places in the data directory.
type Meta struct {
	Version  int         `json:"version"`
	Uploaded []ulid.ULID `json:"uploaded"`
}

const (
	// MetaFilename is the known JSON filename for meta information.
	MetaFilename = "shipper.json"

	// MetaVersion1 represents 1 version of meta.
	MetaVersion1 = 1
)

// WriteMetaFile writes the given meta into <dir>/thanos.shipper.json.
func WriteMetaFile(logger log.Logger, dir string, meta *Meta) error {
	// Make any changes to the file appear atomic.
	path := filepath.Join(dir, MetaFilename)
	tmp := path + ".tmp"

	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "\t")

	if err := enc.Encode(meta); err != nil {
		runutil.CloseWithLogOnErr(logger, f, "write meta file close")
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return renameFile(logger, tmp, path)
}

// ReadMetaFile reads the given meta from <dir>/shipper.json.
func ReadMetaFile(dir string) (*Meta, error) {
	b, err := os.ReadFile(filepath.Join(dir, filepath.Clean(MetaFilename)))
	if err != nil {
		return nil, err
	}
	var m Meta

	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if m.Version != MetaVersion1 {
		return nil, errors.Errorf("unexpected meta file version %d", m.Version)
	}

	return &m, nil
}

func renameFile(logger log.Logger, from, to string) error {
	if err := os.RemoveAll(to); err != nil {
		return err
	}
	if err := os.Rename(from, to); err != nil {
		return err
	}

	// Directory was renamed; sync parent dir to persist rename.
	pdir, err := fileutil.OpenDir(filepath.Dir(to))
	if err != nil {
		return err
	}

	if err = fileutil.Fdatasync(pdir); err != nil {
		runutil.CloseWithLogOnErr(logger, pdir, "rename file dir close")
		return err
	}
	return pdir.Close()
}
