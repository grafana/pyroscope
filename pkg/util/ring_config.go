// SPDX-License-Identifier: AGPL-3.0-only

package util

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

// CommonRingConfig is the configuration commonly used by components that use a ring
// for various coordination tasks such as sharding or service discovery.
type CommonRingConfig struct {
	// KV store details
	KVStore          kv.Config     `yaml:"kvstore" doc:"description=The key-value store used to share the hash ring across multiple instances."`
	HeartbeatPeriod  time.Duration `yaml:"heartbeat_period" category:"advanced"`
	HeartbeatTimeout time.Duration `yaml:"heartbeat_timeout" category:"advanced"`

	// Instance details
	InstanceID             string   `yaml:"instance_id" doc:"default=<hostname>" category:"advanced"`
	InstanceInterfaceNames []string `yaml:"instance_interface_names" doc:"default=[<private network interfaces>]"`
	InstancePort           int      `yaml:"instance_port" category:"advanced"`
	InstanceAddr           string   `yaml:"instance_addr" category:"advanced"`
	EnableIPv6             bool     `yaml:"instance_enable_ipv6" category:"advanced"`

	// Injected internally
	ListenPort int `yaml:"-"`
}

// RegisterFlags adds the flags required to config this to the given FlagSet
func (cfg *CommonRingConfig) RegisterFlags(flagPrefix, kvStorePrefix, componentPlural string, f *flag.FlagSet, logger log.Logger) {
	hostname, err := os.Hostname()
	if err != nil {
		level.Error(logger).Log("msg", "failed to get hostname", "err", err)
		os.Exit(1)
	}

	// Ring flags
	cfg.KVStore.Store = "memberlist"
	cfg.KVStore.RegisterFlagsWithPrefix(flagPrefix, kvStorePrefix, f)
	f.DurationVar(&cfg.HeartbeatPeriod, flagPrefix+"heartbeat-period", 15*time.Second, "Period at which to heartbeat to the ring. 0 = disabled.")
	f.DurationVar(&cfg.HeartbeatTimeout, flagPrefix+"heartbeat-timeout", time.Minute, fmt.Sprintf("The heartbeat timeout after which %s are considered unhealthy within the ring. 0 = never (timeout disabled).", componentPlural))

	// Instance flags
	cfg.InstanceInterfaceNames = netutil.PrivateNetworkInterfacesWithFallback([]string{"eth0", "en0"}, logger)
	f.Var((*flagext.StringSlice)(&cfg.InstanceInterfaceNames), flagPrefix+"instance-interface-names", "List of network interface names to look up when finding the instance IP address.")
	f.StringVar(&cfg.InstanceAddr, flagPrefix+"instance-addr", "", "IP address to advertise in the ring. Default is auto-detected.")
	f.IntVar(&cfg.InstancePort, flagPrefix+"instance-port", 0, "Port to advertise in the ring (defaults to -server.http-listen-port).")
	f.StringVar(&cfg.InstanceID, flagPrefix+"instance-id", hostname, "Instance ID to register in the ring.")
	f.BoolVar(&cfg.EnableIPv6, flagPrefix+"instance-enable-ipv6", false, "Enable using a IPv6 instance address. (default false)")
}

func (cfg *CommonRingConfig) ToRingConfig() ring.Config {
	rc := ring.Config{}
	flagext.DefaultValues(&rc)

	rc.KVStore = cfg.KVStore
	rc.HeartbeatTimeout = cfg.HeartbeatTimeout
	return rc
}
