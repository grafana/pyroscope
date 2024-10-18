package blockcleaner

import (
	"crypto/rand"
	"path/filepath"
	"testing"
	"time"

	"github.com/oklog/ulid"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	"github.com/grafana/pyroscope/pkg/util"
)

func Test_AddAndCheck(t *testing.T) {
	markers := NewDeletionMarkers(
		createDb(t),
		&Config{CompactedBlocksCleanupDelay: time.Second * 2},
		util.Logger,
		nil,
	)

	blockId := ulid.MustNew(ulid.Now(), rand.Reader).String()
	err := markers.Mark(0, "Tenant", blockId, 1000)
	require.NoError(t, err)

	require.True(t, markers.IsMarked(blockId))
}

func createDb(t *testing.T) *bbolt.DB {
	opts := *bbolt.DefaultOptions
	opts.ReadOnly = false
	opts.NoSync = true
	db, err := bbolt.Open(filepath.Join(t.TempDir(), "db.boltdb"), 0644, &opts)
	require.NoError(t, err)
	return db
}
