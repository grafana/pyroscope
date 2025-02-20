package collection

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	connect "connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tenant"
	"github.com/thanos-io/objstore"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/settings/v1/settingsv1connect"
	"github.com/grafana/pyroscope/pkg/settings/store"
)

// allow to overide time for testing
var timeNow = time.Now

type Config struct {
	Enabled           bool   `yaml:"enabled"             category:"experimental"`
	PyroscopeURL      string `yaml:"pyroscope_url"       category:"experimental"` // required to be set when enabled is true
	AlloyTemplatePath string `yaml:"alloy_template_path" category:"experimental"`
}

const (
	flagPrefix            = "tenant-settings.collection-rules."
	flagEnabled           = flagPrefix + "enabled"
	flagPyroscopeURL      = flagPrefix + "pyroscope-url"
	flagAlloyTemplatePath = flagPrefix + "alloy-template-path"
)

func (cfg *Config) RegisterFlags(fs *flag.FlagSet) {
	fs.BoolVar(
		&cfg.Enabled,
		flagEnabled,
		false,
		"Enable the storing of collection config in tenant settings.",
	)
	fs.StringVar(
		&cfg.PyroscopeURL,
		flagPyroscopeURL,
		"",
		"The public facing URL of the Pyroscope instance.",
	)
	fs.StringVar(
		&cfg.AlloyTemplatePath,
		flagAlloyTemplatePath,
		"",
		"Override the default alloy go template.",
	)
}

func (cfg *Config) Validate() error {
	if !cfg.Enabled {
		return nil
	}

	if cfg.PyroscopeURL == "" {
		return fmt.Errorf(
			"%s is required when %s is set",
			flagPyroscopeURL,
			flagEnabled,
		)
	}

	if cfg.AlloyTemplatePath != "" {
		if _, err := os.ReadFile(cfg.AlloyTemplatePath); err != nil {
			return fmt.Errorf(
				"%s is not readable: %w",
				flagAlloyTemplatePath,
				err,
			)
		}
	}

	return nil
}

// Collection handles the communication with Grafana Alloy, and ensures that subscribed instance received updates to rules.
// For each tenant and scope a new hub is created.
type Collection struct {
	cfg    Config
	bucket objstore.Bucket
	logger log.Logger

	lck    sync.RWMutex
	stores map[store.Key]*bucketStore
}

func New(cfg Config, bucket objstore.Bucket, logger log.Logger) *Collection {
	return &Collection{
		cfg:    cfg,
		bucket: bucket,
		logger: logger,
		stores: make(map[store.Key]*bucketStore),
	}
}

var (
	validRuleName = regexp.MustCompile(`^[a-z0-9-]+$`)
)

func isValidRuleName(n string) error {
	if !validRuleName.MatchString(n) {
		return fmt.Errorf("invalid name '%s', must match %s", n, validRuleName)
	}
	return nil
}

func isValidServiceName(n string) error {
	if strings.ContainsRune(n, '`') {
		return fmt.Errorf("invalid service name '%s', must not contain '`'", n)
	}
	return nil
}

var _ settingsv1connect.CollectionRulesServiceHandler = &Collection{}

func (c *Collection) storeForTenant(ctx context.Context) (*bucketStore, error) {
	tenantID, err := tenant.TenantID(ctx)
	if err != nil {
		level.Error(c.logger).Log("error getting tenant ID", "err", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	k := store.Key{TenantID: tenantID}

	c.lck.RLock()
	s, ok := c.stores[k]
	c.lck.RUnlock()
	if ok {
		return s, nil
	}

	c.lck.Lock()
	defer c.lck.Unlock()
	// Try again, holiding the write lock
	s, ok = c.stores[k]
	if ok {
		return s, nil
	}

	// now create a new store
	s = newBucketStore(
		log.With(c.logger, "tenant", tenantID),
		c.bucket,
		k,
		c.cfg.PyroscopeURL,
		c.cfg.AlloyTemplatePath,
	)
	c.stores[k] = s
	return s, nil

}

func validateRequest(o any) error {
	var errs []error

	// check for name, if it exists
	if f, ok := o.(interface {
		GetName() string
	}); ok {
		errs = append(errs, isValidRuleName(f.GetName()))
	}

	// check for service names if they exist
	if f, ok := o.(interface {
		GetServices() []*settingsv1.ServiceData
	}); ok {
		for _, svc := range f.GetServices() {
			errs = append(errs, isValidServiceName(svc.GetName()))
		}
	}

	return errors.Join(errs...)
}

func (c *Collection) GetCollectionRule(
	ctx context.Context,
	req *connect.Request[settingsv1.GetCollectionRuleRequest],
) (*connect.Response[settingsv1.GetCollectionRuleResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	s, err := c.storeForTenant(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := s.get(ctx, req.Msg.Name)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(resp), nil
}

func (c *Collection) UpsertCollectionRule(
	ctx context.Context,
	req *connect.Request[settingsv1.UpsertCollectionRuleRequest],
) (*connect.Response[settingsv1.GetCollectionRuleResponse], error) {

	if err := validateRequest(req.Msg); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	s, err := c.storeForTenant(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.upsertRule(ctx, req.Msg); err != nil {
		var cErr *store.ErrConflictGeneration
		if errors.As(err, &cErr) {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("Conflicting update, please try again"))
		}
		return nil, err
	}
	resp, err := s.get(ctx, req.Msg.Name)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(resp), nil
}

func (c *Collection) ListCollectionRules(
	ctx context.Context,
	_ *connect.Request[settingsv1.ListCollectionRulesRequest],
) (*connect.Response[settingsv1.ListCollectionRulesResponse], error) {

	s, err := c.storeForTenant(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := s.list(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(resp), nil
}

func (c *Collection) DeleteCollectionRule(
	ctx context.Context,
	req *connect.Request[settingsv1.DeleteCollectionRuleRequest],
) (*connect.Response[settingsv1.DeleteCollectionRuleResponse], error) {

	if err := validateRequest(req.Msg); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	s, err := c.storeForTenant(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.store.Delete(ctx, req.Msg.Name); err != nil {
		if err == store.ErrElementNotFound {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no rule with name='%s' found", req.Msg.Name))
		}
		return nil, err
	}

	return connect.NewResponse(&settingsv1.DeleteCollectionRuleResponse{}), nil
}
