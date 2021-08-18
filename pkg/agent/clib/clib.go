// +build clib

// Package main deals with ruby / python / php integrations
package main

import (
	"C"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote"
)
import (
	"sync"

	"github.com/pyroscope-io/pyroscope/pkg/agent/pyspy"
)

var sessionsMapMutex sync.Mutex
var sessionsMap = map[int]*agent.ProfileSession{}

//export Start
func Start(cpid C.int, applicationName *C.char, spyName *C.char, serverAddress *C.char, authToken *C.char, sampleRate C.int, withSubprocesses C.int, logLevel *C.char) int {
	sessionsMapMutex.Lock()
	defer sessionsMapMutex.Unlock()

	pid := int(cpid)
	pyspy.Blocking = false

	if _, ok := sessionsMap[pid]; ok {
		logger.Errorf("session for this pid already exists")
		return -1
	}

	if err := performOSChecks(); pyspy.Blocking && err != nil {
		logger.Errorf("error happened when starting profiling session %v", err)
		return -1
	}

	rc := remote.RemoteConfig{
		AuthToken:              C.GoString(authToken),
		UpstreamAddress:        C.GoString(serverAddress),
		UpstreamThreads:        4,
		UpstreamRequestTimeout: 10 * time.Second,
	}
	u, err := remote.New(rc, logger)
	if err != nil {
		logger.Errorf("error happened when starting profiling session %v", err)
		return -1
	}

	sc := agent.SessionConfig{
		Upstream:         u,
		AppName:          C.GoString(applicationName),
		ProfilingTypes:   []spy.ProfileType{spy.ProfileCPU},
		SpyName:          C.GoString(spyName),
		SampleRate:       uint32(sampleRate),
		UploadRate:       10 * time.Second,
		Pid:              pid,
		WithSubprocesses: withSubprocesses != 0,
	}
	session, err := agent.NewSession(&sc, logger)
	sessionsMap[pid] = session
	if err != nil {
		logger.Errorf("error happened when starting profiling session %v", err)
		return -1
	}
	if err = session.Start(); err != nil {
		logger.Errorf("error happened when starting profiling session %v", err)
		return -1
	}

	return 0
}

//export Stop
func Stop(cpid C.int) int {
	sessionsMapMutex.Lock()
	defer sessionsMapMutex.Unlock()

	pid := int(cpid)

	if _, ok := sessionsMap[pid]; !ok {
		logger.Errorf("session for pid: %d doesn't exists", pid)
		return -1
	}
	sessionsMap[int(cpid)].Stop()
	return 0
}

//export ChangeName
func ChangeName(cpid C.int, newName *C.char) int {
	sessionsMapMutex.Lock()
	defer sessionsMapMutex.Unlock()

	pid := int(cpid)

	if _, ok := sessionsMap[pid]; !ok {
		logger.Errorf("session for pid: %d doesn't exists", pid)
		return -1
	}
	sessionsMap[pid].ChangeName(C.GoString(newName))
	return 0
}

//export SetTag
func SetTag(cpid C.int, key *C.char, value *C.char) int {
	sessionsMapMutex.Lock()
	defer sessionsMapMutex.Unlock()

	pid := int(cpid)

	if _, ok := sessionsMap[pid]; !ok {
		logger.Errorf("session for pid: %d doesn't exists", pid)
		return -1
	}
	sessionsMap[pid].SetTag(C.GoString(key), C.GoString(value))
	return 0
}

func main() {
}
