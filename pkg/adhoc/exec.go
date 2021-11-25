package adhoc

import (
	"path"

	"github.com/pyroscope-io/pyroscope/pkg/agent/pyspy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/rbspy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/types"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/direct"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exec"
	"github.com/pyroscope-io/pyroscope/pkg/exporter"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/sirupsen/logrus"
)

func newExec(cfg *config.Adhoc, args []string, storage *storage.Storage, logger *logrus.Logger) (runner, error) {
	spyName := cfg.SpyName
	if spyName == "" {
		baseName := path.Base(args[0])
		spyName = spy.ResolveAutoName(baseName)
		if spyName == "" {
			return nil, exec.UnsupportedSpyError{Subcommand: "adhoc", Args: args}
		}
	}
	if err := exec.PerformChecks(spyName); err != nil {
		return nil, err
	}

	upstream := direct.New(storage, exporter.MetricsExporter{})

	// if the sample rate is zero, use the default value
	sampleRate := uint32(types.DefaultSampleRate)
	if cfg.SampleRate != 0 {
		sampleRate = uint32(cfg.SampleRate)
	}

	// TODO: this is somewhat hacky, we need to find a better way to configure agents
	pyspy.Blocking = cfg.PyspyBlocking
	rbspy.Blocking = cfg.RbspyBlocking

	return &exec.Exec{
		Args:               args,
		Logger:             logger,
		Upstream:           upstream,
		SpyName:            spyName,
		ApplicationName:    exec.CheckApplicationName(logger, cfg.ApplicationName, spyName, args),
		SampleRate:         sampleRate,
		DetectSubprocesses: cfg.DetectSubprocesses,
	}, nil
}
