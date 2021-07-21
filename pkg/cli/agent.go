package cli

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/kardianos/service"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"

	"github.com/pyroscope-io/pyroscope/pkg/agent/target"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

type agentService struct {
	remote *remote.Remote
	tgtMgr *target.Manager
}

func newAgentService(logger *logrus.Logger, config *config.Agent) (*agentService, error) {
	rc := remote.RemoteConfig{
		AuthToken:              config.AuthToken,
		UpstreamThreads:        config.UpstreamThreads,
		UpstreamAddress:        config.ServerAddress,
		UpstreamRequestTimeout: config.UpstreamRequestTimeout,
		ManualStart:            true,
	}
	upstream, err := remote.New(rc, logger)
	if err != nil {
		return nil, fmt.Errorf("upstream configuration: %w", err)
	}
	s := agentService{
		tgtMgr: target.NewManager(logger, upstream, config),
		remote: upstream,
	}
	return &s, nil
}

func (svc *agentService) Start(_ service.Service) error {
	svc.remote.Start()
	svc.tgtMgr.Start()
	return nil
}

func (svc *agentService) Stop(_ service.Service) error {
	svc.tgtMgr.Stop()
	svc.remote.Stop()
	return nil
}

// loadAgentConfig is a hack for ff parser, which can't parse maps, structs,
// and slices. The function to be called after the parser finishes just to
// fill missing configuration elements.
func loadAgentConfig(c *config.Agent) error {
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
	// Override tags from config file with flags.
	c.Tags = mergeTags(a.Tags, c.Tags)
	for _, t := range a.Targets {
		t.Tags = mergeTags(t.Tags, c.Tags)
		c.Targets = append(c.Targets, t)
	}
	return nil
}

// mergeTags creates a new map with tags from a and b.
// Values from b take precedence. Returned map is never nil.
func mergeTags(a, b map[string]string) map[string]string {
	t := make(map[string]string, len(a))
	for k, v := range a {
		t[k] = v
	}
	for k, v := range b {
		t[k] = v
	}
	return t
}

func createLogger(config *config.Agent) (*logrus.Logger, error) {
	if config.NoLogging {
		logrus.SetOutput(ioutil.Discard)
		return logrus.StandardLogger(), nil
	}
	l, err := logrus.ParseLevel(config.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("parsing log level: %w", err)
	}
	logrus.SetLevel(l)
	if service.Interactive() || config.LogFilePath == "" {
		return logrus.StandardLogger(), nil
	}
	f, err := ensureLogFile(config.LogFilePath)
	if err != nil {
		return nil, fmt.Errorf("log file: %w", err)
	}
	logrus.SetOutput(f)
	return logrus.StandardLogger(), nil
}

func ensureLogFile(p string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(p), 0770); err != nil {
		return nil, err
	}
	return os.OpenFile(p, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
}
