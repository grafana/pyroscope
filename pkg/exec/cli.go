package exec

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/fatih/color"
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
				"could not automatically find a spy for program \"%s\". Pass spy name via %s argument, for example: \n  %s\n\nAvailable spies are: %s\n\nIf you believe this is a mistake, please submit an issue at %s",
				baseName,
				color.YellowString("-spy-name"),
				color.YellowString(suggestedCommand),
				strings.Join(supportedSpies, ","),
				color.BlueString("https://github.com/pyroscope-io/pyroscope/issues"),
			)
		}
	}

	logrus.Info("to disable logging from pyroscope, pass " + color.YellowString("-no-logging") + " argument to pyroscope exec")

	if spyName == "gospy" {
		return fmt.Errorf("gospy can not profile other processes. See our documentation on using gospy: %s", color.BlueString("https://pyroscope.io/docs/"))
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	err := cmd.Start()
	if err != nil {
		return err
	}
	u := remote.New(remote.RemoteConfig{
		UpstreamAddress:        cfg.Exec.UpstreamAddress,
		UpstreamThreads:        cfg.Exec.UpstreamThreads,
		UpstreamRequestTimeout: cfg.Exec.UpstreamRequestTimeout,
	})

	// TODO: make configurable?
	time.Sleep(5 * time.Second)
	// TODO: add sample rate
	sess := agent.NewSession(u, cfg.Exec.ApplicationName, spyName, 100, cmd.Process.Pid, cfg.Exec.DetectSubprocesses)
	sess.Start()
	defer sess.Stop()

	cmd.Wait()
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
