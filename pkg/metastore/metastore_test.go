package metastore

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

func Test_Metastore(t *testing.T) {
	peers := []string{
		"localhost:9100/localhost-0",
		// "localhost:9101/localhost-1",
		// "localhost:9102/localhost-2",
	}

	waitC := make(chan struct{})
	shutdown := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(len(peers))

	go func() {
		defer wg.Done()
		m := newReplica(t, 0, peers)
		<-waitC
		for range time.NewTicker(time.Second).C {
			_, err := m.AddBlock(context.Background(), &metastorev1.AddBlockRequest{
				Block: &metastorev1.BlockMeta{
					Id: "2", TenantServices: []*metastorev1.TenantService{{TenantId: "anon", Name: "svc-1"}},
				},
			})
			assert.NoError(t, err)
		}

		<-shutdown
		t.Log(m.ListBlocksForQuery(context.Background(), &metastorev1.ListBlocksForQueryRequest{
			TenantId: []string{"anon"},
			EndTime:  math.MaxInt64,
			Query:    "{}",
		}))

		assert.NoError(t, m.Shutdown())
	}()

	close(waitC)
	time.Sleep(10 * time.Second)
	close(shutdown)
	wg.Wait()
}

func newReplica(t *testing.T, i int, peers []string) *Metastore {
	dir := "./testdata/" + strconv.Itoa(i)
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

	logger := log.NewLogfmtLogger(os.Stderr)
	m, err := New(config, nil, logger, nil)
	require.NoError(t, err)
	require.NoError(t, m.starting(context.Background()))
	return m
}
