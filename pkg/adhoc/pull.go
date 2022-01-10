package adhoc

import (
	"os"
	goexec "os/exec"
	"os/signal"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/direct"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exec"
	"github.com/pyroscope-io/pyroscope/pkg/exporter"
	"github.com/pyroscope-io/pyroscope/pkg/scrape"
	scrapeconfig "github.com/pyroscope-io/pyroscope/pkg/scrape/config"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/discovery/targetgroup"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/model"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/util/process"
)

type pull struct {
	c        chan os.Signal
	cmd      *goexec.Cmd
	logger   *logrus.Logger
	manager  *scrape.Manager
	upstream upstream.Upstream
	targets  map[string][]*targetgroup.Group
}

func newPull(cfg *config.Adhoc, args []string, st *storage.Storage, logger *logrus.Logger) (runner, error) {
	var c chan os.Signal
	var cmd *goexec.Cmd
	if len(args) > 0 {
		// when arguments are specified, let's launch the target first.
		c = make(chan os.Signal, 10)
		signal.Notify(c)
		cmd = goexec.Command(args[0], args[1:]...)
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		cmd.Stdin = os.Stdin
	}

	// build the scrape manager to retrieve data
	defaultMetricsRegistry := prometheus.DefaultRegisterer
	e, err := exporter.NewExporter(config.MetricsExportRules{}, defaultMetricsRegistry)
	if err != nil {
		return nil, err
	}

	u := direct.New(st, e)
	m := scrape.NewManager(logger, u, defaultMetricsRegistry)
	scrapeCfg := &(*scrapeconfig.DefaultConfig())
	scrapeCfg.JobName = "adhoc"
	scrapeCfg.EnabledProfiles = []string{"cpu", "mem"}
	if err := m.ApplyConfig([]*scrapeconfig.Config{scrapeCfg}); err != nil {
		return nil, err
	}

	appName := exec.CheckApplicationName(logger, cfg.ApplicationName, "pull", args)
	targets := map[string][]*targetgroup.Group{
		"adhoc": {
			&targetgroup.Group{
				Source: "adhoc",
				Labels: model.LabelSet{},
				Targets: []model.LabelSet{
					{
						model.AddressLabel:    model.LabelValue(cfg.URL),
						model.MetricNameLabel: model.LabelValue(appName),
					},
				},
			},
		},
	}

	return &pull{
		c:        c,
		cmd:      cmd,
		logger:   logger,
		manager:  m,
		upstream: u,
		targets:  targets,
	}, nil
}

func (p *pull) Run() error {
	if p.cmd != nil {
		if err := p.cmd.Start(); err != nil {
			return err
		}
		defer func() {
			signal.Stop(p.c)
			close(p.c)
		}()
	}

	p.upstream.Start()
	defer p.upstream.Stop()

	done := make(chan error)
	c := make(chan map[string][]*targetgroup.Group)
	go func() {
		err := p.manager.Run(c)
		if err == nil {
			p.manager.Stop()
		}
		done <- err
	}()
	c <- p.targets

	// Wait till some exit condition happens
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	// Don't check the process if there's none to check
	if p.cmd == nil {
		ticker.Stop()
	}

	for {
		select {
		case s := <-p.c:
			if p.cmd != nil {
				_ = process.SendSignal(p.cmd.Process, s)
			} else {
				return nil
			}
		case err := <-done:
			return err
		case <-ticker.C:
			if !process.Exists(p.cmd.Process.Pid) {
				p.logger.Debug("child process exited")
				return p.cmd.Wait()
			}
		}
	}
}
