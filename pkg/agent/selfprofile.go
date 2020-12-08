package agent

import (
	"time"

	"github.com/petethepig/pyroscope/pkg/agent/upstream"
	"github.com/petethepig/pyroscope/pkg/config"
	"github.com/sirupsen/logrus"
)

func SelfProfile(cfg *config.Config, u upstream.Upstream, name string) {
	ctrl := newController(cfg, u)
	ctrl.Start()

	// TODO: This logic is not particularly great, need to change later. e.g maybe ticker
	//   might deviate over time?
	// I also think this logic should be in session
	now := time.Now()

	// TODO: should be configurable
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
		profileID := ctrl.StartProfiling("gospy", 0)
		time.Sleep(9500 * time.Millisecond) // TODO: horrible, need to fix later
		ctrl.StopProfiling(profileID, name)
	}
}
