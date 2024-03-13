package ingester

import (
	"context"
	"encoding/json"
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

	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/phlaredb/shipper"
	diskutil "github.com/grafana/pyroscope/pkg/util/disk"
)

const (
	// TODO(kolesnikovae): Unify with pkg/phlaredb.
	phlareDBLocalPath = "local"
)

// newDiskCleaner creates a service that will intermittently clean blocks from
// disk.
func newDiskCleaner(logger log.Logger, evictor blockEvictor, policy retentionPolicy, cfg phlaredb.Config) *diskCleaner {
	dc := &diskCleaner{
		logger:        logger,
		policy:        policy,
		config:        cfg,
		blockManager:  newFSBlockManager(cfg.DataPath, evictor, newFS()),
		volumeChecker: diskutil.NewVolumeChecker(policy.MinFreeDisk*1024*1024*1024, policy.MinDiskAvailablePercentage),
		stop:          make(chan struct{}),
	}
	dc.Service = services.NewBasicService(nil, dc.running, dc.stopping)

	return dc
}

// newFSBlockManager creates a component that can manage blocks on a file system.
func newFSBlockManager(root string, evictor blockEvictor, fs fileSystem) fsBlockManager {
	return &realFSBlockManager{
		Root:    root,
		Evictor: evictor,
		FS:      fs,
	}
}

// newFS creates a file system implementation that interacts directly with the
// OS file system.
func newFS() fileSystem {
	return &realFS{}
}

func defaultRetentionPolicy() retentionPolicy {
	return retentionPolicy{
		MinFreeDisk:                phlaredb.DefaultMinFreeDisk,
		MinDiskAvailablePercentage: phlaredb.DefaultMinDiskAvailablePercentage,
		EnforcementInterval:        phlaredb.DefaultRetentionPolicyEnforcementInterval,
		Expiry:                     phlaredb.DefaultRetentionExpiry,
	}
}

type retentionPolicy struct {
	MinFreeDisk                uint64
	MinDiskAvailablePercentage float64
	EnforcementInterval        time.Duration
	Expiry                     time.Duration
}

// diskCleaner monitors disk usage and cleans unused data.
type diskCleaner struct {
	services.Service

	logger        log.Logger
	config        phlaredb.Config
	policy        retentionPolicy
	blockManager  fsBlockManager
	volumeChecker diskutil.VolumeChecker

	stop chan struct{}
	wg   sync.WaitGroup
}

func (dc *diskCleaner) running(ctx context.Context) error {
	dc.wg.Add(1)
	ticker := time.NewTicker(dc.policy.EnforcementInterval)
	defer func() {
		ticker.Stop()
		dc.wg.Done()
	}()

	var deleted int
	var bytesDeleted int
	var hasHighDiskUtilization bool
	for {
		deleted = dc.DeleteUploadedBlocks(ctx)
		level.Debug(dc.logger).Log("msg", "cleaned uploaded blocks", "count", deleted)

		deleted, bytesDeleted, hasHighDiskUtilization = dc.CleanupBlocksWhenHighDiskUtilization(ctx)
		if hasHighDiskUtilization {
			level.Debug(dc.logger).Log(
				"msg", "cleaned files after high disk utilization",
				"deleted_blocks", deleted,
				"deleted_bytes", bytesDeleted,
			)
		}

		select {
		case <-ticker.C:
		case <-ctx.Done():
			return nil
		case <-dc.stop:
			return nil
		}
	}
}

func (dc *diskCleaner) stopping(_ error) error {
	close(dc.stop)
	dc.wg.Wait()
	return nil
}

// DeleteUploadedBlocks scans and deletes blocks on all tenants that have
// already been uploaded. It returns the number of blocks deleted.
func (dc *diskCleaner) DeleteUploadedBlocks(ctx context.Context) int {
	tenantIDs, err := dc.blockManager.GetTenantIDs(ctx)
	if err != nil {
		level.Error(dc.logger).Log(
			"msg", "failed to delete uploaded blocks, could not read tenant ids",
			"err", err,
		)
		return 0
	}

	var deleted int
	for _, tenantID := range tenantIDs {
		blocks, err := dc.blockManager.GetBlocksForTenant(ctx, tenantID)
		if err != nil {
			level.Error(dc.logger).Log(
				"msg", "failed to delete uploaded blocks, could not get blocks for tenant",
				"err", err,
				"tenantID", tenantID,
			)
			continue
		}

		for _, block := range blocks {
			if !dc.isBlockDeletable(block) {
				continue
			}

			err = dc.blockManager.DeleteBlock(ctx, block)
			switch {
			case os.IsNotExist(err):
				level.Warn(dc.logger).Log(
					"msg", "failed to delete uploaded block, does not exist",
					"err", err,
					"path", block.Path,
				)
			case err != nil:
				level.Error(dc.logger).Log(
					"msg", "failed to delete uploaded block",
					"err", err,
					"path", block.Path,
				)
			default:
				deleted++
			}
		}
	}
	return deleted
}

