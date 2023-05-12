package ingester

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/grafana/phlare/pkg/phlaredb"
	diskutil "github.com/grafana/phlare/pkg/util/disk"
)

type mockFS struct {
	mock.Mock
}

func (f *mockFS) HasHighDiskUtilization(path string) (*diskutil.VolumeStats, error) {
	args := f.Called(path)
	return args[0].(*diskutil.VolumeStats), args.Error(1)
}

func (f *mockFS) Open(path string) (fs.File, error) {
	args := f.Called(path)
	return args[0].(fs.File), args.Error(1)
}

func (f *mockFS) RemoveAll(path string) error {
	args := f.Called(path)
	return args.Error(0)
}

func (f *mockFS) ReadDir(path string) ([]fs.DirEntry, error) {
	args := f.Called(path)
	return args[0].([]fs.DirEntry), args.Error(1)
}

type fakeFile struct {
	name string
	dir  bool
}

func (f *fakeFile) Name() string               { return f.name }
func (f *fakeFile) IsDir() bool                { return f.dir }
func (f *fakeFile) Info() (fs.FileInfo, error) { panic("not implemented") }
func (f *fakeFile) Type() fs.FileMode          { panic("not implemented") }

type mockBlockEvicter struct{ mock.Mock }

func (e *mockBlockEvicter) evictBlock(tenantID string, b ulid.ULID, fn func() error) error {
	args := e.Called(tenantID, b, fn)
	if err := fn(); err != nil {
		return err
	}
	return args.Error(0)
}

