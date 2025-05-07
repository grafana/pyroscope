package recording

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-kit/log"
	"github.com/thanos-io/objstore"
	"google.golang.org/protobuf/encoding/protojson"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/pkg/settings/store"
)

func newBucketStore(logger log.Logger, bucket objstore.Bucket, key store.Key) *bucketStore {
	bs := &bucketStore{
		logger: logger,
	}

	bs.store = store.New(logger, bucket, key, &storeHelper{
		b: bs,
	})
	return bs
}

type bucketStore struct {
	logger log.Logger
	store  *store.GenericStore[*settingsv1.RecordingRuleStore, *storeHelper]
}

func (b *bucketStore) Get(ctx context.Context, id string) (*settingsv1.RecordingRuleStore, error) {
	var rule *settingsv1.RecordingRuleStore
	err := b.store.Read(ctx, func(ctx context.Context, c *store.Collection[*settingsv1.RecordingRuleStore]) error {
		for _, r := range c.Elements {
			if r.Id != id {
				continue
			}

			rule = r
			break
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if rule == nil {
		return nil, fmt.Errorf("rule %s not found", id)
	}
	return rule, nil
}

func (b *bucketStore) List(ctx context.Context) (*settingsv1.RecordingRulesStore, error) {
	var rules *settingsv1.RecordingRulesStore
	err := b.store.Read(ctx, func(ctx context.Context, c *store.Collection[*settingsv1.RecordingRuleStore]) error {
		rules = &settingsv1.RecordingRulesStore{
			Rules:      c.Elements,
			Generation: c.Generation,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return rules, nil
}

func (b *bucketStore) Upsert(ctx context.Context, newRule *settingsv1.RecordingRuleStore) (*settingsv1.RecordingRuleStore, error) {
	err := b.store.Upsert(ctx, newRule, &newRule.Generation)
	if err != nil {
		return nil, err
	}

	return newRule, nil
}

func (b *bucketStore) Delete(ctx context.Context, ruleID string) error {
	return b.store.Delete(ctx, ruleID)
}

type storeHelper struct {
	b *bucketStore
}

func (_ *storeHelper) SetGeneration(rule *settingsv1.RecordingRuleStore, generation int64) {
	rule.Generation = generation
}

func (_ *storeHelper) GetGeneration(rule *settingsv1.RecordingRuleStore) int64 {
	return rule.Generation
}

func (_ *storeHelper) FromStore(storeBytes json.RawMessage) (*settingsv1.RecordingRuleStore, error) {
	var store settingsv1.RecordingRuleStore
	err := protojson.Unmarshal(storeBytes, &store)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling json from store: %w", err)
	}

	return &store, nil
}

func (_ *storeHelper) ToStore(rule *settingsv1.RecordingRuleStore) (json.RawMessage, error) {
	return protojson.Marshal(rule)
}

func (_ *storeHelper) ID(rule *settingsv1.RecordingRuleStore) string {
	return rule.Id
}

func (_ *storeHelper) TypePath() string {
	return "settings/recording_rule.v1"
}
