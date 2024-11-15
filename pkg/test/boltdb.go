package test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

func BoltDB(t *testing.T) *bbolt.DB {
	tempDir := t.TempDir()
	opts := bbolt.Options{
		NoGrowSync:      true,
		NoFreelistSync:  true,
		FreelistType:    bbolt.FreelistMapType,
		InitialMmapSize: 32 << 20,
		NoSync:          true,
	}
	db, err := bbolt.Open(filepath.Join(tempDir, "boltdb"), 0644, &opts)
	require.NoError(t, err)
	return db
}
