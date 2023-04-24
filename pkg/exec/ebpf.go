//go:build ebpfspy

package exec

import (
	"context"
	"errors"
	"fmt"
	"github.com/pyroscope-io/pyroscope/pkg/agent/ebpfspy"
	sd "github.com/pyroscope-io/pyroscope/pkg/agent/ebpfspy/sd"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/types"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/util/process"
)

func RunEBPF(cfg *config.EBPF) error {
	if cfg.Pid == -1 && cfg.DetectSubprocesses {
		return fmt.Errorf("pid -1 can only be used without dectecting subprocesses")
	}
	if !isRoot() {
		return errors.New("when using eBPF you're required to run the agent with sudo")
	}

	logger := NewLogger(cfg.LogLevel, cfg.NoLogging)

	rc := remote.RemoteConfig{
		AuthToken:              cfg.AuthToken,
		ScopeOrgID:             cfg.ScopeOrgID,
		HTTPHeaders:            cfg.Headers,
		BasicAuthUser:          cfg.BasicAuthUser,
		BasicAuthPassword:      cfg.BasicAuthPassword,
		UpstreamThreads:        cfg.UpstreamThreads,
		UpstreamAddress:        cfg.ServerAddress,
		UpstreamRequestTimeout: cfg.UpstreamRequestTimeout,
	}
	up, err := remote.New(rc, logger)
	if err != nil {
		return fmt.Errorf("new remote upstream: %v", err)
	}

	// if the sample rate is zero, use the default value
	sampleRate := uint32(types.DefaultSampleRate)
	if cfg.SampleRate != 0 {
		sampleRate = uint32(cfg.SampleRate)
	}

	appName := CheckApplicationName(logger, cfg.ApplicationName, spy.EBPF, []string{})

	var serviceDiscovery sd.ServiceDiscovery = sd.NoopServiceDiscovery{}
	if cfg.KubernetesNode != "" {
		serviceDiscovery, err = sd.NewK8ServiceDiscovery(context.TODO(), logger, cfg.KubernetesNode)
		if err != nil {
			return err
		}
	}

	logger.Debug("starting command")

	// The channel buffer capacity should be sufficient to be keep up with
	// the expected signal rate (in case of Exec all the signals to be relayed
	// to the child process)
	ch := make(chan os.Signal, 10)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	defer func() {
		signal.Stop(ch)
		close(ch)
	}()

	sc := agent.SessionConfig{
		Upstream:         up,
		AppName:          appName,
		Tags:             cfg.Tags,
		ProfilingTypes:   []spy.ProfileType{spy.ProfileCPU},
		SpyName:          spy.EBPF,
		SampleRate:       sampleRate,
		UploadRate:       10 * time.Second,
		Pid:              cfg.Pid,
		WithSubprocesses: cfg.DetectSubprocesses,
		Logger:           logger,
	}
	session, err := agent.NewSessionWithSpyFactory(sc, func(pid int) ([]spy.Spy, error) {
		s := ebpfspy.NewSession(logger, cfg.Pid, sampleRate, cfg.SymbolCacheSize, serviceDiscovery, cfg.OnlyServices)
		err := s.Start()
		if err != nil {
			return nil, err
		}

		res := ebpfspy.NewEBPFSpy(s)
		return []spy.Spy{res}, nil
	})
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}

	up.Start()
	defer up.Stop()

	if err = session.Start(); err != nil {
		return fmt.Errorf("start session: %w", err)
	}
	defer session.Stop()

	// wait for process to exit
	// pid == -1 means we're profiling whole system
	if cfg.Pid == -1 {
		<-ch
		return nil
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ch:
			return nil
		case <-ticker.C:
			if !process.Exists(cfg.Pid) {
				logger.Debugf("child process exited")
				return nil
			}
		}
	}
}
