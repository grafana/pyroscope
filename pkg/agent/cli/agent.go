package cli

import (
	"os"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/csock"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/util/id"
	"github.com/sirupsen/logrus"
)

type Agent struct {
	cfg            *config.Config
	cs             *csock.CSock
	activeProfiles map[int]*agent.ProfileSession
	id             id.ID
	u              upstream.Upstream
}

func New(cfg *config.Config) *Agent {
	// TODO: handle this error properly
	r, _ := remote.New(remote.RemoteConfig{
		UpstreamThreads:        cfg.Agent.UpstreamThreads,
		UpstreamAddress:        cfg.Agent.ServerAddress,
		UpstreamRequestTimeout: cfg.Agent.UpstreamRequestTimeout,
	})
	r.Logger = logrus.StandardLogger()
	return &Agent{
		cfg:            cfg,
		activeProfiles: make(map[int]*agent.ProfileSession),
		u:              r,
	}
}

func (a *Agent) Start() error {
	sockPath := a.cfg.Agent.UNIXSocketPath
	cs, err := csock.NewUnixCSock(sockPath, a.controlSocketHandler)
	if err != nil {
		return err
	}
	a.cs = cs
	defer os.Remove(sockPath)

	go agent.SelfProfile(a.cfg, a.u, "pyroscope.agent.cpu{}", logrus.StandardLogger())
	cs.Start()
	return nil
}

func (a *Agent) Stop() {
	a.cs.Stop()
}

func (a *Agent) controlSocketHandler(req *csock.Request) *csock.Response {
	switch req.Command {
	case "start":
		profileID := int(a.id.Next())
		// TODO: pass withSubprocesses from somewhere
		// TODO: pass appName from somewhere
		// TODO: add sample rate
		s := agent.NewSession(a.u, "testapp.cpu", "gospy", 100, 10*time.Second, 0, false)
		s.Logger = logrus.StandardLogger()
		a.activeProfiles[profileID] = s
		s.Start()
		return &csock.Response{ProfileID: profileID}
	case "stop":
		// TODO: "testapp.cpu{}" should come from the client
		profileID := req.ProfileID
		if s, ok := a.activeProfiles[profileID]; ok {
			s.Stop()
			delete(a.activeProfiles, profileID)
		}
		return &csock.Response{}
	default:
		return &csock.Response{}
	}
}
