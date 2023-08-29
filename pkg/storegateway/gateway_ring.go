package storegateway

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
)

const (
	// RingKey is the key under which we store the store gateways ring in the KVStore.
	RingKey = "store-gateway"

	// RingNameForServer is the name of the ring used by the store gateway server.
	RingNameForServer = "store-gateway"

	// RingNameForClient is the name of the ring used by the store gateway client (we need
	// a different name to avoid clashing Prometheus metrics when running in single-binary).
	RingNameForClient = "store-gateway-client"

	// We use a safe default instead of exposing to config option to the user
	// in order to simplify the config.
	RingNumTokens = 512

	// sharedOptionWithRingClient is a message appended to all config options that should be also
	// set on the components running the store-gateway ring client.
	sharedOptionWithRingClient = " This option needs be set both on the store-gateway, querier and ruler when running in microservices mode."
)

var (
	// BlocksOwnerSync is the operation used to check the authoritative owners of a block
	// (replicas included).
	BlocksOwnerSync = ring.NewOp([]ring.InstanceState{ring.JOINING, ring.ACTIVE, ring.LEAVING}, nil)

	// BlocksOwnerRead is the operation used to check the authoritative owners of a block
	// (replicas included) that are available for queries (a store-gateway is available for
	// queries only when ACTIVE).
	BlocksOwnerRead = ring.NewOp([]ring.InstanceState{ring.ACTIVE}, nil)

	// BlocksRead is the operation run by the querier to query blocks via the store-gateway.
	BlocksRead = ring.NewOp([]ring.InstanceState{ring.ACTIVE}, func(s ring.InstanceState) bool {
		// Blocks can only be queried from ACTIVE instances. However, if the block belongs to
		// a non-active instance, then we should extend the replication set and try to query it
		// from the next ACTIVE instance in the ring (which is expected to have it because a
		// store-gateway keeps their previously owned blocks until new owners are ACTIVE).
		return s != ring.ACTIVE
	})
)

type RingConfig struct {
	KVStore              kv.Config     `yaml:"kvstore" doc:"description=The key-value store used to share the hash ring across multiple instances. This option needs be set both on the store-gateway, querier and ruler when running in microservices mode."`
	HeartbeatPeriod      time.Duration `yaml:"heartbeat_period" category:"advanced"`
	HeartbeatTimeout     time.Duration `yaml:"heartbeat_timeout" category:"advanced"`
	ReplicationFactor    int           `yaml:"replication_factor" category:"advanced"`
	TokensFilePath       string        `yaml:"tokens_file_path"`
	ZoneAwarenessEnabled bool          `yaml:"zone_awareness_enabled"`

	// Wait ring stability.
	WaitStabilityMinDuration time.Duration `yaml:"wait_stability_min_duration" category:"advanced"`
	WaitStabilityMaxDuration time.Duration `yaml:"wait_stability_max_duration" category:"advanced"`

	// Instance details
	InstanceID             string   `yaml:"instance_id" doc:"default=<hostname>" category:"advanced"`
	InstanceInterfaceNames []string `yaml:"instance_interface_names" doc:"default=[<private network interfaces>]"`
	InstancePort           int      `yaml:"instance_port" category:"advanced"`
	InstanceAddr           string   `yaml:"instance_addr" category:"advanced"`
	EnableIPv6             bool     `yaml:"instance_enable_ipv6" category:"advanced"`
	InstanceZone           string   `yaml:"instance_availability_zone"`

	UnregisterOnShutdown bool `yaml:"unregister_on_shutdown"`

	// Injected internally
	ListenPort      int           `yaml:"-"`
	RingCheckPeriod time.Duration `yaml:"-"`
}