func TestRetentionEnforcer_cleanupBlocksWhenHighDiskUtilization(t *testing.T) {
	const suffix = "0000000000000000000000"

	for _, tc := range []struct {
		name     string
		fsMock   func(fs *mockFS, e *mockBlockEvicter)
		logLines []string
		err      string
	}{
		{
			name: "no-high-disk-utilization",
			fsMock: func(f *mockFS, e *mockBlockEvicter) {
				f.On("HasHighDiskUtilization", mock.Anything).Return(&diskutil.VolumeStats{HighDiskUtilization: false}, nil).Once()
			},
		},
		{
			name: "high-disk-utilization-no-blocks",
			fsMock: func(f *mockFS, e *mockBlockEvicter) {
				f.On("HasHighDiskUtilization", mock.Anything).Return(&diskutil.VolumeStats{HighDiskUtilization: true, BytesAvailable: 10}, nil).Once()
				f.On("ReadDir", mock.Anything).Return([]fs.DirEntry{&fakeFile{"just-a-file", false}}, nil).Once()
			},
		},
		{
			name: "high-disk-utilization-delete-single-block",
			fsMock: func(f *mockFS, e *mockBlockEvicter) {
				f.On("HasHighDiskUtilization", "./data").Return(&diskutil.VolumeStats{HighDiskUtilization: true, BytesAvailable: 10}, nil).Once()
				f.On("ReadDir", "./data").Return([]fs.DirEntry{
					&fakeFile{"anonymous", true},
					&fakeFile{"test-tenant", true},
				}, nil).Once()
				f.On("ReadDir", "data/anonymous/local").Return([]fs.DirEntry{
					&fakeFile{"01AC" + suffix, true},
					&fakeFile{"01AB" + suffix, true},
					&fakeFile{"01AA" + suffix, true},
				}, nil).Once()
				f.On("ReadDir", "data/test-tenant/local").Return([]fs.DirEntry{
					&fakeFile{"01AC" + suffix, true},
					&fakeFile{"01AB" + suffix, true},
					&fakeFile{"01AA" + suffix, true},
				}, nil).Once()
				e.On("evictBlock", "anonymous", ulid.MustParse("01AA"+suffix), mock.Anything).Return(nil).Once()
				f.On("RemoveAll", "data/anonymous/local/01AA"+suffix).Return(nil).Once()
				f.On("HasHighDiskUtilization", "./data").Return(&diskutil.VolumeStats{HighDiskUtilization: false, BytesAvailable: 11}, nil).Once()
			},
			logLines: []string{`{"level":"warn", "msg":"disk utilization is high, deleting the oldest block", "path":"data/anonymous/local/01AA0000000000000000000000"}`},
		},
		{
			name: "high-disk-utilization-delete-multiple-blocks",
			fsMock: func(f *mockFS, e *mockBlockEvicter) {
				f.On("HasHighDiskUtilization", "./data").Return(&diskutil.VolumeStats{HighDiskUtilization: true, BytesAvailable: 10}, nil).Once()
				f.On("ReadDir", "./data").Return([]fs.DirEntry{
					&fakeFile{"anonymous", true},
					&fakeFile{"test-tenant", true},
				}, nil).Once()
				f.On("ReadDir", "data/anonymous/local").Return([]fs.DirEntry{
					&fakeFile{"01AC" + suffix, true},
					&fakeFile{"01AB" + suffix, true},
					&fakeFile{"01AA" + suffix, true},
				}, nil).Once()
				f.On("ReadDir", "data/test-tenant/local").Return([]fs.DirEntry{
					&fakeFile{"01AC" + suffix, true},
					&fakeFile{"01AB" + suffix, true},
					&fakeFile{"01AA" + suffix, true},
				}, nil).Once()
				e.On("evictBlock", "anonymous", ulid.MustParse("01AA"+suffix), mock.Anything).Return(nil).Once()
				f.On("RemoveAll", "data/anonymous/local/01AA"+suffix).Return(nil).Once()
				f.On("HasHighDiskUtilization", "./data").Return(&diskutil.VolumeStats{HighDiskUtilization: true, BytesAvailable: 11}, nil).Once()

				e.On("evictBlock", "test-tenant", ulid.MustParse("01AA"+suffix), mock.Anything).Return(nil).Once()
				f.On("RemoveAll", "data/test-tenant/local/01AA"+suffix).Return(nil).Once()
				f.On("HasHighDiskUtilization", "./data").Return(&diskutil.VolumeStats{HighDiskUtilization: true, BytesAvailable: 12}, nil).Once()

				e.On("evictBlock", "anonymous", ulid.MustParse("01AB"+suffix), mock.Anything).Return(nil).Once()
				f.On("RemoveAll", "data/anonymous/local/01AB"+suffix).Return(nil).Once()
				f.On("HasHighDiskUtilization", "./data").Return(&diskutil.VolumeStats{HighDiskUtilization: false, BytesAvailable: 12}, nil).Once()
			},
			logLines: []string{
				`{"level":"warn", "msg":"disk utilization is high, deleting the oldest block", "path":"data/anonymous/local/01AA0000000000000000000000"}`,
				`{"level":"warn", "msg":"disk utilization is high, deleting the oldest block", "path":"data/test-tenant/local/01AA0000000000000000000000"}`,
				`{"level":"warn", "msg":"disk utilization is high, deleting the oldest block", "path":"data/anonymous/local/01AB0000000000000000000000"}`,
			},
		},
		{
			name: "high-disk-utilization-delete-blocks-no-reduction-in-usage",
			fsMock: func(f *mockFS, e *mockBlockEvicter) {
				f.On("HasHighDiskUtilization", "./data").Return(&diskutil.VolumeStats{HighDiskUtilization: true, BytesAvailable: 10}, nil).Once()
				f.On("ReadDir", "./data").Return([]fs.DirEntry{
					&fakeFile{"anonymous", true},
					&fakeFile{"test-tenant", true},
				}, nil).Once()
				f.On("ReadDir", "data/anonymous/local").Return([]fs.DirEntry{
					&fakeFile{"01AC" + suffix, true},
					&fakeFile{"01AB" + suffix, true},
					&fakeFile{"01AA" + suffix, true},
				}, nil).Once()
				f.On("ReadDir", "data/test-tenant/local").Return([]fs.DirEntry{
					&fakeFile{"01AC" + suffix, true},
					&fakeFile{"01AB" + suffix, true},
					&fakeFile{"01AA" + suffix, true},
				}, nil).Once()
				e.On("evictBlock", "anonymous", ulid.MustParse("01AA"+suffix), mock.Anything).Return(nil).Once()
				f.On("RemoveAll", "data/anonymous/local/01AA"+suffix).Return(nil).Once()
				f.On("HasHighDiskUtilization", "./data").Return(&diskutil.VolumeStats{HighDiskUtilization: true, BytesAvailable: 10}, nil).Once()
				e.On("evictBlock", "test-tenant", ulid.MustParse("01AA"+suffix), mock.Anything).Return(nil).Once()
				f.On("RemoveAll", "data/test-tenant/local/01AA"+suffix).Return(nil).Once()
				f.On("HasHighDiskUtilization", "./data").Return(&diskutil.VolumeStats{HighDiskUtilization: false, BytesAvailable: 10}, nil).Once()
			},
			logLines: []string{
				`{"level":"warn", "msg":"disk utilization is high, deleting the oldest block", "path":"data/anonymous/local/01AA0000000000000000000000"}`,
				`{"level":"warn", "msg":"disk utilization is not lowered by deletion of a block, pausing until next cycle"}`,
			},
		},
		{
			name: "block-deletion-failure",
			fsMock: func(f *mockFS, e *mockBlockEvicter) {
				f.On("HasHighDiskUtilization", "./data").Return(&diskutil.VolumeStats{HighDiskUtilization: true, BytesAvailable: 10}, nil).Once()
				f.On("ReadDir", "./data").Return([]fs.DirEntry{
					&fakeFile{"anonymous", true},
				}, nil).Once()
				f.On("ReadDir", "data/anonymous/local").Return([]fs.DirEntry{
					&fakeFile{"01AC" + suffix, true},
					&fakeFile{"01AB" + suffix, true},
					&fakeFile{"01AA" + suffix, true},
				}, nil).Once()
				e.On("evictBlock", "anonymous", ulid.MustParse("01AA"+suffix), mock.Anything).Return(nil).Once()
				f.On("RemoveAll", "data/anonymous/local/01AA"+suffix).Return(fmt.Errorf("error expected")).Once()
			},
			err: `failed to delete block "data/anonymous/local/01AA0000000000000000000000": error expected`,
		},
		{
			name: "block-not-found-error",
			fsMock: func(f *mockFS, e *mockBlockEvicter) {
				f.On("HasHighDiskUtilization", "./data").Return(&diskutil.VolumeStats{HighDiskUtilization: true, BytesAvailable: 10}, nil).Once()
				f.On("ReadDir", "./data").Return([]fs.DirEntry{
					&fakeFile{"anonymous", true},
				}, nil).Once()
				f.On("ReadDir", "data/anonymous/local").Return([]fs.DirEntry{
					&fakeFile{"01AC" + suffix, true},
					&fakeFile{"01AB" + suffix, true},
					&fakeFile{"01AA" + suffix, true},
				}, nil).Once()
				e.On("evictBlock", "anonymous", ulid.MustParse("01AA"+suffix), mock.Anything).Return(nil).Once()
				f.On("RemoveAll", "data/anonymous/local/01AA"+suffix).Return(os.ErrNotExist).Once()
				f.On("HasHighDiskUtilization", "./data").Return(&diskutil.VolumeStats{HighDiskUtilization: true, BytesAvailable: 11}, nil).Once()
				e.On("evictBlock", "anonymous", ulid.MustParse("01AB"+suffix), mock.Anything).Return(nil).Once()
				f.On("RemoveAll", "data/anonymous/local/01AB"+suffix).Return(nil).Once()
				f.On("HasHighDiskUtilization", "./data").Return(&diskutil.VolumeStats{HighDiskUtilization: false, BytesAvailable: 12}, nil).Once()
			},
			logLines: []string{
				`{"level":"warn", "msg":"disk utilization is high, deleting the oldest block", "path":"data/anonymous/local/01AA0000000000000000000000"}`,
				`{"level":"warn","msg":"block not found on disk","path":"data/anonymous/local/01AA0000000000000000000000"}`,
				`{"level":"warn", "msg":"disk utilization is high, deleting the oldest block", "path":"data/anonymous/local/01AB0000000000000000000000"}`,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var (
				logBuf      = bytes.NewBuffer(nil)
				logger      = log.NewJSONLogger(log.NewSyncWriter(logBuf))
				ctx         = context.Background()
				fsMock      = new(mockFS)
				evicterMock = new(mockBlockEvicter)
			)

			e := newRetentionPolicyEnforcer(logger, evicterMock, defaultRetentionPolicy(), phlaredb.Config{
				DataPath: "./data",
			})
			e.fileSystem = fsMock
			e.volumeChecker = fsMock

			tc.fsMock(fsMock, evicterMock)

			if tc.err == "" {
				require.NoError(t, e.cleanupBlocksWhenHighDiskUtilization(ctx))
			} else {
				require.Equal(t, tc.err, e.cleanupBlocksWhenHighDiskUtilization(ctx).Error())
			}

			// check for log lines
			if len(tc.logLines) > 0 {
				lines := strings.Split(strings.TrimSpace(logBuf.String()), "\n")
				require.Len(t, lines, len(tc.logLines))
				for idx := range tc.logLines {
					require.JSONEq(t, tc.logLines[idx], lines[idx])
				}
			}
		})
	}
}
