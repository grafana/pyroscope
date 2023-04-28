// SPDX-License-Identifier: AGPL-3.0-only

package schedulerdiscovery

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/flagext"
	"github.com/grafana/dskit/kv"
	"github.com/grafana/dskit/netutil"
	"github.com/grafana/dskit/ring"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"

	util_log "github.com/grafana/phlare/pkg/util"
)

const (
	// ringKey is the key under which we store the query-schedulers ring in the KVStore.
	ringKey = "query-scheduler"

	// ringNumTokens is how many tokens each query-scheduler should have in the ring.
	// Query-schedulers use a ring for service-discovery so just 1 token is enough.
	ringNumTokens = 1

	// ringAutoForgetUnhealthyPeriods is how many consecutive timeout periods an unhealthy instance
	// in the ring will be automatically removed after.
	ringAutoForgetUnhealthyPeriods = 4

	// sharedOptionWithRingClient is a message appended to all config options that should be also
	// set on the components running the query-scheduler ring client.
	sharedOptionWithRingClient = " When query-scheduler ring-based service discovery is enabled, this option needs be set on query-schedulers, query-frontends and queriers."
)

// RingConfig masks the ring lifecycler config which contains
// many options not really required by the query-scheduler ring. This config
// is used to strip down the config to the minimum, and avoid confusion
// to the user.
type RingConfig struct {
	KVStore          kv.Config     `yaml:"kvstore" doc:"description=The key-value store used to share the hash ring across multiple instances. When query-scheduler ring-based service discovery is enabled, this option needs be set on query-schedulers, query-frontends and queriers."`
	HeartbeatPeriod  time.Duration `yaml:"heartbeat_period" category:"advanced"`
	HeartbeatTimeout time.Duration `yaml:"heartbeat_timeout" category:"advanced"`

	// Instance details
	InstanceID             string   `yaml:"instance_id" doc:"default=<hostname>" category:"advanced"`
	InstanceInterfaceNames []string `yaml:"instance_interface_names" doc:"default=[<private network interfaces>]"`
	InstancePort           int      `yaml:"instance_port" category:"advanced"`
	InstanceAddr           string   `yaml:"instance_addr" category:"advanced"`

	// Injected internally
	ListenPort int `yaml:"-"`
}

// RegisterFlags adds the flags required to config this to the given flag.FlagSet.
func (cfg *RingConfig) RegisterFlags(f *flag.FlagSet, logger log.Logger) {
	hostname, err := os.Hostname()
	if err != nil {
		level.Error(util_log.Logger).Log("msg", "failed to get hostname", "err", err)
		os.Exit(1)
	}

	// Ring flags
	cfg.KVStore.Store = "memberlist" // Override default value.
	cfg.KVStore.RegisterFlagsWithPrefix("query-scheduler.ring.", "collectors/", f)
	f.DurationVar(&cfg.HeartbeatPeriod, "query-scheduler.ring.heartbeat-period", 15*time.Second, "Period at which to heartbeat to the ring. 0 = disabled.")
	f.DurationVar(&cfg.HeartbeatTimeout, "query-scheduler.ring.heartbeat-timeout", time.Minute, "The heartbeat timeout after which query-schedulers are considered unhealthy within the ring."+sharedOptionWithRingClient)

	// Instance flags
	cfg.InstanceInterfaceNames = netutil.PrivateNetworkInterfacesWithFallback([]string{"eth0", "en0"}, logger)
	f.Var((*flagext.StringSlice)(&cfg.InstanceInterfaceNames), "query-scheduler.ring.instance-interface-names", "List of network interface names to look up when finding the instance IP address.")
	f.StringVar(&cfg.InstanceAddr, "query-scheduler.ring.instance-addr", "", "IP address to advertise in the ring. Default is auto-detected.")
	f.IntVar(&cfg.InstancePort, "query-scheduler.ring.instance-port", 0, "Port to advertise in the ring (defaults to -server.grpc-listen-port).")
	f.StringVar(&cfg.InstanceID, "query-scheduler.ring.instance-id", hostname, "Instance ID to register in the ring.")
}

// ToBasicLifecyclerConfig returns a ring.BasicLifecyclerConfig based on the query-scheduler ring config.
func (cfg *RingConfig) ToBasicLifecyclerConfig(logger log.Logger) (ring.BasicLifecyclerConfig, error) {
	instanceAddr, err := ring.GetInstanceAddr(cfg.InstanceAddr, cfg.InstanceInterfaceNames, logger)
	if err != nil {
		return ring.BasicLifecyclerConfig{}, err
	}

	instancePort := ring.GetInstancePort(cfg.InstancePort, cfg.ListenPort)

	return ring.BasicLifecyclerConfig{
		ID:                              cfg.InstanceID,
		Addr:                            fmt.Sprintf("%s:%d", instanceAddr, instancePort),
		HeartbeatPeriod:                 cfg.HeartbeatPeriod,
		HeartbeatTimeout:                cfg.HeartbeatTimeout,
		TokensObservePeriod:             0,
		NumTokens:                       ringNumTokens,
		KeepInstanceInTheRingOnShutdown: false,
	}, nil
}

// ToRingConfig returns a ring.Config based on the query-scheduler ring config.
func (cfg *RingConfig) ToRingConfig() ring.Config {
	rc := ring.Config{}
	flagext.DefaultValues(&rc)

	rc.KVStore = cfg.KVStore
	rc.HeartbeatTimeout = cfg.HeartbeatTimeout
	rc.ReplicationFactor = 1
	rc.SubringCacheDisabled = true

	return rc
}

// NewRingLifecycler creates a new query-scheduler ring lifecycler with all required lifecycler delegates.
func NewRingLifecycler(cfg RingConfig, logger log.Logger, reg prometheus.Registerer) (*ring.BasicLifecycler, error) {
	reg = prometheus.WrapRegistererWithPrefix("pyroscope_", reg)
	kvStore, err := kv.NewClient(cfg.KVStore, ring.GetCodec(), kv.RegistererWithKVName(reg, "query-scheduler-lifecycler"), logger)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize query-schedulers' KV store")
	}

	lifecyclerCfg, err := cfg.ToBasicLifecyclerConfig(logger)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build query-schedulers' lifecycler config")
	}

	var delegate ring.BasicLifecyclerDelegate
	delegate = ring.NewInstanceRegisterDelegate(ring.ACTIVE, ringNumTokens)
	delegate = ring.NewLeaveOnStoppingDelegate(delegate, logger)
	delegate = ring.NewAutoForgetDelegate(ringAutoForgetUnhealthyPeriods*cfg.HeartbeatTimeout, delegate, logger)

	lifecycler, err := ring.NewBasicLifecycler(lifecyclerCfg, "query-scheduler", ringKey, kvStore, delegate, logger, reg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize query-schedulers' lifecycler")
	}

	return lifecycler, nil
}

// NewRingClient creates a client for the query-schedulers ring.
func NewRingClient(cfg RingConfig, component string, logger log.Logger, reg prometheus.Registerer) (*ring.Ring, error) {
	client, err := ring.New(cfg.ToRingConfig(), component, ringKey, logger, prometheus.WrapRegistererWithPrefix("pyroscope_", reg))
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize query-schedulers' ring client")
	}

	return client, err
}
