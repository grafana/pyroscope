package validation

import (
	"encoding/json"
	"flag"
	"fmt"

	"gopkg.in/yaml.v3"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
)

type RecordingRules []*settingsv1.RecordingRule

func (r *RecordingRules) RegisterFlags(f *flag.FlagSet) {
	f.Var(r, "compaction-worker.metrics-exporter.rules-source.static", "List of static recording rules of the type settingsv1.RecordingRule. Will only be use in the absence of a recording rules client.")
}

func (r *RecordingRules) Set(s string) error {
	var rules []*settingsv1.RecordingRule
	if err := json.Unmarshal([]byte(s), &rules); err != nil {
		return fmt.Errorf("failed to unmarshal recording rules: %w", err)
	}
	*r = rules
	return nil
}

func (r *RecordingRules) String() string {
	jsonBytes, err := json.Marshal(*r)
	if err != nil {
		panic(fmt.Errorf("error marshal json: %w", err))
	}
	return string(jsonBytes)
}

func (r *RecordingRules) UnmarshalYAML(value *yaml.Node) error {
	var temp interface{}
	if err := value.Decode(&temp); err != nil {
		return err
	}

	jsonBytes, err := json.Marshal(temp)
	if err != nil {
		return err
	}

	return json.Unmarshal(jsonBytes, r)
}

func (o *Overrides) RecordingRules(tenantId string) []*settingsv1.RecordingRule {
	limits := o.getOverridesForTenant(tenantId)
	return limits.RecordingRules
}