// CleanupBlocksWhenHighDiskUtilization will run more aggressive disk cleaning
// if high disk utilization is detected by deleting blocks that have been
// uploaded but may not necessarily have been synced with the store gateway. It
// returns true if high disk utilization was detected, along with the number of
// files deleted and the estimated bytes recovered. If no high disk utilization
// was detected, false is returned.
func (dc *diskCleaner) CleanupBlocksWhenHighDiskUtilization(ctx context.Context) (int, int, bool) {
	volumeStats, err := dc.volumeChecker.HasHighDiskUtilization(dc.config.DataPath)
	if err != nil {
		level.Error(dc.logger).Log(
			"msg", "failed run high disk cleanup, failed to check disk utilization",
			"err", err,
		)
		return 0, 0, false
	}

	// Not in high disk utilization, nothing to do.
	if !volumeStats.HighDiskUtilization {
		return 0, 0, false
	}
	originalBytesAvailable := volumeStats.BytesAvailable

	tenantIDs, err := dc.blockManager.GetTenantIDs(ctx)
	if err != nil {
		level.Error(dc.logger).Log(
			"msg", "failed run high disk cleanup, could not read tenant ids",
			"err", err,
		)
		return 0, 0, true
	}

	blocks := make([]*tenantBlock, 0)
	for _, tenantID := range tenantIDs {
		tenantBlocks, err := dc.blockManager.GetBlocksForTenant(ctx, tenantID)
		if err != nil {
			level.Error(dc.logger).Log(
				"msg", "failed to get blocks for tenant",
				"tenantID", tenantID,
				"err", err,
			)

			// Keep trying to read blocks from other tenants.
			continue
		}

		blocks = append(blocks, tenantBlocks...)
	}

	// Sort by uploaded, then age (oldest first).
	sort.Sort(blocksByUploadAndAge(blocks))

	prevVolumeStats := &diskutil.VolumeStats{}
	filesDeleted := 0
	for _, block := range blocks {
		if !dc.isBlockDeletable(block) {
			continue
		}

		// Delete a block.
		err = dc.blockManager.DeleteBlock(ctx, block)
		switch {
		case os.IsNotExist(err):
			level.Warn(dc.logger).Log(
				"msg", "failed to delete block, does not exist",
				"err", err,
				"path", block.Path,
			)
			return filesDeleted, int(volumeStats.BytesAvailable - originalBytesAvailable), true
		case err != nil:
			level.Error(dc.logger).Log(
				"msg", "failed run high disk cleanup, could not delete block",
				"path", block.Path,
				"err", err,
			)
			return filesDeleted, int(volumeStats.BytesAvailable - originalBytesAvailable), true
		default:
			filesDeleted++
		}

		// Recheck volume stats.
		prevVolumeStats = volumeStats
		volumeStats, err = dc.volumeChecker.HasHighDiskUtilization(dc.config.DataPath)
		if err != nil {
			level.Error(dc.logger).Log(
				"msg", "failed to check disk utilization",
				"err", err,
			)
			break
		}

		if !volumeStats.HighDiskUtilization {
			// No longer in high disk utilization.
			break
		}

		if prevVolumeStats.BytesAvailable >= volumeStats.BytesAvailable {
			// Disk utilization has not been lowered since the last block was
			// deleted. There may be a delay in VolumeChecker reporting disk
			// utilization. In an effort to be conservative when deleting
			// blocks, stop the clean up now and wait for the next cycle to let
			// VolumeChecker catch up on the current state of the disk.
			level.Warn(dc.logger).Log("msg", "disk utilization is not lowered by deletion of a block, pausing until next cycle")
			break
		}
	}

	return filesDeleted, int(volumeStats.BytesAvailable - originalBytesAvailable), true
}

// isBlockDeletable returns true if this block can be deleted.
func (dc *diskCleaner) isBlockDeletable(block *tenantBlock) bool {
	// TODO(kolesnikovae):
	//  Expiry defaults to -querier.query-store-after which should be deprecated,
	//  blocks-storage.bucket-store.ignore-blocks-within can be used instead.
	expiryTs := time.Now().Add(-dc.policy.Expiry)
	return block.Uploaded && ulid.Time(block.ID.Time()).Before(expiryTs)
}

// blocksByUploadAndAge implements sorting tenantBlock by uploaded then by age
// in ascending order.
type blocksByUploadAndAge []*tenantBlock

