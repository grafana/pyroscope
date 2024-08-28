package read_path

import (
	"flag"
	"time"

	"github.com/grafana/dskit/flagext"
)

type ReadPath int

const (
	QuerierReadPath ReadPath = iota
	QueryBackendReadPath
	CombinedReadPath
)

type Config struct {
	EnableQueryBackend     bool      `yaml:"write_path" json:"write_path" doc:"hidden"`
	EnableQueryBackendFrom time.Time `yaml:"write_path_ingester_weight" json:"write_path_ingester_weight" doc:"hidden"`
}

func (o *Config) RegisterFlags(f *flag.FlagSet) {
	f.BoolVar(&o.EnableQueryBackend, "enable-query-backend", false,
		"This parameter specifies whether the query-backend should be used.")
	f.Var((*flagext.Time)(&o.EnableQueryBackendFrom), "enable-query-backend-from",
		"This parameter specifies the point in time from which data is queried from the new query backend.")
}
