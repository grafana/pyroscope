package exec

import (
	"errors"
	"fmt"
	"os"
	goexec "os/exec"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/pyspy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/rbspy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/types"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote"
	"github.com/pyroscope-io/pyroscope/pkg/util/names"
	"github.com/pyroscope-io/pyroscope/pkg/util/process"
)

// used in tests
var disableMacOSChecks bool
var disableLinuxChecks bool

// Cli is command line interface for both exec and connect commands
func Cli(cfg *Config, args []string) error {
	if cfg.kind == exec {
		if len(args) == 0 {
			return errors.New("no arguments passed")
		}
	} else if (cfg.pid != -1 && cfg.spyName == "ebpfspy") && !process.Exists(cfg.Pid) {
		return errors.New("process not found")
	}

	// TODO: this is somewhat hacky, we need to find a better way to configure agents
	pyspy.Blocking = cfg.pyspyBlocking
	rbspy.Blocking = cfg.rbspyBlocking

	spyName := cfg.spyName
	if spyName == "auto" {
		if cfg.kind == exec {
			baseName := path.Base(args[0])
			spyName = spy.ResolveAutoName(baseName)
			if spyName == "" {
				supportedSpies := spy.SupportedExecSpies()
				suggestedCommand := fmt.Sprintf("pyroscope exec -spy-name %s %s", supportedSpies[0], strings.Join(args, " "))
				return fmt.Errorf(
					"could not automatically find a spy for program \"%s\". Pass spy name via %s argument, for example: \n"+
						"  %s\n\nAvailable spies are: %s\nIf you believe this is a mistake, please submit an issue at %s",
					baseName,
					color.YellowString("-spy-name"),
					color.YellowString(suggestedCommand),
					strings.Join(supportedSpies, ","),
					color.GreenString("https://github.com/pyroscope-io/pyroscope/issues"),
				)
			}
		} else {
			supportedSpies := spy.SupportedExecSpies()
			suggestedCommand := fmt.Sprintf("pyroscope connect -spy-name %s %s", supportedSpies[0], strings.Join(args, " "))
			return fmt.Errorf(
				"pass spy name via %s argument, for example: \n  %s\n\nAvailable spies are: %s\nIf you believe this is a mistake, please submit an issue at %s",
				color.YellowString("-spy-name"),
				color.YellowString(suggestedCommand),
				strings.Join(supportedSpies, ","),
				color.GreenString("https://github.com/pyroscope-io/pyroscope/issues"),
			)
		}
	}

	if err := performChecks(spyName); err != nil {
		return err
	}
	logrus.SetLevel(cfg.logLevel)
	if cfg.logLevel != logrus.PanicLevel {
		logrus.Info("to disable logging from pyroscope, specify " + color.YellowString("-no-logging") + " flag")
	}

	if cfg.applicationName == "" {
		logrus.Infof("we recommend specifying application name via %s flag or env variable %s",
			color.YellowString("-application-name"), color.YellowString("PYROSCOPE_APPLICATION_NAME"))
		cfg.applicationName = spyName + "." + names.GetRandomName(generateSeed(args))
		logrus.Infof("for now we chose the name for you and it's \"%s\"", color.GreenString(cfg.applicationName))
	}

	logrus.WithFields(logrus.Fields{
		"args": fmt.Sprintf("%q", args),
	}).Debug("starting command")

	u, err := remote.New(cfg.RemoteConfig, logrus.StandardLogger())
	if err != nil {
		return fmt.Errorf("new remote upstream: %v", err)
	}
	defer u.Stop()

	// The channel buffer capacity should be sufficient to be keep up with
	// the expected signal rate (in case of Exec all the signals to be relayed
	// to the child process)
	c := make(chan os.Signal, 10)
	pid := cfg.pid
	var cmd *goexec.Cmd
	if cfg.kind == exec {
		// Note that we don't specify which signals to be sent: any signal to be
		// relayed to the child process (including SIGINT and SIGTERM).
		signal.Notify(c)
		cmd = goexec.Command(args[0], args[1:]...)
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		cmd.Stdin = os.Stdin
		if err = adjustCmd(cmd, *cfg); err != nil {
			logrus.Error(err)
		}
		if err = cmd.Start(); err != nil {
			return err
		}
		pid = cmd.Process.Pid
	} else {
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	}
	defer func() {
		signal.Stop(c)
		close(c)
	}()

	logrus.WithFields(logrus.Fields{
		"app-name":            cfg.applicationName,
		"spy-name":            spyName,
		"pid":                 pid,
		"detect-subprocesses": cfg.detectSubprocesses,
	}).Debug("starting agent session")

	sc := agent.SessionConfig{
		Upstream:         u,
		AppName:          cfg.applicationName,
		Tags:             cfg.tags,
		ProfilingTypes:   []spy.ProfileType{spy.ProfileCPU},
		SpyName:          spyName,
		SampleRate:       cfg.sampleRate,
		UploadRate:       10 * time.Second,
		Pid:              pid,
		WithSubprocesses: cfg.detectSubprocesses,
		Logger:           logrus.StandardLogger(),
	}
	session, err := agent.NewSession(sc)
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}
	if err = session.Start(); err != nil {
		return fmt.Errorf("start session: %w", err)
	}
	defer session.Stop()

	if cfg.kind == exec {
		return waitForSpawnedProcessToExit(c, cmd)
	}

	waitForProcessToExit(c, pid)
	return nil
}

func waitForSpawnedProcessToExit(c chan os.Signal, cmd *goexec.Cmd) error {
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

func waitForProcessToExit(c chan os.Signal, pid int) {
	// pid == -1 means we're profiling whole system
	if pid == -1 {
		<-c
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c:
			return
		case <-ticker.C:
			if !process.Exists(pid) {
				logrus.Debug("child process exited")
				return
			}
		}
	}
}

func performChecks(spyName string) error {
	if spyName == types.GoSpy {
		return fmt.Errorf("gospy can not profile other processes. See our documentation on using gospy: %s", color.GreenString("https://pyroscope.io/docs/"))
	}

	err := performOSChecks(spyName)
	if err != nil {
		return err
	}

	if !stringsContains(spy.SupportedSpies, spyName) {
		supportedSpies := spy.SupportedExecSpies()
		return fmt.Errorf(
			"spy \"%s\" is not supported. Available spies are: %s",
			color.GreenString(spyName),
			strings.Join(supportedSpies, ","),
		)
	}

	return nil
}

func stringsContains(arr []string, element string) bool {
	for _, v := range arr {
		if v == element {
			return true
		}
	}
	return false
}

func generateSeed(args []string) string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "<unknown>"
	}
	return cwd + "|" + strings.Join(args, "&")
}
