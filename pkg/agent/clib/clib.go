// +build clib
// Package main deals with ruby / python / php integrations
package main

import (
	"C"
	"fmt"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)
import (
	"errors"

	"github.com/pyroscope-io/pyroscope/pkg/agent/pyspy"
	"github.com/pyroscope-io/pyroscope/pkg/util/caps"
)

type PyspySession struct {
	session *agent.ProfileSession
}

func performOSChecks() error {
	if !caps.HasSysPtraceCap() {
		return errors.New("if you're running pyroscope in a Docker container, add --cap-add=sys_ptrace. See our Docker Guide for more information: https://pyroscope.io/docs/docker-guide")
	}
	return nil
}

var pyspy_session = PyspySession{}

func (pys PyspySession) startNewSession(cfg *config.Exec) (*agent.ProfileSession, error) {
	logger := &agent.NoopLogger{}

	spyName := cfg.SpyName
	pid := cfg.Pid
	pyspy.Blocking = cfg.PyspyBlocking

	if err := performOSChecks(); pyspy.Blocking && err != nil {
		return nil, err
	}

	rc := remote.RemoteConfig{
		AuthToken:              cfg.AuthToken,
		UpstreamAddress:        cfg.ServerAddress,
		UpstreamThreads:        cfg.UpstreamThreads,
		UpstreamRequestTimeout: cfg.UpstreamRequestTimeout,
	}
	u, err := remote.New(rc, logger)
	if err != nil {
		return nil, fmt.Errorf("new remote upstream: %v", err)
	}

	sc := agent.SessionConfig{
		Upstream:         u,
		AppName:          cfg.ApplicationName,
		ProfilingTypes:   []spy.ProfileType{spy.ProfileCPU},
		SpyName:          spyName,
		SampleRate:       uint32(cfg.SampleRate),
		UploadRate:       10 * time.Second,
		Pid:              pid,
		WithSubprocesses: cfg.DetectSubprocesses,
	}
	session, err := agent.NewSession(&sc, logger)
	if err != nil {
		return nil, err
	}
	if err = session.Start(); err != nil {
		return nil, fmt.Errorf("start session: %v", err)
	}

	return session, nil
}

//export Start
func Start(ApplicationName *C.char, Pid C.int, SpyName *C.char, ServerAddress *C.char) int {

	s, err := pyspy_session.startNewSession(&config.Exec{
		SpyName:                C.GoString(SpyName),
		ApplicationName:        C.GoString(ApplicationName),
		SampleRate:             100,
		DetectSubprocesses:     true,
		LogLevel:               "debug",
		ServerAddress:          C.GoString(ServerAddress),
		AuthToken:              "",
		UpstreamThreads:        4,
		UpstreamRequestTimeout: time.Second * 10,
		NoLogging:              false,
		NoRootDrop:             false,
		Pid:                    int(Pid),
		UserName:               "",
		GroupName:              "",
		PyspyBlocking:          false,
	})

	if err != nil {
		fmt.Println(err.Error())
		return -1
	}

	pyspy_session.session = s

	return 0
}

//export Stop
func Stop(Pid C.int) {
	pyspy_session.session.Stop()
}

//export ChangeName
func ChangeName(newName *C.char) {
	pyspy_session.session.ChangeName(C.GoString(newName))
}

func main() {
}
