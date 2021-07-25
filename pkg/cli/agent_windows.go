package cli

import (
	"fmt"

	"github.com/kardianos/service"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func StartAgent(config *config.Agent) error {
	logger, err := createLogger(config)
	if err != nil {
		return fmt.Errorf("could not create logger: %w", err)
	}
	logger.Info("starting pyroscope agent")
	if err = loadAgentConfig(config); err != nil {
		return fmt.Errorf("could not load targets: %w", err)
	}
	agent, err := newAgentService(logger, config)
	if err != nil {
		return fmt.Errorf("could not initialize agent: %w", err)
	}
	svc, err := service.New(agent, &service.Config{Name: "pyroscope"})
	if err != nil {
		return fmt.Errorf("could not initialize system service: %w", err)
	}
	return svc.Run()
}
