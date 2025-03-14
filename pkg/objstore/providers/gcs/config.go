// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/bucket/gcs/config.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package gcs

import (
	"flag"
	"time"

	"github.com/grafana/dskit/flagext"
)

// Config holds the config options for GCS backend
type Config struct {
	BucketName     string         `yaml:"bucket_name"`
	ServiceAccount flagext.Secret `yaml:"service_account" doc:"description_method=GCSServiceAccountLongDescription"`
	HTTP           HTTPConfig     `yaml:"http"`
}

// RegisterFlags registers the flags for GCS storage
func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	cfg.RegisterFlagsWithPrefix("", f)
}

// RegisterFlagsWithPrefix registers the flags for GCS storage with the provided prefix
func (cfg *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.StringVar(&cfg.BucketName, prefix+"gcs.bucket-name", "", "GCS bucket name")
	f.Var(&cfg.ServiceAccount, prefix+"gcs.service-account", cfg.GCSServiceAccountShortDescription())
	cfg.HTTP.RegisterFlagsWithPrefix(prefix, f)
}

type HTTPConfig struct {
	IdleConnTimeout       time.Duration `yaml:"idle_conn_timeout" category:"advanced"`
	ResponseHeaderTimeout time.Duration `yaml:"response_header_timeout" category:"advanced"`
	InsecureSkipVerify    bool          `yaml:"insecure_skip_verify" category:"advanced"`
	TLSHandshakeTimeout   time.Duration `yaml:"tls_handshake_timeout" category:"advanced"`
	ExpectContinueTimeout time.Duration `yaml:"expect_continue_timeout" category:"advanced"`
	MaxIdleConns          int           `yaml:"max_idle_conns" category:"advanced"`
	MaxIdleConnsPerHost   int           `yaml:"max_idle_conns_per_host" category:"advanced"`
	MaxConnsPerHost       int           `yaml:"max_conns_per_host" category:"advanced"`
}

// RegisterFlagsWithPrefix registers the flags for s3 storage with the provided prefix
func (cfg *HTTPConfig) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.DurationVar(&cfg.IdleConnTimeout, prefix+"gcs.http.idle-conn-timeout", 90*time.Second, "The time an idle connection will remain idle before closing.")
	f.DurationVar(&cfg.ResponseHeaderTimeout, prefix+"gcs.http.response-header-timeout", 2*time.Minute, "The amount of time the client will wait for a servers response headers.")
	f.BoolVar(&cfg.InsecureSkipVerify, prefix+"gcs.http.insecure-skip-verify", false, "If the client connects to GCS via HTTPS and this option is enabled, the client will accept any certificate and hostname.")
	f.DurationVar(&cfg.TLSHandshakeTimeout, prefix+"gcs.tls-handshake-timeout", 10*time.Second, "Maximum time to wait for a TLS handshake. 0 means no limit.")
	f.DurationVar(&cfg.ExpectContinueTimeout, prefix+"gcs.expect-continue-timeout", 1*time.Second, "The time to wait for a server's first response headers after fully writing the request headers if the request has an Expect header. 0 to send the request body immediately.")
	f.IntVar(&cfg.MaxIdleConns, prefix+"gcs.max-idle-connections", 0, "Maximum number of idle (keep-alive) connections across all hosts. 0 means no limit.")
	f.IntVar(&cfg.MaxIdleConnsPerHost, prefix+"gcs.max-idle-connections-per-host", 100, "Maximum number of idle (keep-alive) connections to keep per-host. If 0, a built-in default value is used.")
	f.IntVar(&cfg.MaxConnsPerHost, prefix+"gcs.max-connections-per-host", 0, "Maximum number of connections per host. 0 means no limit.")
}

func (cfg *Config) GCSServiceAccountShortDescription() string {
	return "JSON either from a Google Developers Console client_credentials.json file, or a Google Developers service account key. Needs to be valid JSON, not a filesystem path."
}

func (cfg *Config) GCSServiceAccountLongDescription() string {
	return cfg.GCSServiceAccountShortDescription() +
		" If empty, fallback to Google default logic:" +
		"\n1. A JSON file whose path is specified by the GOOGLE_APPLICATION_CREDENTIALS environment variable. For workload identity federation, refer to https://cloud.google.com/iam/docs/how-to#using-workload-identity-federation on how to generate the JSON configuration file for on-prem/non-Google cloud platforms." +
		"\n2. A JSON file in a location known to the gcloud command-line tool: $HOME/.config/gcloud/application_default_credentials.json." +
		"\n3. On Google Compute Engine it fetches credentials from the metadata server."
}
