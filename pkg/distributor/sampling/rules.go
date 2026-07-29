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

// computeExceptions returns, per location id, the recording rules that exempt
// that location from stripping (subject to their deferred matchers being
// satisfied by the sample at strip time). A nil result means the whole profile
// is stripped to totals.
func (s *ProfileStripper) computeExceptions(tenantID string, seriesLabels phlaremodel.Labels, p *profilev1.Profile) map[uint64][]deferredRule {
	targetFuncs := s.targetedFunctions(tenantID, seriesLabels, p)
	if len(targetFuncs) == 0 {
		return nil
	}
	rulesByFunctionID := make(map[uint64][]deferredRule, len(p.Function))
	for _, fn := range p.Function {
		if rules, ok := targetFuncs[p.StringTable[fn.Name]]; ok {
			rulesByFunctionID[fn.Id] = rules
		}
	}
	if len(rulesByFunctionID) == 0 {
		return nil
	}
	exceptions := make(map[uint64][]deferredRule, len(p.Location))
	for _, loc := range p.Location {
		var rules []deferredRule
		for _, line := range loc.Line {
			rules = append(rules, rulesByFunctionID[line.FunctionId]...)
		}
		if len(rules) > 0 {
			exceptions[loc.Id] = rules
		}
	}
	return exceptions
}

func (s *ProfileStripper) targetedFunctions(tenantID string, seriesLabels phlaremodel.Labels, p *profilev1.Profile) map[string][]deferredRule {
	if s.ruler == nil {
		return nil
	}
	profileTypes := profileTypeNames(p, seriesLabels.Get(phlaremodel.LabelNameProfileName))
	var targets map[string][]deferredRule
	for _, rule := range s.ruler.RecordingRules(tenantID) {
		if rule == nil || rule.FunctionName == "" {
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
		if targets == nil {
			targets = make(map[string][]deferredRule)
		}
		targets[rule.FunctionName] = append(targets[rule.FunctionName], deferredRule{
			deferredMatchers: deferredMatchers,
			keepLabels:       keepLabels,
		})
	}
	return targets
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
