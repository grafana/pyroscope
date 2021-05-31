package cli

import (
	"fmt"

	"github.com/kardianos/service"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/agent/target"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func startAgent(c *config.Agent) error {
	if err := loadTargets(c); err != nil {
		return fmt.Errorf("configuration is invalid: %w", err)
	}
	svc, _ := service.New(newAgentService(c), &service.Config{Name: "pyroscope"})
	return svc.Run()
}

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
