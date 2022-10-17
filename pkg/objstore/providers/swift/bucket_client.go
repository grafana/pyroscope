// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/bucket/swift/bucket_client.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package swift

import (
	"github.com/go-kit/log"
	"github.com/prometheus/common/model"
	"github.com/thanos-io/objstore"
	"github.com/thanos-io/objstore/providers/swift"
	yaml "gopkg.in/yaml.v3"
)

// NewBucketClient creates a new Swift bucket client
func NewBucketClient(cfg Config, name string, logger log.Logger) (objstore.Bucket, error) {
	bucketConfig := swift.Config{
		AuthVersion:       cfg.AuthVersion,
		AuthUrl:           cfg.AuthURL,
		Username:          cfg.Username,
		UserDomainName:    cfg.UserDomainName,
		UserDomainID:      cfg.UserDomainID,
		UserId:            cfg.UserID,
		Password:          cfg.Password.String(),
		DomainId:          cfg.DomainID,
		DomainName:        cfg.DomainName,
		ProjectID:         cfg.ProjectID,
		ProjectName:       cfg.ProjectName,
		ProjectDomainID:   cfg.ProjectDomainID,
		ProjectDomainName: cfg.ProjectDomainName,
		RegionName:        cfg.RegionName,
		ContainerName:     cfg.ContainerName,
		Retries:           cfg.MaxRetries,
		ConnectTimeout:    model.Duration(cfg.ConnectTimeout),
		Timeout:           model.Duration(cfg.RequestTimeout),

		// Hard-coded defaults.
		ChunkSize:              swift.DefaultConfig.ChunkSize,
		UseDynamicLargeObjects: false,
	}

	// Thanos currently doesn't support passing the config as is, but expects a YAML,
	// so we're going to serialize it.
	serialized, err := yaml.Marshal(bucketConfig)
	if err != nil {
		return nil, err
	}

	return swift.NewContainer(logger, serialized)
}
