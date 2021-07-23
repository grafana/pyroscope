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
	"github.com/pyroscope-io/pyroscope/pkg/agent/pyspy"
)

var Sessions = map[int]*agent.ProfileSession{}

func startNewSession(cfg *config.Exec) (*agent.ProfileSession, error) {
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
	pid := int(Pid)

	if _, ok := Sessions[pid]; ok {
		fmt.Println(fmt.Errorf("session for pid: %d already exists", pid))
		return -2
	}

	s, err := startNewSession(&config.Exec{
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
		Pid:                    pid,
		UserName:               "",
		GroupName:              "",
		PyspyBlocking:          false,
	})

	if err != nil {
		fmt.Println(err.Error())
		return -1
	}

	Sessions[pid] = s

	return 0
}

//export Stop
func Stop(Pid C.int) int {
	pid := int(Pid)

	if _, ok := Sessions[pid]; !ok {
		fmt.Println(fmt.Errorf("session for pid: %d doesn't exists", pid))
		return -1
	}
	Sessions[int(Pid)].Stop()
	return 0
}

//export ChangeName
func ChangeName(newName *C.char, Pid C.int) int {
	pid := int(Pid)

	if _, ok := Sessions[pid]; !ok {
		fmt.Println(fmt.Errorf("session for pid: %d doesn't exists", pid))
		return -1
	}
	Sessions[int(Pid)].ChangeName(C.GoString(newName))
	return 0
}

func main() {
}
