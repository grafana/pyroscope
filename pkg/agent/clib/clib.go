// +build clib

// Package main deals with ruby / python / php integrations
package main

import (
	"C"
	"time"

	"os"
	"sync"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote"
	"github.com/pyroscope-io/pyroscope/pkg/build"

	"github.com/pyroscope-io/pyroscope/pkg/agent/pyspy"
)

var sessionMutex sync.Mutex
var session *agent.ProfileSession

//export Start
func Start(applicationName *C.char, spyName *C.char, serverAddress *C.char, authToken *C.char, sampleRate C.int, withSubprocesses C.int, logLevel *C.char) int {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()

	pyspy.Blocking = false

	if err := performOSChecks(); pyspy.Blocking && err != nil {
		logger.Errorf("error happened when starting profiling session: %v", err)
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
		logger.Errorf("error happened when starting profiling session: %v", err)
		return -1
	}

	sc := agent.SessionConfig{
		Upstream:         u,
		AppName:          C.GoString(applicationName),
		ProfilingTypes:   []spy.ProfileType{spy.ProfileCPU},
		SpyName:          C.GoString(spyName),
		SampleRate:       uint32(sampleRate),
		UploadRate:       10 * time.Second,
		Pid:              os.Getpid(),
		WithSubprocesses: withSubprocesses != 0,
		ClibIntegration:  true,
	}
	session, err = agent.NewSession(&sc, logger)
	if err != nil {
		logger.Errorf("error happened when starting profiling session: %v", err)
		return -1
	}
	if err = session.Start(); err != nil {
		logger.Errorf("error happened when starting profiling session: %v", err)
		return -1
	}

	return 0
}

//export Stop
func Stop() int {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()

	session.Stop()
	return 0
}

//export ChangeName
func ChangeName(newName *C.char) int {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()

	session.ChangeName(C.GoString(newName))
	return 0
}

//export SetTag
func SetTag(key *C.char, value *C.char) int {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()

	session.SetTag(C.GoString(key), C.GoString(value))
	return 0
}

//export BuildSummary
func BuildSummary() *C.char {
	return C.CString(build.Summary())
}

func main() {
}
