package exec

import (
	"fmt"
	"os"
	goexec "os/exec"
	"os/signal"
	"path"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/pyspy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/rbspy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/types"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/util/process"
)

type Exec struct {
	Args               []string
	Logger             *logrus.Logger
	Upstream           upstream.Upstream
	SpyName            string
	ApplicationName    string
	SampleRate         uint32
	DetectSubprocesses bool
	Tags               map[string]string
	NoRootDrop         bool
	UserName           string
	GroupName          string
}

func NewExec(cfg *config.Exec, args []string) (*Exec, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("no arguments passed")
	}

	spyName := cfg.SpyName
	if spyName == "auto" {
		baseName := path.Base(args[0])
		spyName = spy.ResolveAutoName(baseName)
		if spyName == "" {
			return nil, UnsupportedSpyError{Subcommand: "exec", Args: args}
		}
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

	return &Exec{
		Args:               args,
		Logger:             logger,
		Upstream:           up,
		SpyName:            spyName,
		ApplicationName:    CheckApplicationName(logger, cfg.ApplicationName, spyName, args),
		SampleRate:         sampleRate,
		DetectSubprocesses: cfg.DetectSubprocesses,
		Tags:               cfg.Tags,
		NoRootDrop:         cfg.NoRootDrop,
		UserName:           cfg.UserName,
		GroupName:          cfg.GroupName,
	}, nil
}

func (e *Exec) Run() error {
	e.Logger.WithFields(logrus.Fields{
		"args": fmt.Sprintf("%q", e.Args),
	}).Debug("starting command")

	// The channel buffer capacity should be sufficient to be keep up with
	// the expected signal rate (in case of Exec all the signals to be relayed
	// to the child process)
	c := make(chan os.Signal, 10)
	var cmd *goexec.Cmd
	// Note that we don't specify which signals to be sent: any signal to be
	// relayed to the child process (including SIGINT and SIGTERM).
	signal.Notify(c)
	cmd = goexec.Command(e.Args[0], e.Args[1:]...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	if err := adjustCmd(cmd, e.NoRootDrop, e.UserName, e.GroupName); err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() {
		signal.Stop(c)
		close(c)
	}()

	sc := agent.SessionConfig{
		Upstream:         e.Upstream,
		AppName:          e.ApplicationName,
		Tags:             e.Tags,
		ProfilingTypes:   []spy.ProfileType{spy.ProfileCPU},
		SpyName:          e.SpyName,
		SampleRate:       e.SampleRate,
		UploadRate:       10 * time.Second,
		Pid:              cmd.Process.Pid,
		WithSubprocesses: e.DetectSubprocesses,
		Logger:           e.Logger,
	}
	session, err := agent.NewSession(sc)
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"app-name":            e.ApplicationName,
		"spy-name":            e.SpyName,
		"pid":                 cmd.Process.Pid,
		"detect-subprocesses": e.DetectSubprocesses,
	}).Debug("starting agent session")

	e.Upstream.Start()
	defer e.Upstream.Stop()

	if err := session.Start(); err != nil {
		return fmt.Errorf("start session: %w", err)
	}
	defer session.Stop()

	// Wait for spawned process to exit
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case s := <-c:
			_ = process.SendSignal(cmd.Process, s)
		case <-ticker.C:
			if !process.Exists(cmd.Process.Pid) {
				logrus.Debug("child process exited")
				return cmd.Wait()
			}
		}
	}
}
