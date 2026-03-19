package readpath

import (
	"context"
	"errors"
	"sync"
	"time"

	"google.golang.org/grpc"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

// TenantServiceClient is the subset of the metastore client needed to resolve
// the oldest profile time for a tenant.
type TenantServiceClient interface {
	GetTenant(ctx context.Context, in *metastorev1.GetTenantRequest, opts ...grpc.CallOption) (*metastorev1.GetTenantResponse, error)
}

var ErrNoV2Data = errors.New("no v2 data ingested for tenant")

// MetastoreSplitTimeResolver resolves the split time for "auto" mode by
// querying the metastore for each tenant's oldest profile time. Results
// are cached per tenant to avoid calling the metastore on every query.
type MetastoreSplitTimeResolver struct {
	client TenantServiceClient
	ttl    time.Duration

	mu    sync.RWMutex
	cache map[string]cachedSplitTime
}

type cachedSplitTime struct {
	time      time.Time
	expiresAt time.Time
}

func NewMetastoreSplitTimeResolver(client TenantServiceClient, ttl time.Duration) *MetastoreSplitTimeResolver {
	return &MetastoreSplitTimeResolver{
		client: client,
		ttl:    ttl,
		cache:  make(map[string]cachedSplitTime),
	}
}

func (r *MetastoreSplitTimeResolver) OldestProfileTime(ctx context.Context, tenantID string) (time.Time, error) {
	now := time.Now()
	r.mu.RLock()
	if cached, ok := r.cache[tenantID]; ok && now.Before(cached.expiresAt) {
		r.mu.RUnlock()
		return cached.time, nil
	}
	r.mu.RUnlock()

	resp, err := r.client.GetTenant(ctx, &metastorev1.GetTenantRequest{TenantId: tenantID})
	if err != nil {
		return time.Time{}, err
	}
	stats := resp.GetStats()
	if stats == nil || !stats.DataIngested {
		return time.Time{}, ErrNoV2Data
	}

	t := time.UnixMilli(stats.OldestProfileTime)

	r.mu.Lock()
	r.cache[tenantID] = cachedSplitTime{
		time:      t,
		expiresAt: now.Add(r.ttl),
	}
	r.mu.Unlock()

	return t, nil
}
