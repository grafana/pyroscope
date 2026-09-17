package sampling

import (
	"github.com/prometheus/prometheus/model/labels"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
)

// Ruler provides a tenant's recording rules, used to decide which stacktraces
// of a sampled-out profile must be preserved during stripping.
type Ruler interface {
	RecordingRules(tenant string) []*phlaremodel.RecordingRule
}

type deferredRule struct {
	deferredMatchers []*labels.Matcher
	keepLabels       []string
}

// exceptions captures what recording rules require to survive stripping. Each
// rule contributes a deferredRule whose deferredMatchers must be satisfied by a
// sample before its keepLabels apply:
//   - functionLocations: per location id, the function-filter rules that keep a
//     stacktrace reaching that location.
//   - totalRules: the non-function ("total") rules; a sample they match keeps
//     their labels so the totals can be split and matched/grouped downstream.
type exceptions struct {
	functionLocations map[uint64][]deferredRule
	totalRules        []deferredRule
}

func (s *ProfileStripper) computeExceptions(tenantID string, seriesLabels phlaremodel.Labels, p *profilev1.Profile) exceptions {
	if s.ruler == nil {
		return exceptions{}
	}
	profileTypes := profileTypeNames(p, seriesLabels.Get(phlaremodel.LabelNameProfileName))
	rulesByFunctionName := map[string][]deferredRule{}
	var totalRules []deferredRule
	for _, rule := range s.ruler.RecordingRules(tenantID) {
		if rule == nil {
			continue
		}
		matched, deferredMatchers := recordingRuleMatchesSeries(rule, seriesLabels, profileTypes)
		if !matched {
			continue
		}
		keepLabels := make([]string, 0, len(deferredMatchers)+len(rule.GroupBy))
		for _, m := range deferredMatchers {
			keepLabels = append(keepLabels, m.Name)
		}
		keepLabels = append(keepLabels, rule.GroupBy...)
		r := deferredRule{deferredMatchers: deferredMatchers, keepLabels: keepLabels}

		if rule.FunctionName == "" {
			// Non-function rule: samples that match it keep its labels.
			totalRules = append(totalRules, r)
			continue
		}
		// Function rule: keep the stacktraces reaching FunctionName.
		rulesByFunctionName[rule.FunctionName] = append(rulesByFunctionName[rule.FunctionName], r)
	}

	functionLocations := map[uint64][]deferredRule{}
	if len(rulesByFunctionName) > 0 {
		rulesByFunctionID := make(map[uint64][]deferredRule, len(p.Function))
		for _, fn := range p.Function {
			if rules, ok := rulesByFunctionName[p.StringTable[fn.Name]]; ok {
				rulesByFunctionID[fn.Id] = rules
			}
		}
		for _, loc := range p.Location {
			var rules []deferredRule
			for _, line := range loc.Line {
				rules = append(rules, rulesByFunctionID[line.FunctionId]...)
			}
			if len(rules) > 0 {
				functionLocations[loc.Id] = rules
			}
		}
	}

	return exceptions{
		functionLocations: functionLocations,
		totalRules:        totalRules,
	}
}

func recordingRuleMatchesSeries(rule *phlaremodel.RecordingRule, seriesLabels phlaremodel.Labels, profileTypes []string) (bool, []*labels.Matcher) {
	var deferredMatchers []*labels.Matcher
	for _, m := range rule.Matchers {
		if m.Name == phlaremodel.LabelNameProfileType {
			var matched bool
			for _, pt := range profileTypes {
				if m.Matches(pt) {
					matched = true
					break
				}
			}
			if !matched {
				return false, nil
			}
			continue
		}
		if v, ok := seriesLabels.GetLabel(m.Name); ok {
			if !m.Matches(v.Value) {
				return false, nil
			}
			continue
		}
		deferredMatchers = append(deferredMatchers, m)
	}
	return true, deferredMatchers
}

func profileTypeNames(p *profilev1.Profile, metricName string) []string {
	var periodType, periodUnit string
	if p.PeriodType != nil {
		periodType = p.StringTable[p.PeriodType.Type]
		periodUnit = p.StringTable[p.PeriodType.Unit]
	}
	types := make([]string, 0, len(p.SampleType))
	for _, st := range p.SampleType {
		types = append(types, metricName+":"+p.StringTable[st.Type]+":"+p.StringTable[st.Unit]+":"+periodType+":"+periodUnit)
	}
	return types
}
