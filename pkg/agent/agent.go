package agent

import (
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/agent/csock"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

type Agent struct {
	cfg  *config.Config
	cs   *csock.CSock
	ctrl *Controller
}

func New(cfg *config.Config) *Agent {
	return &Agent{
		cfg:  cfg,
		ctrl: NewController(cfg, remote.New(cfg)),
	}
}

func (a *Agent) Start() {
	sockPath := a.cfg.Agent.UNIXSocketPath
	cs, err := csock.NewUnixCSock(sockPath, a.controlSocketHandler)
	if err != nil {
		log.Fatal(err)
	}
	a.cs = cs
	defer os.Remove(sockPath)

	go SelfProfile(a.cfg, a.ctrl.upstream, "pyroscope.agent.cpu{}")
	a.ctrl.Start()
	log.WithField("addr", cs.CanonicalAddr()).Info("Starting control socket")
	cs.Start()
}

func (a *Agent) Stop() {
	a.cs.Stop()
	a.ctrl.Stop()
}

func (a *Agent) controlSocketHandler(req *csock.Request) *csock.Response {
	switch req.Command {
	case "start":
		// TODO: pass withSubprocesses from somewhere
		profileID := a.ctrl.StartProfiling(req.SpyName, req.Pid, false)
		return &csock.Response{ProfileID: profileID}
	case "stop":
		// TODO: "testapp.cpu{}" should come from the client
		a.ctrl.StopProfiling(req.ProfileID, "testapp.cpu{}")
		return &csock.Response{}
	default:
		return &csock.Response{}
	}
}
