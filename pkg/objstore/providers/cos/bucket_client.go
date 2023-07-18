package cos

import (
	"github.com/go-kit/log"
	"github.com/prometheus/common/model"
	"github.com/thanos-io/objstore"
	"github.com/thanos-io/objstore/exthttp"
	"github.com/thanos-io/objstore/providers/cos"
	"gopkg.in/yaml.v3"
)

// NewBucketClient creates a bucket client for COS
func NewBucketClient(cfg Config, name string, logger log.Logger) (objstore.Bucket, error) {
	bucketConfig := &cos.Config{
		Bucket:    cfg.Bucket,
		Region:    cfg.Region,
		AppId:     cfg.AppID,
		Endpoint:  cfg.Endpoint,
		SecretKey: cfg.SecretKey,
		SecretId:  cfg.SecretID,
		HTTPConfig: exthttp.HTTPConfig{
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
	}

	serializedConfig, err := yaml.Marshal(bucketConfig)
	if err != nil {
		return nil, err
	}

	return cos.NewBucket(logger, serializedConfig, name)
}
