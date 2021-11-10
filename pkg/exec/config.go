package exec

import (
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

type mode int

const (
	modeExec mode = iota + 1
	modeConnect
)

type Config struct {
	mode     mode
	logLevel logrus.Level

	// Spies
	pyspyBlocking bool
	rbspyBlocking bool

	// Connect
	pid int

	// Exec
	noRootDrop bool
	userName   string
	groupName  string

	// Remote upload
	remote.RemoteConfig

	// Session
	spyName            string
	applicationName    string
	sampleRate         uint32
	detectSubprocesses bool
	tags               map[string]string
}

func NewConfig(c *config.Exec) *Config {
	logLevel := logrus.PanicLevel
	if l, err := logrus.ParseLevel(c.LogLevel); !c.NoLogging && err == nil {
		logLevel = l
	}

	// if the sample rate is zero, use the default value
	sampleRate := uint32(spy.DefaultSampleRate)
	if c.SampleRate != 0 {
		sampleRate = uint32(c.SampleRate)
	}

	return &Config{
		mode:          modeExec,
		logLevel:      logLevel,
		pyspyBlocking: c.PyspyBlocking,
		rbspyBlocking: c.RbspyBlocking,
		pid:           c.Pid,
		noRootDrop:    c.NoRootDrop,
		userName:      c.UserName,
		groupName:     c.GroupName,
		RemoteConfig: remote.RemoteConfig{
			AuthToken:              c.AuthToken,
			UpstreamAddress:        c.ServerAddress,
			UpstreamThreads:        c.UpstreamThreads,
			UpstreamRequestTimeout: c.UpstreamRequestTimeout,
		},
		spyName:            c.SpyName,
		applicationName:    c.ApplicationName,
		sampleRate:         sampleRate,
		detectSubprocesses: c.DetectSubprocesses,
		tags:               c.Tags,
	}
}

func (c *Config) WithConnect() *Config {
	c.mode = modeConnect
	return c
}
