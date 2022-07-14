package firedb

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
)

type Config struct {
	DataPath string
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	f.StringVar(&cfg.DataPath, "firedb.data-path", "./data", "Directory used for local storage.")
}

type FireDB struct {
	cfg    *Config
	reg    prometheus.Registerer
	logger log.Logger

	head *Head
}

func New(cfg *Config, logger log.Logger, reg prometheus.Registerer) (*FireDB, error) {
	head, err := NewHead(logger, reg)
	if err != nil {
		return nil, fmt.Errorf("error initializing head: %w", err)
	}
	return &FireDB{
		cfg:    cfg,
		head:   head,
		reg:    reg,
		logger: logger,
	}, nil
}

func (f *FireDB) Head() *Head {
	return f.head
}

func (f *FireDB) Flush(ctx context.Context) error {
	return f.head.Flush(ctx, filepath.Join(f.cfg.DataPath, "head"))
}
