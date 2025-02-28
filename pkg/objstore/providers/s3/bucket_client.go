// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/bucket/s3/bucket_client.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package s3

import (
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/common/model"
	"github.com/thanos-io/objstore"
	"github.com/thanos-io/objstore/providers/s3"
)

// NewBucketClient creates a new S3 bucket client
func NewBucketClient(cfg Config, name string, logger log.Logger) (objstore.Bucket, error) {
	s3Cfg, err := newS3Config(cfg)
	if err != nil {
		return nil, err
	}

	warnForDeprecatedConfigFields(cfg, logger)

	return s3.NewBucketWithConfig(logger, s3Cfg, name, nil)
}

// NewBucketReaderClient creates a new S3 bucket client
func NewBucketReaderClient(cfg Config, name string, logger log.Logger) (objstore.BucketReader, error) {
	s3Cfg, err := newS3Config(cfg)
	if err != nil {
		return nil, err
	}

	warnForDeprecatedConfigFields(cfg, logger)

	return s3.NewBucketWithConfig(logger, s3Cfg, name, nil)
}

func newS3Config(cfg Config) (s3.Config, error) {
	sseCfg, err := cfg.SSE.BuildThanosConfig()
	if err != nil {
		return s3.Config{}, err
	}

	bucketLookupType := s3.AutoLookup
	if cfg.ForcePathStyle || cfg.BucketLookupType == PathStyleLookup {
		bucketLookupType = s3.PathLookup
	} else if cfg.BucketLookupType == VirtualHostedStyleLookup {
		bucketLookupType = s3.VirtualHostLookup
	}

	return s3.Config{
		Bucket:           cfg.BucketName,
		Endpoint:         cfg.Endpoint,
		Region:           cfg.Region,
		AccessKey:        cfg.AccessKeyID,
		SecretKey:        cfg.SecretAccessKey.String(),
		Insecure:         cfg.Insecure,
		SSEConfig:        sseCfg,
		BucketLookupType: bucketLookupType,
		HTTPConfig: s3.HTTPConfig{
			IdleConnTimeout:       model.Duration(cfg.HTTP.IdleConnTimeout),
			ResponseHeaderTimeout: model.Duration(cfg.HTTP.ResponseHeaderTimeout),
			InsecureSkipVerify:    cfg.HTTP.InsecureSkipVerify,
			TLSHandshakeTimeout:   model.Duration(cfg.HTTP.TLSHandshakeTimeout),
			ExpectContinueTimeout: model.Duration(cfg.HTTP.ExpectContinueTimeout),
			MaxIdleConns:          cfg.HTTP.MaxIdleConns,
			MaxIdleConnsPerHost:   cfg.HTTP.MaxIdleConnsPerHost,
			MaxConnsPerHost:       cfg.HTTP.MaxConnsPerHost,
			Transport:             cfg.HTTP.Transport,
		},
		// Enforce signature version 2 if CLI flag is set
		SignatureV2: cfg.SignatureVersion == SignatureVersionV2,
	}, nil
}

func warnForDeprecatedConfigFields(cfg Config, logger log.Logger) {
	if cfg.ForcePathStyle {
		level.Warn(logger).Log("msg", "S3 bucket client config has a deprecated s3.force-path-style flag set. Please, use s3.bucket-lookup-type instead.")
	}
}
