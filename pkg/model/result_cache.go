package model

import prommodel "github.com/prometheus/common/model"

// ResultCacheFragment configures one aligned result-cache duration and the
// lifetime of Redis entries for that duration.
type ResultCacheFragment struct {
	Duration prommodel.Duration `yaml:"duration" json:"duration"`
	TTL      prommodel.Duration `yaml:"ttl" json:"ttl"`
}
