package exec

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/pyspy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/rbspy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/types"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/util/process"
	"github.com/sirupsen/logrus"
)

type Connect struct {
	Logger             *logrus.Logger
	Upstream           upstream.Upstream
	SpyName            string
	ApplicationName    string
	SampleRate         uint32
	DetectSubprocesses bool
	Tags               map[string]string
	Pid                int
}

func NewConnect(cfg *config.Connect) (*Connect, error) {
	spyName := cfg.SpyName
	if cfg.Pid == -1 {
		if spyName != "" && spyName != "ebpfspy" {
			return nil, fmt.Errorf("pid -1 can only be used with ebpfspy")
		}
		spyName = "ebpfspy"
	}
	if err := PerformChecks(spyName); err != nil {
		return nil, err
	}

	logger := NewLogger(cfg.LogLevel, cfg.NoLogging)

	rc := remote.RemoteConfig{
		AuthToken:              cfg.AuthToken,
		UpstreamThreads:        cfg.UpstreamThreads,
		UpstreamAddress:        cfg.ServerAddress,
		UpstreamRequestTimeout: cfg.UpstreamRequestTimeout,
	}
	up, err := remote.New(rc, logger)
	if err != nil {
		return nil, fmt.Errorf("new remote upstream: %v", err)
	}

	// if the sample rate is zero, use the default value
	sampleRate := uint32(types.DefaultSampleRate)
	if cfg.SampleRate != 0 {
		sampleRate = uint32(cfg.SampleRate)
	}

	// TODO: this is somewhat hacky, we need to find a better way to configure agents
	pyspy.Blocking = cfg.PyspyBlocking
	rbspy.Blocking = cfg.RbspyBlocking

	return &Connect{
		Logger:             logger,
		Upstream:           up,
		SpyName:            spyName,
		ApplicationName:    CheckApplicationName(logger, cfg.ApplicationName, spyName, []string{}),
		SampleRate:         sampleRate,
		DetectSubprocesses: cfg.DetectSubprocesses,
		Tags:               cfg.Tags,
		Pid:                cfg.Pid,
	}, nil
}

func (c *Connect) Run() error {
	c.Logger.Debug("starting command")

	c.Upstream.Start()
	defer c.Upstream.Stop()

	// The channel buffer capacity should be sufficient to be keep up with
	// the expected signal rate (in case of Exec all the signals to be relayed
	// to the child process)
	ch := make(chan os.Signal, 10)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	defer func() {
		signal.Stop(ch)
		close(ch)
	}()

	sc := agent.SessionConfig{
		Upstream:         c.Upstream,
		AppName:          c.ApplicationName,
		Tags:             c.Tags,
		ProfilingTypes:   []spy.ProfileType{spy.ProfileCPU},
		SpyName:          c.SpyName,
		SampleRate:       c.SampleRate,
		UploadRate:       10 * time.Second,
		Pid:              c.Pid,
		WithSubprocesses: c.DetectSubprocesses,
		Logger:           c.Logger,
	}
	session, err := agent.NewSession(sc)
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"app-name":            c.ApplicationName,
		"spy-name":            c.SpyName,
		"pid":                 c.Pid,
		"detect-subprocesses": c.DetectSubprocesses,
	}).Debug("starting agent session")

	c.Upstream.Start()
	defer c.Upstream.Stop()

	if err = session.Start(); err != nil {
		return fmt.Errorf("start session: %w", err)
	}
	defer session.Stop()

	// wait for process to exit
	// pid == -1 means we're profiling whole system
	if c.Pid == -1 {
		<-ch
		return nil
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ch:
			return nil
		case <-ticker.C:
			if !process.Exists(c.Pid) {
				c.Logger.Debugf("child process exited")
				return nil
			}
		}
	}
}
