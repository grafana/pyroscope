package test

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/grafana/dskit/flagext"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/metastore"
	"github.com/grafana/pyroscope/pkg/metastore/raftnode"
	"github.com/grafana/pyroscope/pkg/objstore/providers/memory"
)

func TestRaftDetailsAddBlock(t *testing.T) {
	cfg := new(metastore.Config)
	flagext.DefaultValues(cfg)

	ms := NewMetastoreSet(t, cfg, 3, memory.NewInMemBucket())
	defer ms.Close()

	errors := 0
	m := &metastorev1.BlockMeta{
		Id: ulid.MustNew(1, rand.Reader).String(),
	}
	for _, it := range ms.Instances {
		_, err := it.AddBlock(context.Background(), &metastorev1.AddBlockRequest{
			Block: m,
		})
		if err != nil {
			requireRaftDetails(t, err)
			errors++
		}
	}
	require.Equal(t, 2, errors)
}

func TestRaftDetailsPullCompaction(t *testing.T) {
	cfg := new(metastore.Config)
	flagext.DefaultValues(cfg)

	ms := NewMetastoreSet(t, cfg, 3, memory.NewInMemBucket())
	defer ms.Close()

	errors := 0
	for _, it := range ms.Instances {
		_, err := it.PollCompactionJobs(context.Background(), &metastorev1.PollCompactionJobsRequest{})
		if err != nil {
			requireRaftDetails(t, err)
			errors++
		}
	}
	require.Equal(t, 2, errors)
}

func requireRaftDetails(t *testing.T, err error) {
	t.Log("error", err)
	leader, ok := raftnode.RaftLeaderFromStatusDetails(err)
	require.True(t, ok)
	t.Log("leader is", leader)
}
