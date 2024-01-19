package ingester

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/samber/lo"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"

	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/phlaredb/shipper"
	diskutil "github.com/grafana/pyroscope/pkg/util/disk"
)

func TestDiskCleaner_DeleteUploadedBlocks(t *testing.T) {
	t.Run("multi_tenant_blocks", func(t *testing.T) {
		const anonTenantID = "anonymous"
		const tenantID = "1234"

		e := &mockBlockEvictor{}

		bm := &mockBlockManager{}
		bm.On("GetTenantIDs", mock.Anything).
			Return([]string{anonTenantID, tenantID}, nil).
			Once()
		bm.On("GetBlocksForTenant", mock.Anything, anonTenantID).
			Return([]*tenantBlock{{
				ID:       ulid.MustParse(generateBlockID(t, "01AC")),
				TenantID: anonTenantID,
				Path:     fmt.Sprintf("./data/%s/%s", anonTenantID, generateBlockID(t, "01AC")),
				Uploaded: true,
			}}, nil).
			Once()
		bm.On("GetBlocksForTenant", mock.Anything, tenantID).
			Return([]*tenantBlock{{
				ID:       ulid.MustParse(generateBlockID(t, "01AB")),
				TenantID: anonTenantID,
				Path:     fmt.Sprintf("./data/%s/%s", anonTenantID, generateBlockID(t, "01AB")),
				Uploaded: false,
			}}, nil)
		bm.On("DeleteBlock", mock.Anything, mock.Anything).
			Return(nil).
			Once()

		dc := newDiskCleaner(log.NewNopLogger(), e, defaultRetentionPolicy(), phlaredb.Config{
			DataPath: "./data",
		})
		dc.blockManager = bm

		want := 1
		got := dc.DeleteUploadedBlocks(context.Background())
		require.Equal(t, want, got)
	})

	t.Run("delete_blocks_past_expiry", func(t *testing.T) {
		// Two blocks are created and marked as "uploaded", but only one is past
		// the expiry window. Only the expired one should be deleted.

		const anonTenantID = "anonymous"
		entropy := rand.New(rand.NewSource(0))
		expiry := 10 * time.Minute

		nowMS := ulid.Timestamp(time.Now())
		nowID := ulid.MustNew(nowMS, entropy)

		expiredMS := ulid.Timestamp(time.Now().Add(-(2 * expiry))) // Twice as long ago as the expiry.
		expiredID := ulid.MustNew(expiredMS, entropy)

		e := &mockBlockEvictor{}

		bm := &mockBlockManager{}
		bm.On("GetTenantIDs", mock.Anything).
			Return([]string{anonTenantID}, nil).
			Once()
		bm.On("GetBlocksForTenant", mock.Anything, anonTenantID).
			Return([]*tenantBlock{
				{
					ID:       nowID,
					TenantID: anonTenantID,
					Path:     fmt.Sprintf("./data/%s/%s", anonTenantID, nowID.String()),
					Uploaded: true,
				},
				{
					ID:       expiredID,
					TenantID: anonTenantID,
					Path:     fmt.Sprintf("./data/%s/%s", anonTenantID, expiredID.String()),
					Uploaded: true,
				},
			}, nil).
			Once()
		bm.On("DeleteBlock", mock.Anything, mock.Anything).
			Return(nil).
			Once()

		policy := defaultRetentionPolicy()
		policy.Expiry = expiry

		dc := newDiskCleaner(log.NewNopLogger(), e, policy, phlaredb.Config{
			DataPath: "./data",
		})
		dc.blockManager = bm

		want := 1
		got := dc.DeleteUploadedBlocks(context.Background())
		require.Equal(t, want, got)
	})

	t.Run("no_tenant_dirs", func(t *testing.T) {
		e := &mockBlockEvictor{}

		bm := &mockBlockManager{}
		bm.On("GetTenantIDs", mock.Anything).
			Return([]string{}, nil).
			Once()

		dc := newDiskCleaner(log.NewNopLogger(), e, defaultRetentionPolicy(), phlaredb.Config{
			DataPath: "./data",
		})
		dc.blockManager = bm

		want := 0
		got := dc.DeleteUploadedBlocks(context.Background())
		require.Equal(t, want, got)
	})

	t.Run("no_block_dirs", func(t *testing.T) {
		const tenantID = "anonymous"

		e := &mockBlockEvictor{}

		bm := &mockBlockManager{}
		bm.On("GetTenantIDs", mock.Anything).
			Return([]string{tenantID}, nil).
			Once()
		bm.On("GetBlocksForTenant", mock.Anything, tenantID).
			Return([]*tenantBlock{}, nil).
			Once()

		dc := newDiskCleaner(log.NewNopLogger(), e, defaultRetentionPolicy(), phlaredb.Config{
			DataPath: "./data",
		})
		dc.blockManager = bm

		want := 0
		got := dc.DeleteUploadedBlocks(context.Background())
		require.Equal(t, want, got)
	})
}

