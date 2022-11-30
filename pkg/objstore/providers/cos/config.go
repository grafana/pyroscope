package cos

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Config encapsulates the necessary config values to instantiate an cos client.
type Config struct {
	Bucket    string     `yaml:"bucket"`
	Region    string     `yaml:"region"`
	AppID     string     `yaml:"app_id"`
	Endpoint  string     `yaml:"endpoint"`
	SecretKey string     `yaml:"secret_key"`
	SecretID  string     `yaml:"secret_id"`
	HTTP      HTTPConfig `yaml:"http"`
}

// Validate validates cos client config and returns error on failure
func (c *Config) Validate() error {
	if len(c.Endpoint) != 0 {
		if _, err := url.Parse(c.Endpoint); err != nil {
			return fmt.Errorf("cos config: failed to parse endpoint: %w", err)
		}

		if empty(c.SecretKey) || empty(c.SecretID) {
			return errors.New("secret id and secret key cannot be empty")
		}
		return nil
	}

	if empty(c.Bucket) || empty(c.AppID) || empty(c.Region) || empty(c.SecretID) || empty(c.SecretKey) {
		return errors.New("invalid cos configuration, bucket, app_id, region, secret_id and secret_key must be set")
	}
	return nil
}

func empty(s string) bool {
	return len(s) == 0
}

// RegisterFlags registers the flags for COS storage
func (c *Config) RegisterFlags(f *flag.FlagSet) {
	c.RegisterFlagsWithPrefix("", f)
}

// RegisterFlagsWithPrefix register the flags for COS storage with provided prefix
func (c *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.StringVar(&c.Bucket, prefix+"cos.bucket", "", "COS bucket name")
	f.StringVar(&c.Region, prefix+"cos.region", "", "COS region name")
	f.StringVar(&c.AppID, prefix+"cos.app-id", "", "COS app id")
	f.StringVar(&c.Endpoint, prefix+"cos.endpoint", "", "COS storage endpoint")
	f.StringVar(&c.SecretID, prefix+"cos.secret-id", "", "COS secret id")
	f.StringVar(&c.SecretKey, prefix+"cos.secret-key", "", "COS secret key")
	c.HTTP.RegisterFlagsWithPrefix(prefix, f)
}

// HTTPConfig stores the http.Transport configuration for the COS client.
type HTTPConfig struct {
	IdleConnTimeout       time.Duration `yaml:"idle_conn_timeout" category:"advanced"`
	ResponseHeaderTimeout time.Duration `yaml:"response_header_timeout" category:"advanced"`
	InsecureSkipVerify    bool          `yaml:"insecure_skip_verify" category:"advanced"`
	TLSHandshakeTimeout   time.Duration `yaml:"tls_handshake_timeout" category:"advanced"`
	ExpectContinueTimeout time.Duration `yaml:"expect_continue_timeout" category:"advanced"`
	MaxIdleConns          int           `yaml:"max_idle_connections" category:"advanced"`
	MaxIdleConnsPerHost   int           `yaml:"max_idle_connections_per_host" category:"advanced"`
	MaxConnsPerHost       int           `yaml:"max_connections_per_host" category:"advanced"`

	// Allow upstream callers to inject a round tripper
	Transport http.RoundTripper `yaml:"-"`
}

// RegisterFlagsWithPrefix registers the flags for COS storage with the provided prefix
func (cfg *HTTPConfig) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.DurationVar(&cfg.IdleConnTimeout, prefix+"cos.http.idle-conn-timeout", 90*time.Second, "The time an idle connection will remain idle before closing.")
	f.DurationVar(&cfg.ResponseHeaderTimeout, prefix+"cos.http.response-header-timeout", 2*time.Minute, "The amount of time the client will wait for a servers response headers.")
	f.BoolVar(&cfg.InsecureSkipVerify, prefix+"cos.http.insecure-skip-verify", false, "If the client connects to COS via HTTPS and this option is enabled, the client will accept any certificate and hostname.")
	f.DurationVar(&cfg.TLSHandshakeTimeout, prefix+"cos.tls-handshake-timeout", 10*time.Second, "Maximum time to wait for a TLS handshake. 0 means no limit.")
	f.DurationVar(&cfg.ExpectContinueTimeout, prefix+"cos.expect-continue-timeout", 1*time.Second, "The time to wait for a server's first response headers after fully writing the request headers if the request has an Expect header. 0 to send the request body immediately.")
	f.IntVar(&cfg.MaxIdleConns, prefix+"cos.max-idle-connections", 100, "Maximum number of idle (keep-alive) connections across all hosts. 0 means no limit.")
	f.IntVar(&cfg.MaxIdleConnsPerHost, prefix+"cos.max-idle-connections-per-host", 100, "Maximum number of idle (keep-alive) connections to keep per-host. If 0, a built-in default value is used.")
	f.IntVar(&cfg.MaxConnsPerHost, prefix+"cos.max-connections-per-host", 0, "Maximum number of connections per host. 0 means no limit.")
}
