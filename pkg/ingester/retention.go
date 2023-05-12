package ingester

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"
	"github.com/oklog/ulid"

	"github.com/grafana/phlare/pkg/phlaredb"
	"github.com/grafana/phlare/pkg/phlaredb/block"
	diskutil "github.com/grafana/phlare/pkg/util/disk"
)

const (
	defaultMinFreeDisk                        = 10 * 1024 * 1024 * 1024 // 10Gi
	defaultMinDiskAvailablePercentage         = 0.05
	defaultRetentionPolicyEnforcementInterval = 5 * time.Minute

	// TODO(kolesnikovae): Unify with pkg/phlaredb.
	phlareDBLocalPath = "local"
)

type retentionPolicy struct {
	MinFreeDisk                uint64
	MinDiskAvailablePercentage float64
	EnforcementInterval        time.Duration
}

func defaultRetentionPolicy() retentionPolicy {
	return retentionPolicy{
		MinFreeDisk:                defaultMinFreeDisk,
		MinDiskAvailablePercentage: defaultMinDiskAvailablePercentage,
		EnforcementInterval:        defaultRetentionPolicyEnforcementInterval,
	}
}

type retentionPolicyEnforcer struct {
	services.Service

	logger          log.Logger
	retentionPolicy retentionPolicy
	blockEvicter    blockEvicter
	dbConfig        phlaredb.Config
	fileSystem      fileSystem
	volumeChecker   diskutil.VolumeChecker

	stopCh chan struct{}
	wg     sync.WaitGroup
}

type tenantBlock struct {
	ulid     ulid.ULID
	tenantID string
	path     string
}

type fileSystem interface {
	fs.ReadDirFS
	RemoveAll(string) error
}

type realFileSystem struct{}

func (*realFileSystem) Open(name string) (fs.File, error)          { return os.Open(name) }
func (*realFileSystem) ReadDir(name string) ([]fs.DirEntry, error) { return os.ReadDir(name) }
func (*realFileSystem) RemoveAll(path string) error                { return os.RemoveAll(path) }

// blockEvicter unloads blocks from tenant instance.
type blockEvicter interface {
	// evictBlock evicts the block by its ID for the given tenant and invokes
	// fn callback, if the tenant is found. The call is thread-safe: tenant
	// can't be added or removed during the execution.
	evictBlock(tenant string, b ulid.ULID, fn func() error) error
}

func newRetentionPolicyEnforcer(logger log.Logger, blockEvicter blockEvicter, retentionPolicy retentionPolicy, dbConfig phlaredb.Config) *retentionPolicyEnforcer {
	e := retentionPolicyEnforcer{
		logger:          logger,
		blockEvicter:    blockEvicter,
		retentionPolicy: retentionPolicy,
		dbConfig:        dbConfig,
		stopCh:          make(chan struct{}),
		fileSystem:      new(realFileSystem),
		volumeChecker:   diskutil.NewVolumeChecker(retentionPolicy.MinFreeDisk, retentionPolicy.MinDiskAvailablePercentage),
	}
	e.Service = services.NewBasicService(nil, e.running, e.stopping)
	return &e
}

func (e *retentionPolicyEnforcer) running(ctx context.Context) error {
	e.wg.Add(1)
	retentionPolicyEnforcerTicker := time.NewTicker(e.retentionPolicy.EnforcementInterval)
	defer func() {
		retentionPolicyEnforcerTicker.Stop()
		e.wg.Done()
	}()
	for {
		// Enforce retention policy immediately at start.
		level.Debug(e.logger).Log("msg", "enforcing retention policy")
		if err := e.cleanupBlocksWhenHighDiskUtilization(ctx); err != nil {
			level.Error(e.logger).Log("msg", "failed to enforce retention policy", "err", err)
		}
		select {
		case <-retentionPolicyEnforcerTicker.C:
		case <-ctx.Done():
			return nil
		case <-e.stopCh:
			return nil
		}
	}
}

