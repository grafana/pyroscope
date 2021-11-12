package exec

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
)

var _ = Describe("Exec config", func() {
	Context("Default config", func() {
		cfg := config.Exec{
			SpyName:                "spy-name",
			ApplicationName:        "application-name",
			SampleRate:             100,
			DetectSubprocesses:     true,
			LogLevel:               "info",
			ServerAddress:          "http://localhost:4040",
			AuthToken:              "auth-token",
			UpstreamThreads:        4,
			UpstreamRequestTimeout: 10 * time.Second,
			NoLogging:              false,
			NoRootDrop:             true,
			Pid:                    1000,
			UserName:               "user-name",
			GroupName:              "group-name",
			PyspyBlocking:          true,
			RbspyBlocking:          true,
			Tags:                   map[string]string{"key": "value"},
		}

		It("NewConfig returns exec config", func() {
			c := NewConfig(&cfg)
			Expect(c.mode).To(Equal(modeExec))
			Expect(c.spyName).To(Equal("spy-name"))
			Expect(c.applicationName).To(Equal("application-name"))
			Expect(c.sampleRate).To(Equal(uint32(100)))
			Expect(c.detectSubprocesses).To(BeTrue())
			Expect(c.logLevel).To(Equal(logrus.InfoLevel))
			Expect(c.RemoteConfig.UpstreamAddress).To(Equal(cfg.ServerAddress))
			Expect(c.RemoteConfig.AuthToken).To(Equal(cfg.AuthToken))
			Expect(c.RemoteConfig.UpstreamThreads).To(Equal(cfg.UpstreamThreads))
			Expect(c.RemoteConfig.UpstreamRequestTimeout).To(Equal(cfg.UpstreamRequestTimeout))
			Expect(c.noRootDrop).To(BeTrue())
			Expect(c.pid).To(Equal(1000))
			Expect(c.userName).To(Equal("user-name"))
			Expect(c.groupName).To(Equal("group-name"))
			Expect(c.pyspyBlocking).To(BeTrue())
			Expect(c.rbspyBlocking).To(BeTrue())
			Expect(c.tags).To(HaveLen(1))
			Expect(c.tags).To(HaveKeyWithValue("key", "value"))
		})

		It("WithConnect returns connect config", func() {
			c := NewConfig(&cfg).WithConnect()
			Expect(c.mode).To(Equal(modeConnect))
		})

		It("WithAdhoc returns adhoc config", func() {
			c := NewConfig(&cfg).WithAdhoc()
			Expect(c.mode).To(Equal(modeAdhoc))
		})
	})

	Context("Config with no logging", func() {
		It("NewConfig returns config with Panic log level", func() {
			c := NewConfig(&config.Exec{NoLogging: true})
			Expect(c.logLevel).To(Equal(logrus.PanicLevel))
		})
	})

	Context("Config without sample rate", func() {
		It("NewConfig returns config with default sampling rate", func() {
			c := NewConfig(&config.Exec{})
			Expect(c.sampleRate).To(Equal(uint32(100)))
		})
	})
})