func TestDiskCleaner_EnforceHighDiskUtilization(t *testing.T) {
	t.Run("no_high_disk", func(t *testing.T) {
		const anonTenantID = "anonymous"
		e := &mockBlockEvictor{}

		bm := &mockBlockManager{}
		bm.On("GetTenantIDs", mock.Anything).
			Return([]string{anonTenantID}, nil).
			Once()
		bm.On("GetBlocksForTenant", mock.Anything, anonTenantID).
			Return([]*tenantBlock{
				{
					ID:       ulid.MustParse(generateBlockID(t, "01AC")),
					TenantID: anonTenantID,
					Path:     fmt.Sprintf("/data/%s/local/%s", anonTenantID, generateBlockID(t, "01AC")),
					Uploaded: true,
				},
			}, nil).
			Once()
		bm.On("DeleteBlock", mock.Anything, mock.Anything).
			Return(nil)

		vc := &mockVolumeChecker{}
		vc.On("HasHighDiskUtilization", mock.Anything).
			Return(&diskutil.VolumeStats{
				HighDiskUtilization: false,
				BytesAvailable:      100,
				BytesTotal:          200,
			}, nil).
			Once()

		dc := newDiskCleaner(log.NewNopLogger(), e, defaultRetentionPolicy(), phlaredb.Config{
			DataPath: "./data",
		})
		dc.blockManager = bm
		dc.volumeChecker = vc

		deleted, bytesFreed, hadHighDisk := dc.CleanupBlocksWhenHighDiskUtilization(context.Background())
		require.Equal(t, 0, deleted)
		require.Equal(t, 0, bytesFreed)
		require.False(t, hadHighDisk)
	})

	t.Run("has_high_disk", func(t *testing.T) {
		const anonTenantID = "anonymous"

		e := &mockBlockEvictor{}

		bm := &mockBlockManager{}
		bm.On("GetTenantIDs", mock.Anything).
			Return([]string{anonTenantID}, nil).
			Once()
		bm.On("GetBlocksForTenant", mock.Anything, anonTenantID).
			Return([]*tenantBlock{
				{
					ID:       ulid.MustParse(generateBlockID(t, "01AC")),
					TenantID: anonTenantID,
					Path:     fmt.Sprintf("/data/%s/local/%s", anonTenantID, generateBlockID(t, "01AC")),
					Uploaded: true,
				},
				{
					ID:       ulid.MustParse(generateBlockID(t, "01AD")),
					TenantID: anonTenantID,
					Path:     fmt.Sprintf("/data/%s/local/%s", anonTenantID, generateBlockID(t, "01AD")),
					Uploaded: false,
				},
				{
					ID:       ulid.MustParse(generateBlockID(t, "01AE")),
					TenantID: anonTenantID,
					Path:     fmt.Sprintf("/data/%s/local/%s", anonTenantID, generateBlockID(t, "01AE")),
					Uploaded: false,
				},
			}, nil).
			Once()
		bm.On("DeleteBlock", mock.Anything, mock.Anything).
			Return(nil)

		vc := &mockVolumeChecker{}
		vc.On("HasHighDiskUtilization", mock.Anything).
			Return(&diskutil.VolumeStats{
				HighDiskUtilization: true,
				BytesAvailable:      0,
				BytesTotal:          200,
			}, nil).
			Once()
		vc.On("HasHighDiskUtilization", mock.Anything).
			Return(&diskutil.VolumeStats{
				HighDiskUtilization: true,
				BytesAvailable:      100,
				BytesTotal:          200,
			}, nil).
			Once() // Expect the loop to break after a single block delete (since the subsequent blocks aren't uploaded).

		dc := newDiskCleaner(log.NewNopLogger(), e, defaultRetentionPolicy(), phlaredb.Config{
			DataPath: "./data",
		})
		dc.blockManager = bm
		dc.volumeChecker = vc

		deleted, bytesFreed, hadHighDisk := dc.CleanupBlocksWhenHighDiskUtilization(context.Background())
		require.Equal(t, 1, deleted)
		require.Equal(t, 100, bytesFreed)
		require.True(t, hadHighDisk)
	})

	t.Run("has_high_disk_with_delayed_volume_checker_stats", func(t *testing.T) {
		const anonTenantID = "anonymous"

		e := &mockBlockEvictor{}

		bm := &mockBlockManager{}
		bm.On("GetTenantIDs", mock.Anything).
			Return([]string{anonTenantID}, nil).
			Once()
		bm.On("GetBlocksForTenant", mock.Anything, anonTenantID).
			Return([]*tenantBlock{
				{
					ID:       ulid.MustParse(generateBlockID(t, "01AC")),
					TenantID: anonTenantID,
					Path:     fmt.Sprintf("/data/%s/local/%s", anonTenantID, generateBlockID(t, "01AC")),
					Uploaded: true,
				},
				{
					ID:       ulid.MustParse(generateBlockID(t, "01AD")),
					TenantID: anonTenantID,
					Path:     fmt.Sprintf("/data/%s/local/%s", anonTenantID, generateBlockID(t, "01AD")),
					Uploaded: false,
				},
			}, nil).
			Once()
		bm.On("DeleteBlock", mock.Anything, mock.Anything).
			Return(nil)

		vc := &mockVolumeChecker{}
		vc.On("HasHighDiskUtilization", mock.Anything).
			Return(&diskutil.VolumeStats{
				HighDiskUtilization: true,
				BytesAvailable:      100,
				BytesTotal:          200,
			}, nil).
			Twice() // Report the same result twice, causing the loop to break.

		dc := newDiskCleaner(log.NewNopLogger(), e, defaultRetentionPolicy(), phlaredb.Config{
			DataPath: "./data",
		})
		dc.blockManager = bm
		dc.volumeChecker = vc

		deleted, bytesFreed, hadHighDisk := dc.CleanupBlocksWhenHighDiskUtilization(context.Background())
		require.Equal(t, 1, deleted)
		require.Equal(t, 0, bytesFreed)
		require.True(t, hadHighDisk)
	})
}

