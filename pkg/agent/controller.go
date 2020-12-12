package agent

import (
	"os/user"
	"runtime"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"

	"github.com/petethepig/pyroscope/pkg/agent/csock"
	"github.com/petethepig/pyroscope/pkg/agent/upstream"
	"github.com/petethepig/pyroscope/pkg/config"
	"github.com/petethepig/pyroscope/pkg/util/id"
)

type Controller struct {
	cfg            *config.Config
	upstream       upstream.Upstream
	cs             *csock.CSock
	activeProfiles map[int]*profileSession
	id             id.ID
}

func NewController(cfg *config.Config, u upstream.Upstream) *Controller {
	return &Controller{
		cfg:            cfg,
		upstream:       u,
		activeProfiles: make(map[int]*profileSession),
	}
}

func (a *Controller) Start() {
	a.upstream.Start()
}

func (a *Controller) Stop() {
}

func (a *Controller) StartContinuousProfiling(spyName, metricName string, pid int) {
	logrus.Info("StartContinuousProfiling")
	// TODO: This logic is not particularly great, need to change later. e.g maybe ticker
	//   might deviate over time?
	// I also think this logic should be in session
	now := time.Now()

	// TODO: should be configurable or even picked up from server
	period := 10 * time.Second
	nextPeriodStartTime := now.Truncate(period).Add(period)
	time.Sleep(nextPeriodStartTime.Sub(now))

	logrus.WithFields(logrus.Fields{
		"now":                 now,
		"nextPeriodStartTime": nextPeriodStartTime,
		"dur":                 nextPeriodStartTime.Sub(now),
		"now-now":             time.Now(),
	}).Info("self profiling start")
	t := time.NewTicker(period)
	for {
		<-t.C
		profileID := a.StartProfiling(spyName, pid)
		time.Sleep(9500 * time.Millisecond) // TODO: horrible, need to fix later
		a.StopProfiling(profileID, metricName)
	}
}

func (a *Controller) StartProfiling(spyName string, pid int) int {
	s := newSession(spyName, pid)
	profileID := int(a.id.Next())
	a.activeProfiles[profileID] = s

	err := s.start()
	if err != nil {
		log.WithFields(log.Fields{
			"spyName": spyName,
			"pid":     strconv.Itoa(pid),
		}).Error("failed to start spy session")
		printDarwinMessage()
	}
	return profileID
}

func (a *Controller) StopProfiling(profileID int, name string) {
	if sess, ok := a.activeProfiles[profileID]; ok {
		t := sess.stop()
		// TODO: these should be passed from integrations
		a.upstream.Upload(name, sess.startTime, sess.stopTime, t)
	} else {
		log.Debugf("failed to find spy session: %d", profileID)
	}
}

func isRoot() bool {
	u, err := user.Current()
	return err == nil && u.Username == "root"
}

func printDarwinMessage() {
	if runtime.GOOS == "darwin" {
		if !isRoot() {
			log.Error("on macOS it is required to run the agent with sudo")
		}
	}
}
