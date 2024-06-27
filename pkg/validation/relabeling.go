package validation

import (
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/relabel"
)

var (
	godeltaprof         = relabel.MustNewRegexp("godeltaprof_(.*)")
	defaultRelabelRules = []*relabel.Config{
		// replace godeltaprof_ prefix from name
		{
			SourceLabels: []model.LabelName{"__name__"},
			Action:       relabel.Replace,
			Regex:        godeltaprof,
			TargetLabel:  "__name_replaced__",
			Replacement:  "$0",
		},
		{
			SourceLabels: []model.LabelName{"__name__"},
			Regex:        godeltaprof,
			Action:       relabel.Replace,
			TargetLabel:  "__name__",
			Replacement:  "$1",
		},
		// replace wall with process_cpu when __type__ is cpu
		{
			SourceLabels: []model.LabelName{"__name__", "__type__"},
			Separator:    "/",
			Regex:        relabel.MustNewRegexp("wall/cpu"),
			Action:       relabel.Replace,
			Replacement:  "wall",
			TargetLabel:  "__name_replaced__",
		},
		{
			SourceLabels: []model.LabelName{"__name__", "__type__"},
			Separator:    "/",
			Regex:        relabel.MustNewRegexp("wall/cpu"),
			Action:       relabel.Replace,
			Replacement:  "process_cpu",
			TargetLabel:  "__name__",
		},
	}
)

func (o *Overrides) IngestionRelabelingRules(tenantID string) []*relabel.Config {
	l := o.getOverridesForTenant(tenantID)

	// return only custom rules when default rules are disabled
	if l.IngestionRelabelingDefaultRulesPosition == RulePositionDisabled {
		return l.IngestionRelabelingRules
	}

	// quick return if no rules are defined
	if len(l.IngestionRelabelingRules) == 0 {
		return defaultRelabelRules
	}

	rules := make([]*relabel.Config, 0, len(l.IngestionRelabelingRules)+len(defaultRelabelRules))

	if l.IngestionRelabelingDefaultRulesPosition == "" || l.IngestionRelabelingDefaultRulesPosition == RulePositionFirst {
		rules = append(rules, defaultRelabelRules...)
		return append(rules, l.IngestionRelabelingRules...)
	}

	rules = append(rules, l.IngestionRelabelingRules...)
	return append(rules, defaultRelabelRules...)
}
