package validation

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

type RecordingRules []*RulesConfig

type RulesConfig struct {
	MetricName     string              `yaml:"metric_name" json:"metric_name"`
	Matchers       []string            `yaml:"matchers" json:"matchers"`
	ExternalLabels []map[string]string `yaml:"external_labels" json:"external_labels"`
	GroupBy        []string            `yaml:"group_by" json:"group_by"`
}

func (r *RecordingRules) Set(s string) error {
	var rules []*RulesConfig
	if err := json.Unmarshal([]byte(s), &rules); err != nil {
		return fmt.Errorf("failed to unmarshal recording rules: %w", err)
	}
	*r = rules
	return nil
}

func (r *RecordingRules) String() string {
	yamlBytes, err := yaml.Marshal(r)
	if err != nil {
		panic(fmt.Errorf("error marshal yaml: %w", err))
	}

	temp := make([]interface{}, 0, len(*r))
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

func (o *Overrides) RecordingRules(tenantId string) []*RulesConfig {
	limits := o.getOverridesForTenant(tenantId)
	return limits.RecordingRules
}
