package read_path

import (
	"flag"
	"time"

	"github.com/grafana/dskit/flagext"
)

type Config struct {
	EnableQueryBackend     bool      `yaml:"enable_query_backend" json:"enable_query_backend" doc:"hidden"`
	EnableQueryBackendFrom time.Time `yaml:"enable_query_backend_from" json:"enable_query_backend_from" doc:"hidden"`
}

func (o *Config) RegisterFlags(f *flag.FlagSet) {
	f.BoolVar(&o.EnableQueryBackend, "enable-query-backend", false,
		"This parameter specifies whether the new query backend is enabled.")
	f.Var((*flagext.Time)(&o.EnableQueryBackendFrom), "enable-query-backend-from",
		"This parameter specifies the point in time from which data is queried from the new query backend. The format if RFC3339 (2020-10-20T00:00:00Z)")
}
