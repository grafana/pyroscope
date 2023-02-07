// SPDX-License-Identifier: AGPL-3.0-only

package schedulerdiscovery

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/go-kit/log"

	"github.com/grafana/phlare/pkg/util"
)

const (
	ModeFlagName = "query-scheduler.service-discovery-mode"

	ModeDNS  = "dns"
	ModeRing = "ring"
)

var modes = []string{ModeDNS, ModeRing}

type Config struct {
	Mode             string     `yaml:"service_discovery_mode" category:"experimental" doc:"hidden"`
	SchedulerRing    RingConfig `yaml:"ring" doc:"hidden"`
	MaxUsedInstances int        `yaml:"max_used_instances" category:"experimental"`
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet, logger log.Logger) {
	f.StringVar(&cfg.Mode, ModeFlagName, ModeDNS, fmt.Sprintf("Service discovery mode that query-frontends and queriers use to find query-scheduler instances.%s Supported values are: %s.", sharedOptionWithRingClient, strings.Join(modes, ", ")))
	f.IntVar(&cfg.MaxUsedInstances, "query-scheduler.max-used-instances", 0, fmt.Sprintf("The maximum number of query-scheduler instances to use, regardless how many replicas are running. This option can be set only when -%s is set to '%s'. 0 to use all available query-scheduler instances.", ModeFlagName, ModeRing))
	cfg.SchedulerRing.RegisterFlags(f, logger)
}

func (cfg *Config) Validate() error {
	if !util.StringsContain(modes, cfg.Mode) {
		return fmt.Errorf("unsupported query-scheduler service discovery mode (supported values are: %s)", strings.Join(modes, ", "))
	}
	if cfg.MaxUsedInstances > 0 && cfg.Mode != ModeRing {
		return fmt.Errorf("the query-scheduler max used instances can be set only when -%s is set to '%s'", ModeFlagName, ModeRing)
	}
	if cfg.MaxUsedInstances < 0 {
		return errors.New("the query-scheduler max used instances can't be negative")
	}

	return nil
}
