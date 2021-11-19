package cli

import (
	"bytes"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	scrape "github.com/pyroscope-io/pyroscope/pkg/scrape/config"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/discovery"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/model"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

type FlagsStruct struct {
	Config   string            `mapstructure:"config"`
	Foo      string            `mapstructure:"foo"`
	Foos     []string          `mapstructure:"foos"`
	Bar      int               `mapstructure:"bar"`
	Baz      time.Duration     `mapstructure:"baz"`
	FooBar   string            `mapstructure:"foo-bar"`
	FooFoo   float64           `mapstructure:"foo-foo"`
	FooBytes bytesize.ByteSize `mapstructure:"foo-bytes"`
	FooDur   time.Duration     `mapstructure:"foo-dur"`
}

var _ = Describe("flags", func() {
	Context("PopulateFlagSet", func() {
		Context("without config file", func() {
			It("correctly sets all types of arguments", func() {
				vpr := viper.New()
				exampleCommand := &cobra.Command{
					RunE: func(cmd *cobra.Command, args []string) error {
						return nil
					},
				}

				cfg := FlagsStruct{}
				PopulateFlagSet(&cfg, exampleCommand.Flags(), vpr)

				b := bytes.NewBufferString("")
				exampleCommand.SetOut(b)
				exampleCommand.SetArgs([]string{
					fmt.Sprintf("--foo=%s", "test-val-1"),
					fmt.Sprintf("--foos=%s", "test-val-2"),
					fmt.Sprintf("--foos=%s", "test-val-3"),
					fmt.Sprintf("--bar=%s", "123"),
					fmt.Sprintf("--baz=%s", "10h"),
					fmt.Sprintf("--foo-bar=%s", "test-val-4"),
					fmt.Sprintf("--foo-foo=%s", "10.23"),
					fmt.Sprintf("--foo-bytes=%s", "100MB"),
				})

				err := exampleCommand.Execute()
				Expect(err).ToNot(HaveOccurred())
				Expect(cfg.Foo).To(Equal("test-val-1"))
				Expect(cfg.Foos).To(Equal([]string{"test-val-2", "test-val-3"}))
				Expect(cfg.Bar).To(Equal(123))
				Expect(cfg.Baz).To(Equal(10 * time.Hour))
				Expect(cfg.FooBar).To(Equal("test-val-4"))
				Expect(cfg.FooFoo).To(Equal(10.23))
				Expect(cfg.FooBytes).To(Equal(100 * bytesize.MB))
			})
		})

		Context("with config file", func() {
			It("correctly sets all types of arguments", func() {
				cfg := FlagsStruct{}
				vpr := viper.New()
				exampleCommand := &cobra.Command{
					RunE: func(cmd *cobra.Command, args []string) error {
						if cfg.Config != "" {
							// Use config file from the flag.
							vpr.SetConfigFile(cfg.Config)

							// If a config file is found, read it in.
							if err := vpr.ReadInConfig(); err == nil {
								fmt.Fprintln(os.Stderr, "Using config file:", vpr.ConfigFileUsed())
							}

							if err := Unmarshal(vpr, &cfg); err != nil {
								fmt.Fprintln(os.Stderr, "Unable to unmarshal:", err)
							}

							fmt.Printf("configuration is %+v \n", cfg)
						}

						return nil
					},
				}

				PopulateFlagSet(&cfg, exampleCommand.Flags(), vpr)
				vpr.BindPFlags(exampleCommand.Flags())

				b := bytes.NewBufferString("")
				exampleCommand.SetOut(b)
				exampleCommand.SetArgs([]string{fmt.Sprintf("--config=%s", "testdata/example.yml")})

				err := exampleCommand.Execute()
				Expect(err).ToNot(HaveOccurred())
				Expect(cfg.Foo).To(Equal("test-val-1"))
				Expect(cfg.Foos).To(Equal([]string{"test-val-2", "test-val-3"}))
				Expect(cfg.Bar).To(Equal(123))
				Expect(cfg.Baz).To(Equal(10 * time.Hour))
				Expect(cfg.FooBar).To(Equal("test-val-4"))
				Expect(cfg.FooFoo).To(Equal(10.23))
				Expect(cfg.FooBytes).To(Equal(100 * bytesize.MB))
				Expect(cfg.FooDur).To(Equal(5*time.Minute + 23*time.Second))
			})

			It("arguments take precedence", func() {
				cfg := FlagsStruct{}
				vpr := viper.New()
				exampleCommand := &cobra.Command{
					RunE: func(cmd *cobra.Command, args []string) error {
						if cfg.Config != "" {
							// Use config file from the flag.
							vpr.SetConfigFile(cfg.Config)

							// If a config file is found, read it in.
							if err := vpr.ReadInConfig(); err == nil {
								fmt.Fprintln(os.Stderr, "Using config file:", vpr.ConfigFileUsed())
							}

							if err := Unmarshal(vpr, &cfg); err != nil {
								fmt.Fprintln(os.Stderr, "Unable to unmarshal:", err)
							}

							fmt.Printf("configuration is %+v \n", cfg)
						}

						return nil
					},
				}

				PopulateFlagSet(&cfg, exampleCommand.Flags(), vpr)
				vpr.BindPFlags(exampleCommand.Flags())

				b := bytes.NewBufferString("")
				exampleCommand.SetOut(b)
				exampleCommand.SetArgs([]string{
					fmt.Sprintf("--config=%s", "testdata/example.yml"),
					fmt.Sprintf("--foo=%s", "test-val-4"),
					fmt.Sprintf("--foo-dur=%s", "3h"),
				})

				err := exampleCommand.Execute()
				Expect(err).ToNot(HaveOccurred())
				Expect(cfg.Foo).To(Equal("test-val-4"))
				Expect(cfg.FooDur).To(Equal(3 * time.Hour))
			})
			It("server configuration", func() {
				var cfg config.Server
				vpr := viper.New()
				exampleCommand := &cobra.Command{
					RunE: func(cmd *cobra.Command, args []string) error {
						if cfg.Config != "" {
							// Use config file from the flag.
							vpr.SetConfigFile(cfg.Config)

							// If a config file is found, read it in.
							if err := vpr.ReadInConfig(); err == nil {
								fmt.Fprintln(os.Stderr, "Using config file:", vpr.ConfigFileUsed())
							}

							if err := Unmarshal(vpr, &cfg); err != nil {
								fmt.Fprintln(os.Stderr, "Unable to unmarshal:", err)
							}

							Expect(loadScrapeConfigsFromFile(&cfg)).ToNot(HaveOccurred())
							fmt.Printf("configuration is %+v \n", cfg)
						}

						return nil
					},
				}

				PopulateFlagSet(&cfg, exampleCommand.Flags(), vpr, WithSkip("scrape-configs"))
				vpr.BindPFlags(exampleCommand.Flags())

				b := bytes.NewBufferString("")
				exampleCommand.SetOut(b)
				exampleCommand.SetArgs([]string{
					"--config=testdata/server.yml",
					"--log-level=debug",
				})

				err := exampleCommand.Execute()
				Expect(err).ToNot(HaveOccurred())
				Expect(cfg).To(Equal(config.Server{
					AnalyticsOptOut:         false,
					Config:                  "testdata/server.yml",
					LogLevel:                "debug",
					BadgerLogLevel:          "error",
					StoragePath:             "/var/lib/pyroscope",
					APIBindAddr:             ":4040",
					BaseURL:                 "",
					CacheEvictThreshold:     0.25,
					CacheEvictVolume:        0.33,
					BadgerNoTruncate:        false,
					DisablePprofEndpoint:    false,
					EnableExperimentalAdmin: false,
					MaxNodesSerialization:   2048,
					MaxNodesRender:          8192,
					HideApplications:        []string{},
					Retention:               0,
					RetentionLevels: config.RetentionLevels{
						Zero: 100 * time.Second,
						One:  1000 * time.Second,
					},
					SampleRate:          0,
					OutOfSpaceThreshold: 0,
					CacheDimensionSize:  0,
					CacheDictionarySize: 0,
					CacheSegmentSize:    0,
					CacheTreeSize:       0,
					Auth: config.Auth{
						Google: config.GoogleOauth{
							Enabled:        false,
							ClientID:       "",
							ClientSecret:   "",
							RedirectURL:    "",
							AuthURL:        "https://accounts.google.com/o/oauth2/auth",
							TokenURL:       "https://accounts.google.com/o/oauth2/token",
							AllowedDomains: []string{},
						},
						Gitlab: config.GitlabOauth{
							Enabled:       false,
							ClientID:      "",
							ClientSecret:  "",
							RedirectURL:   "",
							AuthURL:       "https://gitlab.com/oauth/authorize",
							TokenURL:      "https://gitlab.com/oauth/token",
							APIURL:        "https://gitlab.com/api/v4",
							AllowedGroups: []string{},
						},
						Github: config.GithubOauth{
							Enabled:              false,
							ClientID:             "",
							ClientSecret:         "",
							RedirectURL:          "",
							AuthURL:              "https://github.com/login/oauth/authorize",
							TokenURL:             "https://github.com/login/oauth/access_token",
							AllowedOrganizations: []string{},
						},
						JWTSecret:                "",
						LoginMaximumLifetimeDays: 0,
					},

					MetricsExportRules: config.MetricsExportRules{
						"my_metric_name": {
							Expr:    `app.name{foo=~"bar"}`,
							Node:    "a;b;c",
							GroupBy: []string{"foo"},
						},
					},
					AdminSocketPath: "/tmp/pyroscope.sock",

					ScrapeConfigs: []*scrape.Config{
						{
							JobName:          "testing",
							EnabledProfiles:  []string{"cpu", "mem"},
							Profiles:         scrape.DefaultConfig().Profiles,
							ScrapeInterval:   10 * time.Second,
							ScrapeTimeout:    15 * time.Second,
							Scheme:           "http",
							HTTPClientConfig: scrape.DefaultHTTPClientConfig,
							ServiceDiscoveryConfigs: []discovery.Config{
								discovery.StaticConfig{
									{
										Targets: []model.LabelSet{
											{"__address__": "localhost:6060", "__name__": "app"},
										},
										Labels: model.LabelSet{"foo": "bar"},
										Source: "0",
									},
								},
							},
						},
					},
				}))
			})

			It("agent configuration", func() {
				var cfg config.Agent
				vpr := viper.New()
				exampleCommand := &cobra.Command{
					Run: func(cmd *cobra.Command, args []string) {
						Expect(vpr.BindPFlags(cmd.Flags())).ToNot(HaveOccurred())
						vpr.SetConfigFile(cfg.Config)
						Expect(vpr.ReadInConfig()).ToNot(HaveOccurred())
						Expect(Unmarshal(vpr, &cfg)).ToNot(HaveOccurred())
						Expect(loadAgentConfig(&cfg)).ToNot(HaveOccurred())
					},
				}

				PopulateFlagSet(&cfg, exampleCommand.Flags(), vpr)
				exampleCommand.SetArgs([]string{
					"--config=testdata/agent.yml",
					"--log-level=debug",
					"--tag=foo=xxx",
				})

				Expect(exampleCommand.Execute()).ToNot(HaveOccurred())
				Expect(cfg).To(Equal(config.Agent{
					Config:                 "testdata/agent.yml",
					LogLevel:               "debug",
					NoLogging:              false,
					ServerAddress:          "http://localhost:4040",
					AuthToken:              "",
					UpstreamThreads:        4,
					UpstreamRequestTimeout: 10 * time.Second,
					Targets: []config.Target{
						{
							ServiceName:        "foo",
							SpyName:            "debugspy",
							ApplicationName:    "foo.app",
							SampleRate:         0,
							DetectSubprocesses: false,
							PyspyBlocking:      false,
							Tags: map[string]string{
								"foo": "xxx",
								"baz": "qux",
							},
						},
					},
					Tags: map[string]string{
						"foo": "xxx",
						"baz": "qux",
					},
				}))
			})
		})
	})
})