func (b blocksByUploadAndAge) Len() int      { return len(b) }
func (b blocksByUploadAndAge) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b blocksByUploadAndAge) Less(i, j int) bool {
	switch {
	case b[i].Uploaded == b[j].Uploaded:
		return b[i].ID.Compare(b[j].ID) < 0
	case b[i].Uploaded:
		return !b[j].Uploaded
	case b[j].Uploaded:
		fallthrough
	default:
		return b[i].Uploaded
	}
}

// blockEvictor unloads blocks from tenant instance.
type blockEvictor interface {
	// evictBlock evicts the block by its ID from the memory and
	// invokes fn callback, regardless of if the tenant is found.
	// The call is thread-safe: tenant can't be added or removed
	// during the execution.
	evictBlock(tenant string, b ulid.ULID, fn func() error) error
}

type fileSystem interface {
	fs.ReadDirFS
	RemoveAll(name string) error
}

type realFS struct{}

func (*realFS) Open(name string) (fs.File, error)          { return os.Open(name) }
func (*realFS) ReadDir(name string) ([]fs.DirEntry, error) { return os.ReadDir(name) }
func (*realFS) RemoveAll(path string) error                { return os.RemoveAll(path) }

type tenantBlock struct {
	ID       ulid.ULID
	TenantID string
	Path     string
	Uploaded bool
}

func (t *tenantBlock) String() string {
	return t.ID.String()
}

type fsBlockManager interface {
	GetTenantIDs(ctx context.Context) ([]string, error)
	GetBlocksForTenant(ctx context.Context, tenantID string) ([]*tenantBlock, error)
	DeleteBlock(ctx context.Context, block *tenantBlock) error
}

type realFSBlockManager struct {
	Root    string
	Evictor blockEvictor
	FS      fileSystem
}

func (bm *realFSBlockManager) GetTenantIDs(ctx context.Context) ([]string, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	dirs, err := fs.ReadDir(bm.FS, bm.Root)
	if err != nil {
		return nil, err
	}

	tenantIDs := make([]string, 0)
	for _, dir := range dirs {
		if !bm.isTenantDir(bm.Root, dir) {
			continue
		}

		tenantIDs = append(tenantIDs, dir.Name())
	}
	return tenantIDs, nil
}

func (bm *realFSBlockManager) GetBlocksForTenant(ctx context.Context, tenantID string) ([]*tenantBlock, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	localDirPath := filepath.Join(bm.Root, tenantID, phlareDBLocalPath)
	blockDirs, err := fs.ReadDir(bm.FS, localDirPath)
	if err != nil {
		return nil, err
	}

	shipperPath := filepath.Join(localDirPath, shipper.MetaFilename)
	bytes, err := fs.ReadFile(bm.FS, shipperPath)
	if err != nil {
		return nil, err
	}

	var meta shipper.Meta
	err = json.Unmarshal(bytes, &meta)
	if err != nil {
		return nil, err
	}

	uploadedBlockIDs := make(map[ulid.ULID]struct{}, len(meta.Uploaded))
	for _, id := range meta.Uploaded {
		uploadedBlockIDs[id] = struct{}{}
	}

	// Read blocks.
	blocks := make([]*tenantBlock, 0)
	for _, blockDir := range blockDirs {
		if !blockDir.IsDir() {
			continue
		}

		path := filepath.Join(localDirPath, blockDir.Name())
		blockID, ok := block.IsBlockDir(path)
		if !ok {
			// A malformed/invalid ULID likely means that the directory is not a
			// valid block, ignoring.
			continue
		}

		_, uploaded := uploadedBlockIDs[blockID]
		blocks = append(blocks, &tenantBlock{
			ID:       blockID,
			TenantID: tenantID,
			Path:     path,
			Uploaded: uploaded,
		})
	}
	return blocks, nil
}

func (bm *realFSBlockManager) DeleteBlock(ctx context.Context, block *tenantBlock) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return bm.Evictor.evictBlock(block.TenantID, block.ID, func() error {
		err := bm.FS.RemoveAll(block.Path)
		switch {
		case os.IsNotExist(err):
			return err
		case err != nil:
			return fmt.Errorf("failed to delete block: %q: %w", block.Path, err)
		}
		return nil
	})
}

// isTenantDir checks if a directory is a tenant directory.
func (bm *realFSBlockManager) isTenantDir(path string, entry fs.DirEntry) bool {
	if !entry.IsDir() {
		return false
	}

	subEntries, err := bm.FS.ReadDir(filepath.Join(path, entry.Name()))
	if err != nil {
		return false
	}

	foundLocalDir := false
	for _, subEntry := range subEntries {
		if !subEntry.IsDir() {
			continue
		}

		if subEntry.Name() == phlareDBLocalPath {
			foundLocalDir = true
			break
		}
	}
	return foundLocalDir
}
