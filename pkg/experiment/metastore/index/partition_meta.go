package index

import (
	"time"
)

type PartitionMeta struct {
	Key      PartitionKey
	Ts       time.Time
	Duration time.Duration
	Tenants  []string

	tenantMap map[string]struct{}
}

func (m *PartitionMeta) HasTenant(tenant string) bool {
	m.loadTenants()
	_, ok := m.tenantMap[tenant]
	return ok
}

func (m *PartitionMeta) StartTime() time.Time {
	return m.Ts
}

func (m *PartitionMeta) EndTime() time.Time {
	return m.Ts.Add(m.Duration)
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
	return m.Ts.Compare(other.Ts)
}

// [ m.StartTime(), m.EndTime() )
func (m *PartitionMeta) overlaps(start, end int64) bool {
	return start < m.EndTime().UnixMilli() && end >= m.StartTime().UnixMilli()
}

// [ m.StartTime(), m.EndTime() )
func (m *PartitionMeta) contains(t int64) bool {
	return t >= m.StartTime().UnixMilli() && t < m.EndTime().UnixMilli()
}
