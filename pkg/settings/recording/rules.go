package recording

import (
	"context"

	"github.com/go-kit/log"
	"github.com/thanos-io/objstore"
	"google.golang.org/protobuf/proto"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/v2/pkg/settings/store"
)

// EffectiveRules returns the rules a tenant is evaluated with: the rules
// provisioned by config, followed by the stored rules whose ID is not taken by
// a provisioned one. The inputs are not modified.
func EffectiveRules(config []*settingsv1.RecordingRule, stored []*settingsv1.RecordingRuleStore) []*settingsv1.RecordingRule {
	rules := provisionedRules(config)
	ids := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		ids[rule.Id] = struct{}{}
	}
	for _, rule := range stored {
		if _, overridden := ids[rule.Id]; overridden {
			continue
		}
		rules = append(rules, convertRuleToAPI(rule))
	}
	return rules
}

func provisionedRules(config []*settingsv1.RecordingRule) []*settingsv1.RecordingRule {
	rules := make([]*settingsv1.RecordingRule, len(config))
	for i, rule := range config {
		rule = proto.Clone(rule).(*settingsv1.RecordingRule)
		rule.Provisioned = true
		if rule.Id == "" {
			rule.Id = idForRule(rule)
		}
		rules[i] = rule
	}
	return rules
}

type StoredRules struct {
	Generation int64
	Rules      []*settingsv1.RecordingRuleStore
}

// ReadStoredRules reads the rules file of a tenant; a missing file means no rules.
func ReadStoredRules(ctx context.Context, bucket objstore.Bucket, logger log.Logger, tenantID string) (*StoredRules, error) {
	bs := newBucketStore(logger, bucket, store.Key{TenantID: tenantID})
	rules, err := bs.List(ctx)
	if err != nil {
		return nil, err
	}
	return &StoredRules{Generation: rules.Generation, Rules: rules.Rules}, nil
}
