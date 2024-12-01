package index

import (
	"time"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/index/store"
)

type PartitionMeta struct {
	Key       store.PartitionKey
	Timestamp time.Time
	Duration  time.Duration
	Tenants   []string

	tenantMap map[string]struct{}
}

func (m *PartitionMeta) HasTenant(tenant string) bool {
	m.loadTenants()
	_, ok := m.tenantMap[tenant]
	return ok
}

func (m *PartitionMeta) StartTime() time.Time {
	return m.Timestamp
}

func (m *PartitionMeta) EndTime() time.Time {
	return m.Timestamp.Add(m.Duration)
}

func (m *PartitionMeta) loadTenants() {
	if len(m.Tenants) > 0 && len(m.tenantMap) == 0 {
		m.tenantMap = make(map[string]struct{}, len(m.Tenants))
		for _, t := range m.Tenants {
			m.tenantMap[t] = struct{}{}
		}
	}
}

func (m *PartitionMeta) AddTenant(tenant string) {
	m.loadTenants()
	if _, ok := m.tenantMap[tenant]; !ok {
		m.tenantMap[tenant] = struct{}{}
		m.Tenants = append(m.Tenants, tenant)
	}
}

func (m *PartitionMeta) compare(other *PartitionMeta) int {
	if m == other {
		return 0
	}
	return m.Timestamp.Compare(other.Timestamp)
}

// [ m.StartTime(), m.EndTime() )
func (m *PartitionMeta) overlaps(start, end time.Time) bool {
	return start.Before(m.EndTime()) && !end.Before(m.StartTime())
}

// [ m.StartTime(), m.EndTime() )
func (m *PartitionMeta) contains(t time.Time) bool {
	return !t.Before(m.StartTime()) && t.Before(m.EndTime())
}
