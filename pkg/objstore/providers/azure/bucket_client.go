// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/bucket/azure/bucket_client.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package azure

import (
	"github.com/go-kit/log"
	"github.com/thanos-io/objstore"
	"github.com/thanos-io/objstore/providers/azure"
	yaml "gopkg.in/yaml.v3"
)

func NewBucketClient(cfg Config, name string, logger log.Logger) (objstore.Bucket, error) {
	// Start with default config to make sure that all parameters are set to sensible values, especially
	// HTTP Config field.
	bucketConfig := azure.DefaultConfig
	bucketConfig.StorageAccountName = cfg.StorageAccountName
	bucketConfig.StorageAccountKey = cfg.StorageAccountKey.String()
	bucketConfig.ContainerName = cfg.ContainerName
	bucketConfig.Endpoint = cfg.Endpoint
	bucketConfig.MaxRetries = cfg.MaxRetries
	bucketConfig.UserAssignedID = cfg.UserAssignedID

	// Thanos currently doesn't support passing the config as is, but expects a YAML,
	// so we're going to serialize it.
	serialized, err := yaml.Marshal(bucketConfig)
	if err != nil {
		return nil, err
	}

	return azure.NewBucket(logger, serialized, name)
}