func TestDiskCleaner_isBlockDeletable(t *testing.T) {
	tests := []struct {
		Name   string
		Expiry time.Duration
		Block  *tenantBlock
		Want   bool
	}{
		{
			Name:   "uploaded_and_expired",
			Expiry: 10 * time.Minute,
			Block: &tenantBlock{
				ID:       generateBlockIDFromTS(t, time.Now().Add(-(11 * time.Minute))),
				Uploaded: true,
			},
			Want: true,
		},
		{
			Name:   "not_uploaded",
			Expiry: 10 * time.Minute,
			Block: &tenantBlock{
				ID:       generateBlockIDFromTS(t, time.Now().Add(-(11 * time.Minute))),
				Uploaded: false,
			},
			Want: false,
		},
		{
			Name:   "not_expired",
			Expiry: 10 * time.Minute,
			Block: &tenantBlock{
				ID:       generateBlockIDFromTS(t, time.Now().Add(-(9 * time.Minute))),
				Uploaded: true,
			},
			Want: false,
		},
		{
			Name:   "not_uploaded_and_not_expired",
			Expiry: 10 * time.Minute,
			Block: &tenantBlock{
				ID:       generateBlockIDFromTS(t, time.Now().Add(-(9 * time.Minute))),
				Uploaded: false,
			},
			Want: false,
		},
	}

	dc := &diskCleaner{
		policy: defaultRetentionPolicy(),
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			dc.policy.Expiry = tt.Expiry

			got := dc.isBlockDeletable(tt.Block)
			require.Equal(t, tt.Want, got)
		})
	}
}

