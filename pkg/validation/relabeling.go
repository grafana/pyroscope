package validation

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"

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
			SourceLabels: []model.LabelName{"__name_replaced__"},
			Action:       relabel.Replace,
			Regex:        godeltaprof,
			TargetLabel:  "__delta__",
			Replacement:  "false",
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

type RelabelRulesPosition string

func (p *RelabelRulesPosition) Set(s string) error {
	switch sp := RelabelRulesPosition(s); sp {
	case RelabelRulePositionFirst, RelabelRulePositionLast, RelabelRulePositionDisabled:
		*p = sp
		return nil
	}
	return fmt.Errorf("invalid ingestion_relabeling_default_rules_position: %s", s)
}

func (p *RelabelRulesPosition) String() string {
	return string(*p)
}

const (
	RelabelRulePositionFirst    RelabelRulesPosition = "first"
	RelabelRulePositionDisabled RelabelRulesPosition = "disabled"
	RelabelRulePositionLast     RelabelRulesPosition = "last"
)

type RelabelRules []*relabel.Config

func (p *RelabelRules) Set(s string) error {
	v := []*relabel.Config{}
	if err := yaml.Unmarshal([]byte(s), &v); err != nil {
		return err
	}

	for idx, rule := range v {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("rule at pos %d is not valid: %w", idx, err)
		}
	}
	*p = v
	return nil
}

func (p *RelabelRules) String() string {
	yamlBytes, err := yaml.Marshal(p)
	if err != nil {
		panic(fmt.Errorf("error marshal yaml: %w", err))
	}

	temp := make([]interface{}, 0, len(*p))
	err = yaml.Unmarshal(yamlBytes, &temp)
	if err != nil {
		panic(fmt.Errorf("error unmarshal yaml: %w", err))
	}

	jsonBytes, err := json.Marshal(temp)
	if err != nil {
		panic(fmt.Errorf("error marshal json: %w", err))
	}
	return string(jsonBytes)
}

type IngestionRelabelRules []*relabel.Config

func (r *IngestionRelabelRules) Set(s string) error {
	return (*RelabelRules)(r).Set(s)
}

func (r *IngestionRelabelRules) String() string {
	return (*RelabelRules)(r).String()
}

func (r *IngestionRelabelRules) ExampleDoc() (comment string, yaml interface{}) {
	return `This example consists of two rules, the first one will drop all profiles received with an label 'environment="secrets"' and the second rule will add a label 'powered_by="Grafana Labs"' to all profile series.`,
		[]map[string]interface{}{
			{"action": "drop", "source_labels": []interface{}{"environment"}, "regex": "secret"},
			{"action": "replace", "replacement": "grafana-labs", "target_label": "powered_by"},
		}
}

type SampleTypeRelabelRules []*relabel.Config

func (r *SampleTypeRelabelRules) Set(s string) error {
	if err := (*RelabelRules)(r).Set(s); err != nil {
		return err
	}

	for idx, rule := range *r {
		if rule.Action != relabel.Drop && rule.Action != relabel.Keep {
			return fmt.Errorf("rule at pos %d: sample type relabeling only supports 'drop' and 'keep' actions, got '%s'", idx, rule.Action)
		}
	}
	return nil
}

func (r *SampleTypeRelabelRules) String() string {
	return (*RelabelRules)(r).String()
}

func (r *SampleTypeRelabelRules) ExampleDoc() (comment string, yaml interface{}) {
	return `This example shows sample type filtering rules. The first rule drops all allocation-related sample types (alloc_objects, alloc_space) from memory profiles, keeping only in-use metrics. The second rule keeps only CPU-related sample types by matching the __type__ label. The third rule shows how to drop allocation sample types for a specific service by combining __type__ and service_name labels.`,
		[]map[string]interface{}{
			{"action": "drop", "source_labels": []interface{}{"__type__"}, "regex": "alloc_.*"},
			{"action": "keep", "source_labels": []interface{}{"__type__"}, "regex": "cpu|wall"},
			{"action": "drop", "source_labels": []interface{}{"__type__", "service_name"}, "separator": ";", "regex": "alloc_.*;my-service"},
		}
}

func (o *Overrides) IngestionRelabelingRules(tenantID string) []*relabel.Config {
	l := o.getOverridesForTenant(tenantID)

	// return only custom rules when default rules are disabled
	if l.IngestionRelabelingDefaultRulesPosition == RelabelRulePositionDisabled {
		return l.IngestionRelabelingRules
	}

	// quick return if no rules are defined
	if len(l.IngestionRelabelingRules) == 0 {
		return defaultRelabelRules
	}

	rules := make([]*relabel.Config, 0, len(l.IngestionRelabelingRules)+len(defaultRelabelRules))

	if l.IngestionRelabelingDefaultRulesPosition == "" || l.IngestionRelabelingDefaultRulesPosition == RelabelRulePositionFirst {
		rules = append(rules, defaultRelabelRules...)
		return append(rules, l.IngestionRelabelingRules...)
	}

	rules = append(rules, l.IngestionRelabelingRules...)
	return append(rules, defaultRelabelRules...)
}

func (o *Overrides) SampleTypeRelabelingRules(tenantID string) []*relabel.Config {
	l := o.getOverridesForTenant(tenantID)
	return l.SampleTypeRelabelingRules
}
