package agent

import (
	"strconv"

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

func newController(cfg *config.Config, u upstream.Upstream) *Controller {
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

func (a *Controller) StartProfiling(spyName string, pid int) int {
	s := newSession(spyName, pid)
	profileID := int(a.id.Next())
	a.activeProfiles[profileID] = s

	err := s.start()
	if err != nil {
		log.WithFields(log.Fields{
			"spyName": spyName,
			"pid":     strconv.Itoa(pid),
		}).Debug("failed to start spy session")
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
