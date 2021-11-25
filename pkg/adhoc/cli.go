package adhoc

import (
	"fmt"
	"path"
	"time"

	"github.com/fatih/color"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exec"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

type runner interface {
	Run() error
}

type mode int

const (
	modeExec mode = iota + 1
	modeConnect
	modePush
	modePull
)

func Cli(cfg *config.Adhoc, args []string) error {
	// Determine the mode to use to gather profiling data
	var m mode
	if cfg.Push {
		if cfg.SpyName != "auto" {
			return fmt.Errorf("'--push' and '--spy-name' can not be set together")
		}
		if cfg.Pid != 0 {
			return fmt.Errorf("'--push' and '--pid' can not be set together")
		}
		if cfg.URL != "" {
			return fmt.Errorf("'--push' and '--url' can not be set together")
		}
		m = modePush
	} else if cfg.URL != "" {
		if cfg.SpyName != "auto" {
			return fmt.Errorf("'--url' and '--spy-name' can not be set together")
		}
		if cfg.Pid != 0 {
			return fmt.Errorf("'--url' and '--pid' can not be set together")
		}
		m = modePull
	} else if cfg.Pid != 0 {
		m = modeConnect
	} else if cfg.SpyName != "auto" {
		m = modeExec
	} else {
		if len(args) == 0 {
			return fmt.Errorf("could not detect the preferred profiling mode. Either pass the proper flag or some argument")
		}
		baseName := path.Base(args[0])
		if spy.ResolveAutoName(baseName) == "" {
			m = modePush
		} else {
			m = modeExec
		}
	}

	// TODO(abeaumont): Move logger configuration to a generic place
	logger := exec.NewLogger(cfg.LogLevel, cfg.NoLogging)

	switch cfg.OutputFormat {
	case "json", "pprof", "collapsed":
	default:
		return fmt.Errorf("invalid output format '%s', the only supported output formats are 'json', 'pprof' and 'collapsed'", cfg.OutputFormat)
	}

	st, err := storage.New(newStorageConfig(cfg), logger, prometheus.DefaultRegisterer)
	if err != nil {
		return fmt.Errorf("could not initialize storage: %w", err)
	}

	var r runner
	switch m {
	case modeExec:
		r, err = newExec(cfg, args, st, logger)
	case modeConnect:
		r, err = newConnect(cfg, args, st, logger)
	case modePush:
		r, err = newPush(cfg, args, st, logger)
	case modePull:
		r, err = newPull(cfg, args, st, logger)
	default:
		err = fmt.Errorf("could not determine the profiling mode. This shouldn't happen, please reporte the bug at %s",
			color.GreenString("https://github.com/pyroscope-io/pyroscope/issues"),
		)
	}
	if err != nil {
		return err
	}

	t0 := time.Now()
	r.Run()
	newWriter(cfg, st, logger).write(t0, time.Now())

	logger.Debug("stopping storage")
	if err := st.Close(); err != nil {
		logger.WithError(err).Error("storage close")
	}
	return err
}

func newStorageConfig(cfg *config.Adhoc) *storage.Config {
	return storage.NewConfig(&config.Server{MaxNodesSerialization: cfg.MaxNodesSerialization}).WithInMemory()
}