func (e *retentionPolicyEnforcer) stopping(_ error) error {
	close(e.stopCh)
	e.wg.Wait()
	return nil
}

func (e *retentionPolicyEnforcer) localBlocks(dir string) ([]*tenantBlock, error) {
	blocks := make([]*tenantBlock, 0, 32)
	tenants, err := fs.ReadDir(e.fileSystem, dir)
	if err != nil {
		return nil, err
	}
	var blockDirs []fs.DirEntry
	for _, tenantDir := range tenants {
		if !tenantDir.IsDir() {
			continue
		}
		tenantID := tenantDir.Name()
		tenantDirPath := filepath.Join(dir, tenantID, phlareDBLocalPath)
		if blockDirs, err = fs.ReadDir(e.fileSystem, tenantDirPath); err != nil {
			if os.IsNotExist(err) {
				// Must be created by external means, skipping.
				continue
			}
			return nil, err
		}
		for _, blockDir := range blockDirs {
			if !blockDir.IsDir() {
				continue
			}
			blockPath := filepath.Join(tenantDirPath, blockDir.Name())
			if blockID, ok := block.IsBlockDir(blockPath); ok {
				blocks = append(blocks, &tenantBlock{
					ulid:     blockID,
					path:     blockPath,
					tenantID: tenantID,
				})
			}
			// A malformed/invalid ULID likely means that the
			// directory is not a valid block, ignoring.
		}
	}

	// Sort the blocks by their id, which will be the time they've been created.
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].ulid.Compare(blocks[j].ulid) < 0
	})

	return blocks, nil
}

func (e *retentionPolicyEnforcer) cleanupBlocksWhenHighDiskUtilization(ctx context.Context) error {
	var volumeStatsPrev *diskutil.VolumeStats
	volumeStatsCurrent, err := e.volumeChecker.HasHighDiskUtilization(e.dbConfig.DataPath)
	if err != nil {
		return err
	}
	// Not in high disk utilization, nothing to do.
	if !volumeStatsCurrent.HighDiskUtilization {
		return nil
	}
	// Get all block across all the tenants. Any block
	// produced or imported during the procedure is ignored.
	blocks, err := e.localBlocks(e.dbConfig.DataPath)
	if err != nil {
		return err
	}

	for volumeStatsCurrent.HighDiskUtilization && len(blocks) > 0 && ctx.Err() == nil {
		// When disk utilization is not lower since the last loop, we end the
		// cleanup there to avoid deleting all blocks when disk usage reporting
		// is delayed.
		if volumeStatsPrev != nil && volumeStatsPrev.BytesAvailable >= volumeStatsCurrent.BytesAvailable {
			level.Warn(e.logger).Log("msg", "disk utilization is not lowered by deletion of a block, pausing until next cycle")
			break
		}
		// Delete the oldest block.
		var b *tenantBlock
		b, blocks = blocks[0], blocks[1:]
		level.Warn(e.logger).Log("msg", "disk utilization is high, deleting the oldest block", "path", b.path)
		if err = e.deleteBlock(b); err != nil {
			return err
		}
		volumeStatsPrev = volumeStatsCurrent
		if volumeStatsCurrent, err = e.volumeChecker.HasHighDiskUtilization(e.dbConfig.DataPath); err != nil {
			return err
		}
	}

	return ctx.Err()
}

func (e *retentionPolicyEnforcer) deleteBlock(b *tenantBlock) error {
	return e.blockEvicter.evictBlock(b.tenantID, b.ulid, func() error {
		switch err := e.fileSystem.RemoveAll(b.path); {
		case err == nil:
		case os.IsNotExist(err):
			level.Warn(e.logger).Log("msg", "block not found on disk", "path", b.path)
		default:
			return fmt.Errorf("failed to delete block %q: %w", b.path, err)
		}
		return nil
	})
}
