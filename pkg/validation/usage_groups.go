// This file is a modified copy of the usage groups implementation in Mimir:
//
// https://github.com/grafana/mimir/blob/0e8c09f237649e95dc1bf3f7547fd279c24bdcf9/pkg/ingester/activeseries/custom_trackers_config.go#L48

package validation

import (
	"fmt"

	amlabels "github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/prometheus/model/labels"

	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

const (
	// Maximum number of usage groups that can be configured (per tenant).
	maxUsageGroups = 50
)

// TenantUsageGroups is an allowlist of service names that have per-app usage
// enabled. This allowlist is constructed on a per-tenant basis.
type TenantUsageGroups struct {
	TenantID string
	config   map[string][]*labels.Matcher
}

// GetUsageGroupName matches the label set to a usage group. If no usage group
// is matched, the default group name is used.
func (u *TenantUsageGroups) GetUsageGroup(lbls phlaremodel.Labels) string {
	groupName := "other"
	if u == nil {
		return groupName
	}

	for name, matchers := range u.config {
		if matchesAll(matchers, lbls) {
			groupName = name
		}
	}
	return groupName
}

// DistributorUsageGroups returns the usage groups that are enabled for this
// tenant.
func (o *Overrides) DistributorUsageGroups(tenantID string) (*TenantUsageGroups, error) {
	ug := &TenantUsageGroups{
		TenantID: tenantID,
	}

	groups := o.getOverridesForTenant(tenantID).DistributorUsageGroups
	if len(groups) == 0 {
		return ug, nil
	}

	if len(groups) > maxUsageGroups {
		return nil, fmt.Errorf("too many usage groups configured for tenant %q: got %d, max %d", tenantID, len(groups), maxUsageGroups)
	}

	existingNames := make(map[string]struct{}, len(groups))
	ug.config = make(map[string][]*labels.Matcher, len(groups))
	for _, group := range groups {
		for name, matchersString := range group {
			if _, ok := existingNames[name]; ok {
				return nil, fmt.Errorf("duplicate usage group name %q for tenant %q", name, tenantID)
			}
			existingNames[name] = struct{}{}

			if name == "" {
				return nil, fmt.Errorf("empty service name in usage group for tenant %q", tenantID)
			}

			if matchersString == "" {
				return nil, fmt.Errorf("no matchers for usage group %q and tenant %q", name, tenantID)
			}

			amMatchers, err := amlabels.ParseMatchers(matchersString)
			if err != nil {
				return nil, fmt.Errorf("failed to parse matchers for usage group %q and tenant %q: %w", name, tenantID, err)
			}

			matchers := make([]*labels.Matcher, len(amMatchers))
			for i, m := range amMatchers {
				matchers[i] = amlabelMatcherToProm(m)
			}
			ug.config[name] = matchers
		}
	}

	return ug, nil
}

func amlabelMatcherToProm(m *amlabels.Matcher) *labels.Matcher {
	// TODO(bryanhuhta) we actually don't have a test (yet).
	// labels.MatchType(m.Type) is a risky conversion because it depends on the iota order, but we have a test for it
	return labels.MustNewMatcher(labels.MatchType(m.Type), m.Name, m.Value)
}

func matchesAll(matchers []*labels.Matcher, lbls phlaremodel.Labels) bool {
	if len(lbls) == 0 {
		return false
	}

	for _, m := range matchers {
		for _, lbl := range lbls {
			if lbl.Name == m.Name && !m.Matches(lbl.Value) {
				return false
			}
		}
	}
	return true
}
