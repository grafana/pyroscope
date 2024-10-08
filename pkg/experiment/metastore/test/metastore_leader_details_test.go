package test

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/grafana/dskit/flagext"
	"github.com/oklog/ulid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore"
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
		_, err := it.MetastoreInstanceClient.AddBlock(context.Background(), &metastorev1.AddBlockRequest{
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
		_, err := it.CompactorInstanceClient.PollCompactionJobs(context.Background(), &compactorv1.PollCompactionJobsRequest{})
		if err != nil {
			requireRaftDetails(t, err)
			errors++
		}
	}
	require.Equal(t, 2, errors)
}

func requireRaftDetails(t *testing.T, err error) {
	t.Log("error", err)
	s, ok := status.FromError(err)
	detailsLeader := ""
	if ok && s.Code() == codes.Unavailable {
		ds := s.Details()
		if len(ds) > 0 {
			for _, d := range ds {
				if rd, ok := d.(*typesv1.RaftDetails); ok {
					detailsLeader = rd.Leader
					break
				}
			}
		}
	}
	t.Log("leader is", detailsLeader)
	require.NotEmpty(t, detailsLeader)
}
