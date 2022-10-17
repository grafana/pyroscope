// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/bucket/azure/config.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package azure

import (
	"flag"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/flagext"
)

// Config holds the config options for an Azure backend
type Config struct {
	StorageAccountName string         `yaml:"account_name"`
	StorageAccountKey  flagext.Secret `yaml:"account_key"`
	ContainerName      string         `yaml:"container_name"`
	Endpoint           string         `yaml:"endpoint_suffix"`
	MaxRetries         int            `yaml:"max_retries" category:"advanced"`
	MSIResource        string         `yaml:"msi_resource" category:"advanced" doc:"hidden"` // TODO Remove in Mimir 2.7.
	UserAssignedID     string         `yaml:"user_assigned_id" category:"advanced"`
}

// RegisterFlags registers the flags for Azure storage
func (cfg *Config) RegisterFlags(f *flag.FlagSet, logger log.Logger) {
	cfg.RegisterFlagsWithPrefix("", f, logger)
}

// RegisterFlagsWithPrefix registers the flags for Azure storage
func (cfg *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet, logger log.Logger) {
	f.StringVar(&cfg.StorageAccountName, prefix+"azure.account-name", "", "Azure storage account name")
	f.Var(&cfg.StorageAccountKey, prefix+"azure.account-key", "Azure storage account key")
	f.StringVar(&cfg.ContainerName, prefix+"azure.container-name", "", "Azure storage container name")
	f.StringVar(&cfg.Endpoint, prefix+"azure.endpoint-suffix", "", "Azure storage endpoint suffix without schema. The account name will be prefixed to this value to create the FQDN. If set to empty string, default endpoint suffix is used.")
	f.IntVar(&cfg.MaxRetries, prefix+"azure.max-retries", 20, "Number of retries for recoverable errors")
	flagext.DeprecatedFlag(f, prefix+"azure.msi-resource", "Deprecated: this setting was used for obtaining ServicePrincipalToken from MSI. The Azure SDK now chooses the address.", logger)
	f.StringVar(&cfg.UserAssignedID, prefix+"azure.user-assigned-id", "", "User assigned identity. If empty, then System assigned identity is used.")
}
