package index

import "time"

type PartitionMeta struct {
	Key      PartitionKey  `json:"key"`
	Ts       time.Time     `json:"ts"`
	Duration time.Duration `json:"duration"`
	Tenants  []string      `json:"tenants"`

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
