package adhoc

import (
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/pyroscope-io/pyroscope/pkg/analytics"
	"github.com/pyroscope-io/pyroscope/pkg/health"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/config"
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

func (m mode) String() string {
	switch m {
	case modeExec:
		return "exec"
	case modeConnect:
		return "connect"
	case modePush:
		return "push"
	case modePull:
		return "pull"
	default:
		return "unknown"
	}
}

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

	level := logrus.PanicLevel
	if l, err := logrus.ParseLevel(cfg.LogLevel); err == nil && !cfg.NoLogging {
		level = l
	}
	logger := logrus.StandardLogger()
	logger.SetLevel(level)
	// an adhoc run shouldn't be a long-running process, make the output less verbose and more human-friendly.
	logger.Formatter = &logrus.TextFormatter{DisableTimestamp: true}
	logger.SetReportCaller(false)

	switch cfg.OutputFormat {
	case "html", "pprof", "collapsed", "none":
	default:
		return fmt.Errorf("invalid output format '%s', the only supported output formats are 'html', 'pprof' and 'collapsed'", cfg.OutputFormat)
	}

	st, err := storage.New(newStorageConfig(cfg), logger, prometheus.DefaultRegisterer, new(health.Controller))
	if err != nil {
		return fmt.Errorf("could not initialize storage: %w", err)
	}

	var r runner
	switch m {
	case modeExec:
		r, err = newExec(cfg, args, st, logger)
	case modeConnect:
		r, err = newConnect(cfg, st, logger)
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
	status := "success"
	if err := r.Run(); err != nil {
		status = "failure"
		logger.WithError(err).Error("running profiler")
	}

	wg := sync.WaitGroup{}
	if !cfg.AnalyticsOptOut {
		wg.Add(1)
		go analytics.AdhocReport(m.String()+"-"+status, &wg)
	}

	if err := newWriter(cfg, st, logger).write(t0, time.Now()); err != nil {
		logger.WithError(err).Error("writing profiling data")
	}

	logger.Debug("stopping storage")
	if err := st.Close(); err != nil {
		logger.WithError(err).Error("storage close")
	}
	wg.Wait()
	return err
}

func newStorageConfig(cfg *config.Adhoc) *storage.Config {
	return storage.NewConfig(&config.Server{MaxNodesSerialization: cfg.MaxNodesSerialization}).WithInMemory()
}