func TestFSBlockManager(t *testing.T) {
	const root = "/data"
	blocksByTenant := map[string][]*tenantBlock{
		"anonymous": {
			{
				ID:       ulid.MustParse(generateBlockID(t, "01AC")),
				TenantID: "anonymous",
				Path:     "/data/anonymous/local/" + generateBlockID(t, "01AC"),
				Uploaded: false,
			},
			{
				ID:       ulid.MustParse(generateBlockID(t, "01AD")),
				TenantID: "anonymous",
				Path:     "/data/anonymous/local/" + generateBlockID(t, "01AD"),
				Uploaded: true,
			},
		},
		"1218": {
			{
				ID:       ulid.MustParse(generateBlockID(t, "11AC")),
				TenantID: "1218",
				Path:     "/data/1218/local/" + generateBlockID(t, "11AC"),
				Uploaded: false,
			},
			{
				ID:       ulid.MustParse(generateBlockID(t, "11AD")),
				TenantID: "1218",
				Path:     "/data/1218/local/" + generateBlockID(t, "11AD"),
				Uploaded: true,
			},
		},
	}

	e := &mockBlockEvictor{}

	fs := &mockFS{
		Fs:   afero.NewMemMapFs(),
		Root: root,
	}
	for tenantID, blocks := range blocksByTenant {
		blockIDs := lo.Map(blocks, func(block *tenantBlock, _ int) string {
			return block.ID.String()
		})
		fs.createBlocksForTenant(t, tenantID, blockIDs...)

		uploadedBlockIDs := lo.Map(lo.Filter(blocks, func(block *tenantBlock, _ int) bool {
			return block.Uploaded
		}), func(block *tenantBlock, _ int) string {
			return block.ID.String()
		})
		fs.markBlocksShippedForTenant(t, tenantID, uploadedBlockIDs...)
	}

	// Create a lost+found directory.
	fs.createDirectories(t, "lost+found")

	t.Run("GetTenantIDs", func(t *testing.T) {
		bm := newFSBlockManager(root, e, fs)
		tenantIDs, err := bm.GetTenantIDs(context.Background())
		require.NoError(t, err)
		require.Equal(t, []string{"1218", "anonymous"}, tenantIDs)
		// Explicitly check lost+found isn't in tenant id list.
		require.NotContains(t, tenantIDs, "lost+found")
	})

	t.Run("GetBlocksForTenant", func(t *testing.T) {
		bm := newFSBlockManager(root, e, fs)
		blocks, err := bm.GetBlocksForTenant(context.Background(), "anonymous")
		require.NoError(t, err)
		require.Equal(t, blocksByTenant["anonymous"], blocks)

		blocks, err = bm.GetBlocksForTenant(context.Background(), "1218")
		require.NoError(t, err)
		require.Equal(t, blocksByTenant["1218"], blocks)

		_, err = bm.GetBlocksForTenant(context.Background(), "missing")
		require.ErrorContains(t, err, "file does not exist")
	})

	t.Run("DeleteBlock", func(t *testing.T) {
		e = &mockBlockEvictor{}
		e.On("evictBlock", "anonymous", mock.Anything, mock.Anything).
			Return(nil)

		bm := newFSBlockManager(root, e, fs)
		for _, block := range blocksByTenant["anonymous"] {
			err := bm.DeleteBlock(context.Background(), block)
			require.NoError(t, err)
		}
	})
}

