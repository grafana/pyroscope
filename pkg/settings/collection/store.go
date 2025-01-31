package collection

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"text/template"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/alloy/syntax/parser"
	"github.com/thanos-io/objstore"
	"google.golang.org/protobuf/encoding/protojson"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
)

type storeKey struct {
	tenantID string
}

func (k storeKey) path() string {
	return fmt.Sprintf("%s/settings/collection.v1", k.tenantID)
}

// write through cache for the bucket
type bucketStore struct {
	logger log.Logger
	bucket objstore.Bucket
	apiURL string
	key    storeKey
	path   string

	cacheLock sync.RWMutex
	cache     *settingsv1.ListCollectionRulesResponse
}

func newBucketStore(logger log.Logger, bucket objstore.Bucket, key storeKey, apiURL string) *bucketStore {
	return &bucketStore{
		logger: logger,
		bucket: bucket,
		key:    key,
		apiURL: apiURL,
		path:   key.path() + ".json",
	}
}

type pyroscopeVars struct {
	PyroscopeURL      string
	PyroscopeUsername string
	PyroscopeRules    []pyroscopeRule
	EBPF              *settingsv1.EBPFSettings
	Java              *settingsv1.JavaSettings
}

type pyroscopeRule struct {
	Action       string
	Regex        string
	SourceLabels string
}

func (b *bucketStore) varsFromStore(s *settingsv1.CollectionRuleStore) *pyroscopeVars {
	vars := &pyroscopeVars{
		PyroscopeURL:      b.apiURL,
		PyroscopeUsername: b.key.tenantID,
		EBPF:              &settingsv1.EBPFSettings{Enabled: false},
		Java:              &settingsv1.JavaSettings{Enabled: false},
	}
	if s.Ebpf != nil {
		vars.EBPF = s.Ebpf
	}
	if s.Java != nil {
		vars.Java = s.Java
	}

	// build rules
	drops := make([]string, 0, len(s.Services))
	for _, svc := range s.Services {
		if !svc.Enabled {
			drops = append(drops, regexp.QuoteMeta(svc.Name))
		}
	}
	if len(drops) > 0 {
		vars.PyroscopeRules = append(vars.PyroscopeRules, pyroscopeRule{
			Action:       "drop",
			Regex:        strings.Join(drops, "|"),
			SourceLabels: `["service_name"]`,
		})
	}

	return vars

}

//go:embed config.alloy.gotempl
var alloyTemplate string

func (b *bucketStore) template(s *settingsv1.CollectionRuleStore) (string, error) {
	vars := b.varsFromStore(s)

	// generate the pyroscope config
	var config strings.Builder

	configTemplate, err := template.New("config.alloy.gotempl").Parse(alloyTemplate)
	if err != nil {
		return "", fmt.Errorf("error parsing alloy collection template: %w", err)
	}

	if err := configTemplate.Execute(&config, vars); err != nil {
		return "", fmt.Errorf("error executing alloy collection template: %w", err)
	}

	if _, err := parser.ParseFile("", []byte(config.String())); err != nil {
		return "", fmt.Errorf("error parsing result of collection template: %w", err)
	}

	return config.String(), nil
}

// get reads the store object from bucket
func (b *bucketStore) getStore(ctx context.Context) (*settingsv1.CollectionRulesStore, error) {
	// fetch from bucket
	r, err := b.bucket.Get(ctx, b.path)
	if b.bucket.IsObjNotFoundErr(err) {
		return &settingsv1.CollectionRulesStore{}, nil
	}
	if err != nil {
		return nil, err
	}

	bodyJson, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	// unmarshal the data
	var data settingsv1.CollectionRulesStore
	if err := protojson.Unmarshal(bodyJson, &data); err != nil {
		return nil, err
	}

	return &data, nil
}

// unsafeLoad reads from bucket into the cache, only call with write lock held
func (b *bucketStore) unsafeLoadCache(ctx context.Context) error {
	// fetch from bucket
	data, err := b.getStore(ctx)
	if err != nil {
		return err
	}

	b.cache = &settingsv1.ListCollectionRulesResponse{
		Rules:      make([]*settingsv1.GetCollectionRuleResponse, 0, len(data.Rules)),
		Generation: data.Generation,
	}
	for _, r := range data.Rules {
		config, err := b.template(r)
		if err != nil {
			return err
		}
		ruleGenerated := &settingsv1.GetCollectionRuleResponse{
			Name:          r.Name,
			Services:      r.Services,
			Ebpf:          r.Ebpf,
			Java:          r.Java,
			Generation:    data.Generation,
			LastUpdated:   r.LastUpdated,
			Configuration: config,
		}
		if ruleGenerated.Ebpf == nil {
			ruleGenerated.Ebpf = &settingsv1.EBPFSettings{Enabled: false}
		}
		if ruleGenerated.Java == nil {
			ruleGenerated.Java = &settingsv1.JavaSettings{Enabled: false}
		}
		b.cache.Rules = append(b.cache.Rules, ruleGenerated)
	}

	return nil
}

