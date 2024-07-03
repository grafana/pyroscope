package metastore

import (
	"context"
	"flag"
	"math"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	metastoreclient "github.com/grafana/pyroscope/pkg/metastore/client"
)

func Test_Metastore_(t *testing.T) {
	go func() {
		cfg := metastoreclient.Config{}
		cfg.RegisterFlags(flag.NewFlagSet("", flag.ExitOnError))
		cfg.MetastoreAddress = "localhost:9200"
		c, err := metastoreclient.New(cfg)
		require.NoError(t, err)
		for {
			time.Sleep(500 * time.Millisecond)
			resp, err := c.ListBlocksForQuery(context.Background(), &metastorev1.ListBlocksForQueryRequest{
				TenantId:  []string{"a-tenant"},
				StartTime: 0,
				EndTime:   math.MaxInt64,
				Query:     "{}",
			})
			t.Log(err, resp)
		}
	}()

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

	time.Sleep(10 * time.Second)
	t.Log("Shutdown")
	// assert.NoError(t, m.raft.Snapshot().Error())
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
	h := health.NewServer()
	m, err := New(config, nil, logger, nil, h)
	require.NoError(t, err)
	require.NoError(t, m.starting(context.Background()))
	go func() {
		srv := grpc.NewServer()
		srv.RegisterService(&metastorev1.MetastoreService_ServiceDesc, m)
		srv.RegisterService(&grpc_health_v1.Health_ServiceDesc, h)
		l, err := net.Listen("tcp", ":920"+strconv.Itoa(i))
		require.NoError(t, err)
		require.NoError(t, srv.Serve(l))
	}()

	return m
}