func TestFSBlockManager_isTenantDir(t *testing.T) {
	const root = "/data"
	dirPaths := []string{
		// Skip, not tenant ids
		"lost+found",
		".DS_Store",

		// Skip, no local dir
		"1234/head/01HKWWF79V1STKXBNYW7WCMDGM",
		"1234/head/01HKWWF8939QM6E7BS69X0RASG",

		// Tenant dirs
		"anonymous/local/01HKWWF3CTFC5EJN6JJ96TY4W9",
		"anonymous/local/01HKWWF4C298KVTEEQ3RW6TVHZ",
		"1218/local/01HKWWF5BB2DJVDP0DTMT9MDMN",
		"1218/local/01HKWWF6AKVZDCWQB12MHWG7FN",
		"9876/local",
	}
	filePaths := []string{
		// Skip all files
		"somefile.txt",
	}

	fs := &mockFS{
		Fs:   afero.NewMemMapFs(),
		Root: root,
	}
	fs.createDirectories(t, dirPaths...)
	fs.createFiles(t, filePaths...)

	gotTenantIDs := []string{}
	entries, err := fs.ReadDir(fs.Root)
	require.NoError(t, err)

	bm := &realFSBlockManager{
		Root: fs.Root,
		FS:   fs,
	}
	for _, entry := range entries {
		if bm.isTenantDir(fs.Root, entry) {
			gotTenantIDs = append(gotTenantIDs, entry.Name())
		}
	}
	slices.Sort(gotTenantIDs)

	wantTenantIDs := []string{"1218", "9876", "anonymous"}
	require.Equal(t, wantTenantIDs, gotTenantIDs)
}

func TestSortBlocks(t *testing.T) {
	createAnonymousBlock := func(t *testing.T, blockID string, uploaded bool) *tenantBlock {
		t.Helper()

		return &tenantBlock{
			ID:       ulid.MustParse(blockID),
			TenantID: "anonymous",
			Path:     fmt.Sprintf("/data/anonymous/local/%s", blockID),
			Uploaded: uploaded,
		}
	}

	tests := []struct {
		Name   string
		Blocks []*tenantBlock
		Want   []*tenantBlock
	}{
		{
			Name: "uploaded_and_non_uploaded",
			Blocks: []*tenantBlock{
				createAnonymousBlock(t, "01HH5BVHA006AFVGQT5ZYC0GEK", true),  // unix ms: 1702061000000
				createAnonymousBlock(t, "01HH5CT1W0ZW908PVKS1Q4ZYAZ", false), // unix ms: 1702062000000
				createAnonymousBlock(t, "01HH5DRJE0YSHABVQ85AYZ8JHD", true),  // unix ms: 1702063000000
				createAnonymousBlock(t, "01HH5EQ3001DTZP60DNX4AF7Q0", false), // unix ms: 1702064000000
				createAnonymousBlock(t, "01HH5FNKJ0P46KJHJHGM7X98BR", true),  // unix ms: 1702065000000
			},
			Want: []*tenantBlock{
				createAnonymousBlock(t, "01HH5BVHA006AFVGQT5ZYC0GEK", true),  // unix ms: 1702061000000
				createAnonymousBlock(t, "01HH5DRJE0YSHABVQ85AYZ8JHD", true),  // unix ms: 1702063000000
				createAnonymousBlock(t, "01HH5FNKJ0P46KJHJHGM7X98BR", true),  // unix ms: 1702065000000
				createAnonymousBlock(t, "01HH5CT1W0ZW908PVKS1Q4ZYAZ", false), // unix ms: 1702062000000
				createAnonymousBlock(t, "01HH5EQ3001DTZP60DNX4AF7Q0", false), // unix ms: 1702064000000
			},
		},
		{
			Name: "uploaded_and_non_uploaded_at_same_timestamp",
			Blocks: []*tenantBlock{
				createAnonymousBlock(t, "01HH5BVHA006AFVGQT5ZYC0GEK", false), // unix ms: 1702061000000
				createAnonymousBlock(t, "01HH5BVHA0ZW908PVKS1Q4ZYAZ", true),  // unix ms: 1702061000000
			},
			Want: []*tenantBlock{
				createAnonymousBlock(t, "01HH5BVHA0ZW908PVKS1Q4ZYAZ", true),  // unix ms: 1702061000000
				createAnonymousBlock(t, "01HH5BVHA006AFVGQT5ZYC0GEK", false), // unix ms: 1702061000000
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			sort.Sort(blocksByUploadAndAge(tt.Blocks))
			require.Equal(t, tt.Want, tt.Blocks)
		})
	}
}

type mockFS struct {
	afero.Fs

	Root string
}

func (mfs *mockFS) Open(name string) (fs.File, error) {
	return mfs.Fs.Open(name)
}

func (mfs *mockFS) ReadDir(name string) ([]fs.DirEntry, error) {
	dirs, err := afero.ReadDir(mfs.Fs, name)
	if err != nil {
		return nil, err
	}

	entries := make([]fs.DirEntry, 0, len(dirs))
	for _, dir := range dirs {
		entries = append(entries, fs.FileInfoToDirEntry(dir))
	}
	return entries, nil
}

