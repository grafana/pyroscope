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
	store  *store.GenericStore[*settingsv1.RecordingRule_Store, *storeHelper]
}

func (b *bucketStore) List(ctx context.Context) (*settingsv1.RecordingRules_Store, error) {
	var rules *settingsv1.RecordingRules_Store
	err := b.store.Read(ctx, func(ctx context.Context, c *store.Collection[*settingsv1.RecordingRule_Store]) error {
		rules = &settingsv1.RecordingRules_Store{
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

func (b *bucketStore) insert(ctx context.Context, newRule *settingsv1.RecordingRule_Store) (*settingsv1.RecordingRule_Store, error) {
	err := b.store.Update(ctx, func(ctx context.Context, c *store.Collection[*settingsv1.RecordingRule_Store]) error {
		for _, rule := range c.Elements {
			if rule.Id == newRule.Id {
				return fmt.Errorf("rule %s already exists", newRule.Id)
			}
		}

		c.Elements = append(c.Elements, newRule)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return newRule, nil
}

func (b *bucketStore) delete(ctx context.Context, ruleID string) error {
	return b.store.Delete(ctx, ruleID)
}

type storeHelper struct {
	b *bucketStore
}

func (_ *storeHelper) SetGeneration(rule *settingsv1.RecordingRule_Store, generation int64) {
	rule.Generation = generation
}

func (_ *storeHelper) GetGeneration(rule *settingsv1.RecordingRule_Store) int64 {
	return rule.Generation
}

func (_ *storeHelper) FromStore(storeBytes json.RawMessage) (*settingsv1.RecordingRule_Store, error) {
	var store settingsv1.RecordingRule_Store
	err := protojson.Unmarshal(storeBytes, &store)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling json from store: %w", err)
	}

	return &store, nil
}

func (_ *storeHelper) ToStore(rule *settingsv1.RecordingRule_Store) (json.RawMessage, error) {
	return protojson.Marshal(rule)
}

func (_ *storeHelper) ID(rule *settingsv1.RecordingRule_Store) string {
	return rule.Id
}

func (_ storeHelper) TypePath() string {
	return "settings/recording_rule.v1"
}