// unsafeFlush writes from arguments into the bucket and then reset cache. Only call with write lock held
func (b *bucketStore) unsafeFlush(ctx context.Context, data *settingsv1.CollectionRulesStore) error {
	// increment generation
	data.Generation++

	// marshal the data
	dataJson, err := protojson.Marshal(data)
	if err != nil {
		return err
	}

	// reset cache
	b.cache = nil

	// write to bucket
	return b.bucket.Upload(ctx, b.path, bytes.NewReader(dataJson))
}

// updates or create a new rule
func (b *bucketStore) upsertRule(ctx context.Context, rule *settingsv1.UpsertCollectionRuleRequest) error {
	// get write lock and fetch from bucket
	b.cacheLock.Lock()
	defer b.cacheLock.Unlock()

	// ensure we have the latest data
	data, err := b.getStore(ctx)
	if err != nil {
		return err
	}

	// iterate over the store list to find the rule
	pos := -1
	for idx, r := range data.Rules {
		if r.Name == rule.Name {
			pos = idx
		}
	}

	if pos == -1 {
		// create a new rule
		pos = len(data.Rules)
		data.Rules = append(data.Rules, &settingsv1.CollectionRuleStore{})
	} else {
		// check if there had been a conflicted updated
		storedRule := data.Rules[pos]
		if rule.ObservedLastUpdated != nil && *rule.ObservedLastUpdated != storedRule.LastUpdated {
			level.Warn(b.logger).Log(
				"msg", "conflicting update, please try again",
				"observed_last_updated", time.UnixMilli(*rule.ObservedLastUpdated),
				"stored_last_updated", time.UnixMilli(storedRule.LastUpdated),
			)
			return connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("Conflicting update, please try again"))
		}
	}

	data.Rules[pos].Name = rule.Name
	data.Rules[pos].Services = rule.Services
	data.Rules[pos].Ebpf = rule.Ebpf
	data.Rules[pos].Java = rule.Java
	data.Rules[pos].LastUpdated = timeNow().UnixMilli()

	// save the ruleset
	return b.unsafeFlush(ctx, data)
}

func (b *bucketStore) deleteRule(ctx context.Context, name string) error {
	// get write lock and fetch from bucket
	b.cacheLock.Lock()
	defer b.cacheLock.Unlock()

	// ensure we have the latest data
	data, err := b.getStore(ctx)
	if err != nil {
		return err
	}

	// iterate over the rules to find the rule
	for idx, r := range data.Rules {
		if r.Name == name {
			// delete the rule
			data.Rules = append(data.Rules[:idx], data.Rules[idx+1:]...)

			// save the ruleset
			return b.unsafeFlush(ctx, data)
		}
	}
	return connect.NewError(connect.CodeNotFound, fmt.Errorf("no rule with name='%s' found", name))

}

func (b *bucketStore) get(ctx context.Context, name string) (*settingsv1.GetCollectionRuleResponse, error) {
	data, err := b.list(ctx)
	if err != nil {
		return nil, err
	}

	for _, r := range data.Rules {
		if r.Name == name {
			return r, nil
		}
	}

	return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no rule with name='%s' found", name))
}

func (b *bucketStore) list(ctx context.Context) (*settingsv1.ListCollectionRulesResponse, error) {
	// serve from cache if available
	b.cacheLock.RLock()
	if b.cache != nil {
		b.cacheLock.RUnlock()
		return b.cache, nil
	}
	b.cacheLock.RUnlock()

	// get write lock and fetch from bucket
	b.cacheLock.Lock()
	defer b.cacheLock.Unlock()

	// check again if cache is available in the meantime
	if b.cache != nil {
		return b.cache, nil
	}

	// load from bucket
	if err := b.unsafeLoadCache(ctx); err != nil {
		return nil, err
	}

	return b.cache, nil
}
