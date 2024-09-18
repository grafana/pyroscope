package metastore

import (
	"context"
	"math"
	"sync"

	"golang.org/x/sync/errgroup"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func (m *Metastore) GetProfileStats(
	ctx context.Context,
	r *metastorev1.GetProfileStatsRequest,
) (*typesv1.GetProfileStatsResponse, error) {
	// TODO(kolesnikovae): ReadIndex
	return m.state.getProfileStats(r.TenantId, ctx)
}

func (m *metastoreState) getProfileStats(tenant string, ctx context.Context) (*typesv1.GetProfileStatsResponse, error) {
	var respMutex sync.Mutex
	resp := typesv1.GetProfileStatsResponse{
		DataIngested:      false,
		OldestProfileTime: math.MaxInt64,
		NewestProfileTime: math.MinInt64,
	}
	m.shardsMutex.Lock()
	defer m.shardsMutex.Unlock()
	g, ctx := errgroup.WithContext(ctx)
	for _, s := range m.shards {
		s := s
		g.Go(func() error {
			oldest := int64(math.MaxInt64)
			newest := int64(math.MinInt64)
			ingested := len(s.segments) > 0
			for _, b := range s.segments {
				if b.TenantId != "" && b.TenantId != tenant {
					continue
				}
				hasTenant := b.TenantId == tenant
				if !hasTenant {
					for _, d := range b.Datasets {
						if d.TenantId == tenant {
							hasTenant = true
							break
						}
					}
				}
				if !hasTenant {
					continue
				}
				if b.MinTime < oldest {
					oldest = b.MinTime
				}
				if b.MaxTime > newest {
					newest = b.MaxTime
				}
			}
			respMutex.Lock()
			defer respMutex.Unlock()
			resp.DataIngested = resp.DataIngested || ingested
			if oldest < resp.OldestProfileTime {
				resp.OldestProfileTime = oldest
			}
			if newest > resp.NewestProfileTime {
				resp.NewestProfileTime = newest
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return &resp, nil
}
