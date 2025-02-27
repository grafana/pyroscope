package collection

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"text/template"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/grafana/alloy/syntax/parser"
	"github.com/thanos-io/objstore"
	"google.golang.org/protobuf/encoding/protojson"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/pkg/settings/store"
)

type storeHelper struct {
	b *bucketStore
}

func (_ *storeHelper) SetGeneration(r *settingsv1.GetCollectionRuleResponse, v int64) {
	r.Generation = v
}

func (_ *storeHelper) GetGeneration(r *settingsv1.GetCollectionRuleResponse) int64 {
	return r.Generation
}

func (h *storeHelper) FromStore(storeBytes json.RawMessage) (*settingsv1.GetCollectionRuleResponse, error) {
	var store settingsv1.CollectionRuleStore
	if err := protojson.Unmarshal(storeBytes, &store); err != nil {
		return nil, fmt.Errorf("error unmarshaling json from store: %w", err)
	}

	var api settingsv1.GetCollectionRuleResponse

	if err := h.b.convertFromStoreRule(&store, &api); err != nil {
		return nil, fmt.Errorf("error converting from store to API: %w", err)
	}

	return &api, nil
}

func (_ *storeHelper) ToStore(api *settingsv1.GetCollectionRuleResponse) (json.RawMessage, error) {
	var store settingsv1.CollectionRuleStore

	store.Name = api.Name
	store.Ebpf = api.Ebpf
	store.Java = api.Java
	store.Generation = api.Generation
	store.LastUpdated = api.LastUpdated
	store.Services = api.Services

	return protojson.Marshal(&store)
}

func (_ *storeHelper) ID(v *settingsv1.GetCollectionRuleResponse) string {
	return v.Name
}

func (_ *storeHelper) TypePath() string { return "settings/collection.v1" }

// write through cache for the bucket
type bucketStore struct {
	store             *store.GenericStore[*settingsv1.GetCollectionRuleResponse, *storeHelper]
	logger            log.Logger
	key               store.Key
	apiURL            string
	alloyTemplatePath string
}

func newBucketStore(logger log.Logger, bucket objstore.Bucket, key store.Key, apiURL string, alloyTemplatePath string) *bucketStore {
	bs := &bucketStore{
		key:               key,
		logger:            logger,
		apiURL:            apiURL,
		alloyTemplatePath: alloyTemplatePath,
	}

	bs.store = store.New[*settingsv1.GetCollectionRuleResponse, *storeHelper](
		logger, bucket, key, &storeHelper{b: bs},
	)
	return bs
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

type varSource interface {
	GetEbpf() *settingsv1.EBPFSettings
	GetJava() *settingsv1.JavaSettings
	GetServices() []*settingsv1.ServiceData
}

func (b *bucketStore) varsFromStore(s varSource) *pyroscopeVars {
	vars := &pyroscopeVars{
		PyroscopeURL:      b.apiURL,
		PyroscopeUsername: b.key.TenantID,
		EBPF:              &settingsv1.EBPFSettings{Enabled: false},
		Java:              &settingsv1.JavaSettings{Enabled: false},
	}
	if s := s.GetEbpf(); s != nil {
		vars.EBPF = s
	}
	if s := s.GetJava(); s != nil {
		vars.Java = s
	}

	// build rules
	services := s.GetServices()
	drops := make([]string, 0, len(services))
	for _, svc := range services {
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

func (b *bucketStore) template(s varSource) (string, error) {
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
	cfg, err := b.template(rule)
	if err != nil {
		return err
	}

	api := &settingsv1.GetCollectionRuleResponse{
		Name:          rule.Name,
		Services:      rule.Services,
		Ebpf:          rule.Ebpf,
		Java:          rule.Java,
		LastUpdated:   timeNow().UnixMilli(),
		Configuration: cfg,
	}

	return b.store.Upsert(
		ctx,
		api,
		rule.ObservedGeneration,
	)
}

func (b *bucketStore) get(ctx context.Context, name string) (*settingsv1.GetCollectionRuleResponse, error) {
	data, err := b.store.Get(ctx)
	if err != nil {
		return nil, err
	}

	for _, ruleAPI := range data.Elements {
		if ruleAPI.Name == name {
			return ruleAPI, nil
		}
	}

	return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no rule with name='%s' found", name))
}

func (b *bucketStore) convertFromStoreRule(store *settingsv1.CollectionRuleStore, api *settingsv1.GetCollectionRuleResponse) error {
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
	api.Generation = store.Generation

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

	return &settingsv1.ListCollectionRulesResponse{
		Rules:      data.Elements,
		Generation: data.Generation,
	}, nil
}
