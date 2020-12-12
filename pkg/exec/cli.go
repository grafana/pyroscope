package exec

import (
	"errors"
	"os"
	"os/exec"

	"github.com/petethepig/pyroscope/pkg/agent"
	"github.com/petethepig/pyroscope/pkg/agent/upstream/remote"
	"github.com/petethepig/pyroscope/pkg/config"
	"github.com/sirupsen/logrus"
)

func Cli(cfg *config.Config, args []string) error {
	if len(args) == 0 {
		return errors.New("no arguments passed")
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	err := cmd.Start()
	if err != nil {
		return err
	}
	u := remote.New(cfg)
	ctrl := agent.NewController(cfg, u)
	ctrl.Start()
	defer ctrl.Stop()

	logrus.Debug("cmd.Process", cmd.Process)

	go ctrl.StartContinuousProfiling(cfg.Exec.SpyName, cfg.Exec.UploadName, cmd.Process.Pid)

	cmd.Wait()
	return nil
}