func (mfs *mockFS) createBlocksForTenant(t *testing.T, tenantID string, blockIDs ...string) {
	t.Helper()
	localDirPath := filepath.Join(mfs.Root, tenantID, phlareDBLocalPath)
	for _, blockID := range blockIDs {
		path := filepath.Join(localDirPath, blockID)
		err := mfs.MkdirAll(path, 0755)
		if err != nil {
			t.Fatalf("failed to create block: %s: %v", localDirPath, err)
			return
		}
	}
}

func (mfs *mockFS) markBlocksShippedForTenant(t *testing.T, tenantID string, blockIDs ...string) {
	t.Helper()
	localDirPath := filepath.Join(mfs.Root, tenantID, phlareDBLocalPath)
	shipperPath := filepath.Join(localDirPath, shipper.MetaFilename)
	bytes, err := fs.ReadFile(mfs, shipperPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("failed to read shipper.json: %v", err)
		return
	}

	meta := shipper.Meta{}
	if len(bytes) != 0 {
		err = json.Unmarshal(bytes, &meta)
		if err != nil {
			t.Fatalf("failed to unmarshal shipper.json: %v", err)
			return
		}
	}

	for _, blockID := range blockIDs {
		id, err := ulid.Parse(blockID)
		if err != nil {
			t.Fatalf("failed to create ULID from %s: %v", blockID, err)
			return
		}
		meta.Uploaded = append(meta.Uploaded, id)
	}

	bytes, err = json.Marshal(meta)
	if err != nil {
		t.Fatalf("failed to marshal shipper.json: %v", err)
		return
	}
	err = afero.WriteFile(mfs.Fs, shipperPath, bytes, 0755)
	if err != nil {
		t.Fatalf("failed to update shipper.json: %v", err)
	}
}

func (mfs *mockFS) createDirectories(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		path = filepath.Join(mfs.Root, path)
		err := mfs.MkdirAll(path, 0755)
		if err != nil {
			t.Fatalf("failed to create directory: %s: %v", path, err)
			return
		}
	}
}

func (mfs *mockFS) createFiles(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		path = filepath.Join(mfs.Root, path)
		_, err := mfs.Create(path)
		if err != nil {
			t.Fatalf("failed to create file: %s: %v", path, err)
			return
		}
	}
}

type mockBlockManager struct {
	mock.Mock
}

func (bm *mockBlockManager) DeleteBlock(ctx context.Context, block *tenantBlock) error {
	args := bm.Called(ctx, block)
	return args.Error(0)
}

func (bm *mockBlockManager) GetBlocksForTenant(ctx context.Context, tenantID string) ([]*tenantBlock, error) {
	args := bm.Called(ctx, tenantID)
	return args[0].([]*tenantBlock), args.Error(1)
}

func (bm *mockBlockManager) GetTenantIDs(ctx context.Context) ([]string, error) {
	args := bm.Called(ctx)
	return args[0].([]string), args.Error(1)
}

type mockBlockEvictor struct {
	mock.Mock
}

func (e *mockBlockEvictor) evictBlock(tenant string, b ulid.ULID, fn func() error) error {
	args := e.Called(tenant, b, fn)

	err := fn()
	if err != nil {
		return err
	}

	return args.Error(0)
}

type mockVolumeChecker struct {
	mock.Mock
}

func (vc *mockVolumeChecker) HasHighDiskUtilization(path string) (*diskutil.VolumeStats, error) {
	args := vc.Called(path)
	return args[0].(*diskutil.VolumeStats), args.Error(1)
}

func generateBlockID(t *testing.T, prefix string) string {
	t.Helper()

	const maxLen = 26
	const padding = "0"
	return fmt.Sprintf("%s%s", prefix, strings.Repeat(padding, maxLen-len(prefix)))
}

func generateBlockIDFromTS(t *testing.T, ts time.Time) ulid.ULID {
	t.Helper()

	entropy := rand.New(rand.NewSource(time.Now().UnixNano()))
	return ulid.MustNew(ulid.Timestamp(ts), entropy)
}
