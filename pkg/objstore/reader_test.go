package objstore_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/phlare/pkg/objstore/client"
	"github.com/grafana/phlare/pkg/objstore/providers/filesystem"
)

func Test_FileSystem(t *testing.T) {
	testDir := t.TempDir()
	b, err := client.NewBucket(context.Background(), client.Config{
		StorageBackendConfig: client.StorageBackendConfig{
			Backend: client.Filesystem,
			Filesystem: filesystem.Config{
				Directory: testDir,
			},
		},
		StoragePrefix: "testdata/",
	}, "foo")
	require.NoError(t, err)

	// make the file locally
	dir := filepath.Join(testDir, "testdata")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(dir+"/foo.parquet", []byte(`12345`), 0o644))

	ra, err := b.ReaderAt(context.Background(), "foo.parquet")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, ra.Close())
	}()
	require.NotNil(t, ra)
	require.IsType(t, &filesystem.FileReaderAt{}, ra)

	buf := make([]byte, 2)
	read, err := ra.ReadAt(buf, 3)
	require.NoError(t, err)
	require.Equal(t, 2, read)
	require.Equal(t, []byte("45"), buf)
}
