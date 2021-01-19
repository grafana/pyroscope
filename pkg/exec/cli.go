package exec

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/mitchellh/go-ps"
	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/sirupsen/logrus"
)

func Cli(cfg *config.Config, args []string) error {
	if len(args) == 0 {
		return errors.New("no arguments passed")
	}

	spyName := cfg.Exec.SpyName
	if spyName == "auto" {
		baseName := path.Base(args[0])
		spyName = spy.ResolveAutoName(baseName)
		if spyName == "" {
			supportedSpies := supportedSpiesWithoutGospy()
			suggestedCommand := fmt.Sprintf("pyroscope exec -spy-name %s %s", supportedSpies[0], strings.Join(args, " "))
			return fmt.Errorf(
				"could not automatically find a spy for program \"%s\". Pass spy name via %s argument, for example: \n  %s\n\nAvailable spies are: %s\n%s\nIf you believe this is a mistake, please submit an issue at %s",
				baseName,
				color.YellowString("-spy-name"),
				color.YellowString(suggestedCommand),
				strings.Join(supportedSpies, ","),
				armMessage(),
				color.BlueString("https://github.com/pyroscope-io/pyroscope/issues"),
			)
		}
	}

	logrus.Info("to disable logging from pyroscope, pass " + color.YellowString("-no-logging") + " argument to pyroscope exec")

	if err := performChecks(spyName); err != nil {
		return err
	}

	signal.Ignore(syscall.SIGCHLD)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.Setpgid = true
	err := cmd.Start()
	if err != nil {
		return err
	}
	u := remote.New(remote.RemoteConfig{
		UpstreamAddress:        cfg.Exec.ServerAddress,
		UpstreamThreads:        cfg.Exec.UpstreamThreads,
		UpstreamRequestTimeout: cfg.Exec.UpstreamRequestTimeout,
	})

	// TODO: make configurable?
	time.Sleep(5 * time.Second)
	// TODO: add sample rate
	sess := agent.NewSession(u, cfg.Exec.ApplicationName, spyName, 100, cmd.Process.Pid, cfg.Exec.DetectSubprocesses)
	sess.Start()
	defer sess.Stop()

	// TODO: very hacky, at some point we'll need to make wait work
	// cmd.Wait()

	for {
		time.Sleep(time.Second)
		p, err := ps.FindProcess(cmd.Process.Pid)
		if p == nil || err != nil {
			break
		}
	}
	return nil
}

func supportedSpiesWithoutGospy() []string {
	supportedSpies := []string{}
	for _, s := range spy.SupportedSpies {
		if s != "gospy" {
			supportedSpies = append(supportedSpies, s)
		}
	}

	return supportedSpies
}

func performChecks(spyName string) error {
	if spyName == "gospy" {
		return fmt.Errorf("gospy can not profile other processes. See our documentation on using gospy: %s", color.BlueString("https://pyroscope.io/docs/"))
	}

	if runtime.GOOS == "darwin" {
		if !isRoot() {
			logrus.Error("on macOS you're required to run the agent with sudo")
		}
	}

	if stringsContains(spy.SupportedSpies, spyName) {
		supportedSpies := supportedSpiesWithoutGospy()
		return fmt.Errorf(
			"Spy \"%s\" is not supported. Available spies are: %s\n%s",
			color.BlueString("spyName"),
			strings.Join(supportedSpies, ","),
			armMessage(),
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

func isRoot() bool {
	u, err := user.Current()
	return err == nil && u.Username == "root"
}

func armMessage() string {
	if runtime.GOARCH == "arm64" {
		return "Note that rbspy is not available on arm64 platform"
	}
	return ""
}
