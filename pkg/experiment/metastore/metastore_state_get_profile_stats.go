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
	var err error
	m.index.run(func() {
		g, _ := errgroup.WithContext(ctx)
		for _, p := range m.index.partitionMap {
			p := p
			g.Go(func() error {
				oldest := int64(math.MaxInt64)
				newest := int64(math.MinInt64)
				ingested := false
				for _, s := range p.shards {
					for tKey, t := range s.tenants {
						if tKey != "" && tKey != tenant {
							continue
						}
						hasTenant := tKey == tenant
						if !hasTenant { // this is an anonymous tenant, skipping for simplicity
							continue
						}
						ingested = len(t.blocks) > 0
						for _, b := range t.blocks {
							if b.MinTime < oldest {
								oldest = b.MinTime
							}
							if b.MaxTime > newest {
								newest = b.MaxTime
							}
						}
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

		if err = g.Wait(); err != nil {
			return
		}
	})
	if err != nil {
		return nil, err
	}

	return &resp, nil
}
