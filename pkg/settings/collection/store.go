package collection

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/thanos-io/objstore"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
)

// write through cache for the bucket
type bucketStore struct {
	bucket objstore.Bucket
	key    hubKey
	path   string

	cacheLock sync.RWMutex
	cache     *settingsv1.CollectionRuleStore
}

func newBucketStore(bucket objstore.Bucket, key hubKey) *bucketStore {
	return &bucketStore{
		bucket: bucket,
		key:    key,
		path:   key.path() + ".json",
	}
}

// unsafeLoad reads from bucket into the cache, only call with write lock held
func (b *bucketStore) unsafeLoad(ctx context.Context) error {
	// fetch from bucket
	r, err := b.bucket.Get(ctx, b.path)
	if b.bucket.IsObjNotFoundErr(err) {
		b.cache = &settingsv1.CollectionRuleStore{}
		return nil
	} else if err != nil {
		return err
	}

	// unmarshal the data
	var data settingsv1.CollectionRuleStore
	if err := json.NewDecoder(r).Decode(&data); err != nil {
		return err
	}
	b.cache = &data

	return nil
}

// unsafeFlush writes from cache into the bucket, only call with write lock held
func (b *bucketStore) unsafeFlush(ctx context.Context) error {
	// update last modified time
	b.cache.LastUpdated = time.Now().UnixMilli()

	// marshal the data
	data, err := json.Marshal(b.cache)
	if err != nil {
		return err
	}

	return b.bucket.Upload(ctx, b.path, bytes.NewReader(data))
}

func (b *bucketStore) insertRule(ctx context.Context, rule *settingsv1.CollectionPayloadRuleInsert) error {
	// get write lock and fetch from bucket
	b.cacheLock.Lock()
	defer b.cacheLock.Unlock()

	// ensure we have the latest data
	if err := b.unsafeLoad(ctx); err != nil {
		return err
	}

	var (
		nextId   int64 = 1  // what id should the rule get
		position       = -1 // where should the rule be inserted
	)
	// iterate over the rules to find next id
	for idx, r := range b.cache.Rules {
		if r.Id >= nextId {
			nextId = r.Id + 1
		}

		// mark correct position
		if rule.After != nil && r.Id == *rule.After {
			position = idx
		}
	}

	if rule.After == nil {
		position = 0
	}

	if position == -1 && rule.After != nil {
		return fmt.Errorf("no rule with id %d found to insert after", *rule.After)
	}

	// set the id
	r := rule.Rule.CloneVT()
	r.Id = nextId

	// insert the rule
	b.cache.Rules = append(b.cache.Rules, nil)
	copy(b.cache.Rules[position+1:], b.cache.Rules[position:])
	b.cache.Rules[position] = r

	// save the ruleset
	return b.unsafeFlush(ctx)
}

func (b *bucketStore) deleteRule(ctx context.Context, id int64) error {
	// get write lock and fetch from bucket
	b.cacheLock.Lock()
	defer b.cacheLock.Unlock()

	// ensure we have the latest data
	if err := b.unsafeLoad(ctx); err != nil {
		return err
	}

	// iterate over the rules to find the rule
	for idx, r := range b.cache.Rules {
		if r.Id == id {
			// delete the rule
			b.cache.Rules = append(b.cache.Rules[:idx], b.cache.Rules[idx+1:]...)

			// save the ruleset
			return b.unsafeFlush(ctx)
		}
	}
	return fmt.Errorf("no rule with id %d found", id)

}

func (b *bucketStore) list(ctx context.Context) ([]*settingsv1.CollectionRule, error) {
	// serve from cache if available
	b.cacheLock.RLock()
	if b.cache != nil {
		defer b.cacheLock.RUnlock()
		return b.cache.Rules, nil
	}
	b.cacheLock.RUnlock()

	// get write lock and fetch from bucket
	b.cacheLock.Lock()
	defer b.cacheLock.Unlock()

	// check again if cache is available in the meantime
	if b.cache != nil {
		return b.cache.Rules, nil
	}

	// load from bucket
	if err := b.unsafeLoad(ctx); err != nil {
		return nil, err
	}

	return b.cache.Rules, nil
}