// RegisterFlags adds the flags required to config this to the given FlagSet
func (cfg *RingConfig) RegisterFlags(f *flag.FlagSet, logger log.Logger) {
	hostname, err := os.Hostname()
	if err != nil {
		level.Error(logger).Log("msg", "failed to get hostname", "err", err)
		os.Exit(1)
	}

	ringFlagsPrefix := "store-gateway.sharding-ring."

	// Ring flags
	cfg.KVStore.Store = "memberlist"
	cfg.KVStore.RegisterFlagsWithPrefix(ringFlagsPrefix, "collectors/", f)
	f.DurationVar(&cfg.HeartbeatPeriod, ringFlagsPrefix+"heartbeat-period", 15*time.Second, "Period at which to heartbeat to the ring. 0 = disabled.")
	f.DurationVar(&cfg.HeartbeatTimeout, ringFlagsPrefix+"heartbeat-timeout", time.Minute, "The heartbeat timeout after which store gateways are considered unhealthy within the ring. 0 = never (timeout disabled)."+sharedOptionWithRingClient)
	f.IntVar(&cfg.ReplicationFactor, ringFlagsPrefix+"replication-factor", 1, "The replication factor to use when sharding blocks."+sharedOptionWithRingClient)
	f.StringVar(&cfg.TokensFilePath, ringFlagsPrefix+"tokens-file-path", "", "File path where tokens are stored. If empty, tokens are not stored at shutdown and restored at startup.")
	f.BoolVar(&cfg.ZoneAwarenessEnabled, ringFlagsPrefix+"zone-awareness-enabled", false, "True to enable zone-awareness and replicate blocks across different availability zones."+sharedOptionWithRingClient)

	// Wait stability flags.
	f.DurationVar(&cfg.WaitStabilityMinDuration, ringFlagsPrefix+"wait-stability-min-duration", 0, "Minimum time to wait for ring stability at startup, if set to positive value.")
	f.DurationVar(&cfg.WaitStabilityMaxDuration, ringFlagsPrefix+"wait-stability-max-duration", 5*time.Minute, "Maximum time to wait for ring stability at startup. If the store-gateway ring keeps changing after this period of time, the store-gateway will start anyway.")

	// Instance flags
	cfg.InstanceInterfaceNames = netutil.PrivateNetworkInterfacesWithFallback([]string{"eth0", "en0"}, logger)
	f.Var((*flagext.StringSlice)(&cfg.InstanceInterfaceNames), ringFlagsPrefix+"instance-interface-names", "List of network interface names to look up when finding the instance IP address.")
	f.StringVar(&cfg.InstanceAddr, ringFlagsPrefix+"instance-addr", "", "IP address to advertise in the ring. Default is auto-detected.")
	f.IntVar(&cfg.InstancePort, ringFlagsPrefix+"instance-port", 0, "Port to advertise in the ring (defaults to -server.grpc-listen-port).")
	f.StringVar(&cfg.InstanceID, ringFlagsPrefix+"instance-id", hostname, "Instance ID to register in the ring.")
	f.BoolVar(&cfg.EnableIPv6, ringFlagsPrefix+"instance-enable-ipv6", false, "Enable using a IPv6 instance address. (default false)")
	f.StringVar(&cfg.InstanceZone, ringFlagsPrefix+"instance-availability-zone", "", "The availability zone where this instance is running. Required if zone-awareness is enabled.")

	f.BoolVar(&cfg.UnregisterOnShutdown, ringFlagsPrefix+"unregister-on-shutdown", true, "Unregister from the ring upon clean shutdown.")

	// Defaults for internal settings.
	cfg.RingCheckPeriod = 5 * time.Second
}

func (cfg *RingConfig) ToRingConfig() ring.Config {
	rc := ring.Config{}
	flagext.DefaultValues(&rc)

	rc.KVStore = cfg.KVStore
	rc.HeartbeatTimeout = cfg.HeartbeatTimeout
	rc.ReplicationFactor = cfg.ReplicationFactor
	rc.ZoneAwarenessEnabled = cfg.ZoneAwarenessEnabled
	rc.SubringCacheDisabled = true

	return rc
}

func (cfg *RingConfig) ToLifecyclerConfig(logger log.Logger) (ring.BasicLifecyclerConfig, error) {
	instanceAddr, err := ring.GetInstanceAddr(cfg.InstanceAddr, cfg.InstanceInterfaceNames, logger, cfg.EnableIPv6)
	if err != nil {
		return ring.BasicLifecyclerConfig{}, err
	}

	instancePort := ring.GetInstancePort(cfg.InstancePort, cfg.ListenPort)

	return ring.BasicLifecyclerConfig{
		ID:                              cfg.InstanceID,
		Addr:                            fmt.Sprintf("%s:%d", instanceAddr, instancePort),
		Zone:                            cfg.InstanceZone,
		HeartbeatPeriod:                 cfg.HeartbeatPeriod,
		HeartbeatTimeout:                cfg.HeartbeatTimeout,
		TokensObservePeriod:             0,
		NumTokens:                       RingNumTokens,
		KeepInstanceInTheRingOnShutdown: !cfg.UnregisterOnShutdown,
	}, nil
}
