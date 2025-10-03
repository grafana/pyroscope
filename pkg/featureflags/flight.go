package featureflags

import (
	"flag"
	"time"
)

// ArrowFlightConfig configures Arrow Flight integration
type ArrowFlightConfig struct {
	Enabled bool `yaml:"enabled" category:"advanced"`

	// Server configuration
	Server ServerFlightConfig `yaml:"server" category:"advanced"`

	// Client configuration
	Client ClientFlightConfig `yaml:"client" category:"advanced"`
}

// ServerFlightConfig configures Arrow Flight server
type ServerFlightConfig struct {
	Address string `yaml:"address" category:"advanced"`
	Port    int    `yaml:"port" category:"advanced"`
}

// ClientFlightConfig configures Arrow Flight client
type ClientFlightConfig struct {
	Enabled     bool          `yaml:"enabled" category:"advanced"`
	Addresses   string        `yaml:"addresses" category:"advanced"`
	Timeout     time.Duration `yaml:"timeout" category:"advanced"`
	TLS         bool          `yaml:"tls" category:"advanced"`
	TLSInsecure bool          `yaml:"tls_insecure" category:"advanced"`
}

func (c *ArrowFlightConfig) RegisterFlags(f *flag.FlagSet) {
	f.BoolVar(&c.Enabled, "arrow-flight.enabled", false, "Enable Arrow Flight integration instead of ConnectRPC")
	f.StringVar(&c.Server.Address, "arrow-flight.server.address", "0.0.0.0", "Arrow Flight server bind address")
	f.IntVar(&c.Server.Port, "arrow-flight.server.port", 8086, "Arrow Flight server port")
	f.BoolVar(&c.Client.Enabled, "arrow-flight.client.enabled", true, "Enable Arrow Flight client")
	f.StringVar(&c.Client.Addresses, "arrow-flight.client.addresses", "", "Arrow Flight server addresses (comma-separated)")
	f.DurationVar(&c.Client.Timeout, "arrow-flight.client.timeout", 30*time.Second, "Arrow Flight client timeout")
	f.BoolVar(&c.Client.TLS, "arrow-flight.client.tls", false, "Use TLS for Arrow Flight client")
	f.BoolVar(&c.Client.TLSInsecure, "arrow-flight.client.tls_insecure", false, "Skip TLS verification for Arrow Flight client")
}

// IsArrowFlightEnabled returns true if Arrow Flight is enabled
func IsArrowFlightEnabled(cfg ArrowFlightConfig) bool {
	return cfg.Enabled
}
