package target

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/types"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/util/names"
)

const (
	defaultBackoffPeriod = time.Second * 10
)

// Manager tracks targets and attaches spies to running processes.
type Manager struct {
	ctx    context.Context
	cancel context.CancelFunc

	logger *logrus.Logger
	remote *remote.Remote
	config *config.Agent
	stop   chan struct{}
	wg     sync.WaitGroup

	resolve       func(config.Target) (target, bool)
	backoffPeriod time.Duration
}

type target interface {
	// attach blocks till the context cancellation or the target
	// process exit, whichever occurs first.
	attach(ctx context.Context)
}

func NewManager(l *logrus.Logger, r *remote.Remote, c *config.Agent) *Manager {
	mgr := Manager{
		logger:        l,
		remote:        r,
		config:        c,
		stop:          make(chan struct{}),
		backoffPeriod: defaultBackoffPeriod,
	}
	mgr.ctx, mgr.cancel = context.WithCancel(context.Background())
	mgr.resolve = mgr.resolveTarget
	return &mgr
}

func (mgr *Manager) canonise(t *config.Target) error {
	if t.SpyName == types.GoSpy {
		return fmt.Errorf("gospy can not profile other processes")
	}
	var found bool
	for _, s := range spy.SupportedSpies {
		if s == t.SpyName {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("spy %q is not supported", t.SpyName)
	}
	if t.SampleRate == 0 {
		t.SampleRate = types.DefaultSampleRate
	}
	if t.ApplicationName == "" {
		t.ApplicationName = t.SpyName + "." + names.GetRandomName(generateSeed(t.ServiceName, t.SpyName))
		logger := mgr.logger.WithField("spy-name", t.SpyName)
		if t.ServiceName != "" {
			logger = logger.WithField("service-name", t.ServiceName)
		}
		logger.Infof("we recommend specifying application name via 'application-name' parameter")
		logger.Infof("for now we chose the name for you and it's %q", t.ApplicationName)
	}
	return nil
}

func (mgr *Manager) Start() {
	for _, t := range mgr.config.Targets {
		var tgt target
		var ok bool
		err := mgr.canonise(&t)
		if err == nil {
			tgt, ok = mgr.resolve(t)
			if !ok {
				err = fmt.Errorf("unknown target type")
			}
		}
		if err != nil {
			mgr.logger.
				WithField("app-name", t.ApplicationName).
				WithField("spy-name", t.SpyName).
				WithError(err).Error("failed to setup target")
			continue
		}
		mgr.wg.Add(1)
		go mgr.runTarget(tgt)
	}
}

func (mgr *Manager) Stop() {
	mgr.cancel()
	close(mgr.stop)
	mgr.wg.Wait()
}

func (mgr *Manager) resolveTarget(t config.Target) (target, bool) {
	var tgt target
	switch {
	case t.ServiceName != "":
		tgt = newServiceTarget(mgr.logger, mgr.remote, t)
	default:
		return nil, false
	}
	return tgt, true
}

func (mgr *Manager) runTarget(t target) {
	ticker := time.NewTicker(mgr.backoffPeriod)
	defer func() {
		ticker.Stop()
		mgr.wg.Done()
	}()
	for {
		select {
		default:
		case <-mgr.stop:
			return
		}
		t.attach(mgr.ctx)
		// Unless manager is stopped, run spy again after some backoff
		// period regardless of the exit reason.
		select {
		case <-ticker.C:
		case <-mgr.stop:
			return
		}
	}
}

func generateSeed(args ...string) string {
	path, err := os.Getwd()
	if err != nil {
		path = "<unknown>"
	}
	return path + "|" + strings.Join(args, "&")
}
