package metastore

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/util/health"
)

func Test_Metastore_(t *testing.T) {
	peers := []string{
		"localhost:9100/localhost-0",
		// "localhost:9101/localhost-1",
		// "localhost:9102/localhost-2",
	}

	m := newReplica(t, 0, "./testdata/", peers)
	time.Sleep(2 * time.Second)

	_, err := m.AddBlock(context.Background(), &metastorev1.AddBlockRequest{
		Block: &metastorev1.BlockMeta{
			Id:    "my-block-id-1",
			Shard: 0xF0F0,
			TenantServices: []*metastorev1.TenantService{
				{TenantId: "a-tenant", Name: "svc-1"},
			},
		},
	})
	assert.NoError(t, err)

	_, err = m.AddBlock(context.Background(), &metastorev1.AddBlockRequest{
		Block: &metastorev1.BlockMeta{
			Id:    "my-block-id-2",
			Shard: 0xF0F1,
			TenantServices: []*metastorev1.TenantService{
				{TenantId: "a-tenant", Name: "svc-1"},
			},
		},
	})
	assert.NoError(t, err)

	t.Log(m.ListBlocksForQuery(context.Background(), &metastorev1.ListBlocksForQueryRequest{
		TenantId: []string{"a-tenant"},
		EndTime:  math.MaxInt64,
		Query:    "{}",
	}))

	t.Log("Shutdown")
	assert.NoError(t, m.raft.Snapshot().Error())
	assert.NoError(t, m.Shutdown())

	m = newReplica(t, 0, "./testdata/", peers)
	time.Sleep(2 * time.Second)
	t.Log(m.ListBlocksForQuery(context.Background(), &metastorev1.ListBlocksForQueryRequest{
		TenantId: []string{"a-tenant"},
		EndTime:  math.MaxInt64,
		Query:    "{}",
	}))
	assert.NoError(t, m.Shutdown())
}

func newReplica(t *testing.T, i int, dir string, peers []string) *Metastore {
	dir = filepath.Join(dir, strconv.Itoa(i))
	config := Config{
		DataDir: filepath.Join(dir, "data"),
		Raft: RaftConfig{
			BootstrapPeers:   peers,
			BindAddress:      ":910" + strconv.Itoa(i),
			AdvertiseAddress: "localhost:910" + strconv.Itoa(i),
			ServerID:         "localhost-" + strconv.Itoa(i),
			Dir:              filepath.Join(dir, "raft"),
			ApplyTimeout:     5 * time.Second,
		},
	}

	logger := log.NewLogfmtLogger(os.Stdout)
	m, err := New(config, nil, logger, nil, health.NoOpService)
	require.NoError(t, err)
	require.NoError(t, m.starting(context.Background()))
	return m
}
