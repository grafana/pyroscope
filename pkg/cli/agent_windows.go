package cli

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/kardianos/service"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"

	"github.com/pyroscope-io/pyroscope/pkg/agent/target"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

type agentService struct{ tgtMgr *target.Manager }

func newAgentService(c *config.Agent) agentService {
	return agentService{target.NewManager(logrus.StandardLogger(), c)}
}

func (svc agentService) Start(_ service.Service) error {
	return svc.start()
}

func (svc agentService) Stop(_ service.Service) error {
	svc.tgtMgr.Stop()
	return nil
}

func (svc agentService) start() error { return svc.tgtMgr.Start() }

func startAgent(c *config.Agent) error {
	if err := loadTargets(c); err != nil {
		return fmt.Errorf("configuration is invalid: %w", err)
	}
	svc, _ := service.New(newAgentService(c), &service.Config{Name: "pyroscope"})
	return svc.Run()
}

func loadTargets(c *config.Agent) error {
	b, err := ioutil.ReadFile(c.Config)
	switch {
	case err == nil:
	case os.IsNotExist(err):
		return nil
	default:
		return err
	}
	var a config.Agent
	if err = yaml.Unmarshal(b, &a); err != nil {
		return err
	}
	c.Targets = a.Targets
	return nil
}
