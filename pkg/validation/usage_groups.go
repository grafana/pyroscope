package validation

import phlaremodel "github.com/grafana/pyroscope/pkg/model"

// TenantUsageGroups is an allowlist of service names that have per-app usage
// enabled. This allowlist is constructed on a per-tenant basis.
type TenantUsageGroups struct {
	TenantID  string
	allowlist map[string]struct{}
}

// GetServiceName returns the service name to be used when reporting per-app
// usage metrics.
func (u *TenantUsageGroups) GetServiceName(labels phlaremodel.Labels) string {
	if u == nil {
		return ""
	}

	_, ok := u.allowlist[labels.Get("service_name")]
	if ok {
		return labels.Get("service_name")
	}
	return ""
}

// DistributorUsageGroups returns the usage groups that are enabled for this
// tenant.
func (o *Overrides) DistributorUsageGroups(tenantID string) *TenantUsageGroups {
	ug := &TenantUsageGroups{
		TenantID: tenantID,
	}

	groups := o.getOverridesForTenant(tenantID).DistributorUsageGroups
	if len(groups) == 0 {
		return ug
	}

	ug.allowlist = make(map[string]struct{}, len(groups))
	for _, group := range groups {
		ug.allowlist[group] = struct{}{}
	}
	return ug
}
