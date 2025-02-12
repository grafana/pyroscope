package collection

import (
	"context"
	_ "embed"
	"fmt"
	"regexp"
	"strings"
	"text/template"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/alloy/syntax/parser"
	"github.com/thanos-io/objstore"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/pkg/settings/store"
)

// write through cache for the bucket
type bucketStore struct {
	store             *store.GenericStore[settingsv1.CollectionRulesStore, *settingsv1.CollectionRulesStore]
	logger            log.Logger
	key               store.Key
	apiURL            string
	alloyTemplatePath string
}

func newBucketStore(logger log.Logger, bucket objstore.Bucket, key store.Key, apiURL string, alloyTemplatePath string) *bucketStore {
	return &bucketStore{
		store: store.New[settingsv1.CollectionRulesStore, *settingsv1.CollectionRulesStore](
			logger, bucket, key, "settings/collection.v1",
		),
		key:               key,
		logger:            logger,
		apiURL:            apiURL,
		alloyTemplatePath: alloyTemplatePath,
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
		PyroscopeUsername: b.key.TenantID,
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

	// generate the pyroscope alloy config
	var (
		config         strings.Builder
		err            error
		configTemplate = template.New("config.alloy.gotempl")
	)

	if b.alloyTemplatePath != "" {
		configTemplate, err = configTemplate.ParseFiles(b.alloyTemplatePath)
	} else {
		configTemplate, err = configTemplate.Parse(alloyTemplate)
	}
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

// updates or create a new rule
func (b *bucketStore) upsertRule(ctx context.Context, rule *settingsv1.UpsertCollectionRuleRequest) error {
	return b.store.Update(ctx, func(data *settingsv1.CollectionRulesStore) error {
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

		data.Generation++

		return nil
	})

}

func (b *bucketStore) deleteRule(ctx context.Context, name string) error {
	return b.store.Update(ctx, func(data *settingsv1.CollectionRulesStore) error {

		// iterate over the rules to find the rule
		for idx, r := range data.Rules {
			if r.Name == name {
				// delete the rule
				data.Rules = append(data.Rules[:idx], data.Rules[idx+1:]...)

				// return early and save the ruleset
				return nil
			}
		}
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("no rule with name='%s' found", name))
	})
}

func (b *bucketStore) get(ctx context.Context, name string) (*settingsv1.GetCollectionRuleResponse, error) {
	data, err := b.store.Get(ctx)
	if err != nil {
		return nil, err
	}

	for _, ruleStore := range data.Rules {
		if ruleStore.Name == name {
			var ruleAPI settingsv1.GetCollectionRuleResponse
			if err := b.convertFromStoreRule(ctx, ruleStore, &ruleAPI); err != nil {
				return nil, err
			}
			return &ruleAPI, nil
		}
	}

	return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no rule with name='%s' found", name))
}

func (b *bucketStore) convertFromStoreRule(ctx context.Context, store *settingsv1.CollectionRuleStore, api *settingsv1.GetCollectionRuleResponse) error {
	config, err := b.template(store)
	if err != nil {
		return err
	}
	api.Name = store.Name
	api.Configuration = config
	api.Services = store.Services
	api.Ebpf = store.Ebpf
	api.Java = store.Java
	api.LastUpdated = store.LastUpdated

	if api.Ebpf == nil {
		api.Ebpf = &settingsv1.EBPFSettings{Enabled: false}
	}
	if api.Java == nil {
		api.Java = &settingsv1.JavaSettings{Enabled: false}
	}
	return nil
}

func (b *bucketStore) list(ctx context.Context) (*settingsv1.ListCollectionRulesResponse, error) {
	data, err := b.store.Get(ctx)
	if err != nil {
		return nil, err
	}

	result := &settingsv1.ListCollectionRulesResponse{
		Rules:      make([]*settingsv1.GetCollectionRuleResponse, 0, len(data.Rules)),
		Generation: data.Generation,
	}
	for _, ruleStore := range data.Rules {
		ruleAPI := &settingsv1.GetCollectionRuleResponse{
			Generation: data.Generation,
		}
		if err := b.convertFromStoreRule(ctx, ruleStore, ruleAPI); err != nil {
			return nil, err
		}
		result.Rules = append(result.Rules, ruleAPI)
	}

	return result, nil
}
