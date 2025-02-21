package metrics

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/pkg/model"
)

const (
	envVarRecordingRules = "PYROSCOPE_RECORDING_RULES"
)

type StaticRuler struct {
	rules  map[string][]*model.RecordingRule
	logger log.Logger
}

func NewStaticRulerFromEnvVars(logger log.Logger) (Ruler, error) {
	jsonRules := os.Getenv(envVarRecordingRules)

	var rulesByTenant map[string][]*settingsv1.RecordingRule
	if err := json.Unmarshal([]byte(jsonRules), &rulesByTenant); err != nil {
		return nil, fmt.Errorf("failed to unmarshal recording rules: %w", err)
	}

	ruler := &StaticRuler{
		rules:  make(map[string][]*model.RecordingRule, len(rulesByTenant)),
		logger: logger,
	}
	for tenant, rules := range rulesByTenant {
		rs := make([]*model.RecordingRule, 0, len(rules))
		for _, rule := range rules {
			r, err := model.NewRecordingRule(rule)
			if err == nil {
				rs = append(rs, r)
			} else {
				level.Error(logger).Log("msg", "failed to parse recording rule", "rule", rule, "err", err)
			}
		}
		ruler.rules[tenant] = rs
	}
	return ruler, nil
}

func (r StaticRuler) RecordingRules(tenant string) []*model.RecordingRule {
	return r.rules[tenant]
}
