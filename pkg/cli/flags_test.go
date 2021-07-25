package cli

// import (
// 	"context"
// 	"flag"
// 	"time"

// 	. "github.com/onsi/ginkgo"
// 	. "github.com/onsi/gomega"
// 	"github.com/peterbourgon/ff/v3"
// 	"github.com/peterbourgon/ff/v3/ffcli"

// 	"github.com/pyroscope-io/pyroscope/pkg/config"
// 	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
// )

// type FlagsStruct struct {
// 	Config   string
// 	Foo      string
// 	Foos     []string
// 	Bar      int
// 	Baz      time.Duration
// 	FooBar   string
// 	FooFoo   float64
// 	FooBytes bytesize.ByteSize
// }

// var _ = Describe("flags", func() {
// 	Context("PopulateFlagSet", func() {
// 		Context("without config file", func() {
// 			It("correctly sets all types of arguments", func() {
// 				exampleFlagSet := flag.NewFlagSet("example flag set", flag.ExitOnError)
// 				cfg := FlagsStruct{}
// 				PopulateFlagSet(&cfg, exampleFlagSet)

// 				exampleCommand := &ffcli.Command{
// 					FlagSet: exampleFlagSet,
// 					Exec: func(_ context.Context, args []string) error {
// 						return nil
// 					},
// 				}

// 				err := exampleCommand.ParseAndRun(context.Background(), []string{
// 					"-foo", "test-val-1",
// 					"-foos", "test-val-2",
// 					"-foos", "test-val-3",
// 					"-bar", "123",
// 					"-baz", "10h",
// 					"-foo-bar", "test-val-4",
// 					"-foo-foo", "10.23",
// 					"-foo-bytes", "100MB",
// 				})

// 				Expect(err).ToNot(HaveOccurred())
// 				Expect(cfg.Foo).To(Equal("test-val-1"))
// 				Expect(cfg.Foos).To(Equal([]string{"test-val-2", "test-val-3"}))
// 				Expect(cfg.Bar).To(Equal(123))
// 				Expect(cfg.Baz).To(Equal(10 * time.Hour))
// 				Expect(cfg.FooBar).To(Equal("test-val-4"))
// 				Expect(cfg.FooFoo).To(Equal(10.23))
// 				Expect(cfg.FooBytes).To(Equal(100 * bytesize.MB))
// 			})
// 		})

// 		Context("with config file", func() {
// 			It("correctly sets all types of arguments", func() {
// 				exampleFlagSet := flag.NewFlagSet("example flag set", flag.ExitOnError)
// 				cfg := FlagsStruct{}
// 				PopulateFlagSet(&cfg, exampleFlagSet)

// 				exampleCommand := &ffcli.Command{
// 					FlagSet: exampleFlagSet,
// 					Options: []ff.Option{
// 						ff.WithConfigFileParser(parser),
// 						ff.WithConfigFileFlag("config"),
// 					},
// 					Exec: func(_ context.Context, args []string) error {
// 						return nil
// 					},
// 				}

// 				err := exampleCommand.ParseAndRun(context.Background(), []string{
// 					"-config", "testdata/example.yml",
// 				})

// 				Expect(err).ToNot(HaveOccurred())
// 				Expect(cfg.Foo).To(Equal("test-val-1"))
// 				Expect(cfg.Foos).To(Equal([]string{"test-val-2", "test-val-3"}))
// 				Expect(cfg.Bar).To(Equal(123))
// 				Expect(cfg.Baz).To(Equal(10 * time.Hour))
// 				Expect(cfg.FooBar).To(Equal("test-val-4"))
// 				Expect(cfg.FooFoo).To(Equal(10.23))
// 				Expect(cfg.FooBytes).To(Equal(100 * bytesize.MB))
// 			})

// 			It("arguments take precedence", func() {
// 				exampleFlagSet := flag.NewFlagSet("example flag set", flag.ExitOnError)
// 				cfg := FlagsStruct{}
// 				PopulateFlagSet(&cfg, exampleFlagSet)

// 				exampleCommand := &ffcli.Command{
// 					FlagSet: exampleFlagSet,
// 					Options: []ff.Option{
// 						ff.WithConfigFileParser(parser),
// 						ff.WithConfigFileFlag("config"),
// 					},
// 					Exec: func(_ context.Context, args []string) error {
// 						return nil
// 					},
// 				}

// 				err := exampleCommand.ParseAndRun(context.Background(), []string{
// 					"-config", "testdata/example.yml",
// 					"-foo", "test-val-4",
// 				})

// 				Expect(err).ToNot(HaveOccurred())
// 				Expect(cfg.Foo).To(Equal("test-val-4"))
// 			})

// 			It("agent configuration", func() {
// 				exampleFlagSet := flag.NewFlagSet("example flag set", flag.ExitOnError)
// 				var cfg config.Agent
// 				PopulateFlagSet(&cfg, exampleFlagSet)

// 				exampleCommand := &ffcli.Command{
// 					FlagSet: exampleFlagSet,
// 					Options: []ff.Option{
// 						ff.WithIgnoreUndefined(true),
// 						ff.WithConfigFileParser(parser),
// 						ff.WithConfigFileFlag("config"),
// 					},
// 					Exec: func(_ context.Context, args []string) error {
// 						return nil
// 					},
// 				}

// 				err := exampleCommand.ParseAndRun(context.Background(), []string{
// 					"-config", "testdata/agent.yml",
// 					"-tag", "baz=zzz",
// 				})

// 				Expect(err).ToNot(HaveOccurred())
// 				Expect(cfg).To(Equal(config.Agent{
// 					Config:                 "testdata/agent.yml",
// 					LogLevel:               "debug",
// 					NoLogging:              false,
// 					ServerAddress:          "http://localhost:4040",
// 					AuthToken:              "",
// 					UpstreamThreads:        4,
// 					UpstreamRequestTimeout: 10 * time.Second,
// 					Tags: map[string]string{
// 						"baz": "zzz",
// 					},
// 				}))

// 				Expect(loadAgentConfig(&cfg)).ToNot(HaveOccurred())
// 				Expect(cfg).To(Equal(config.Agent{
// 					Config:                 "testdata/agent.yml",
// 					LogLevel:               "debug",
// 					NoLogging:              false,
// 					ServerAddress:          "http://localhost:4040",
// 					AuthToken:              "",
// 					UpstreamThreads:        4,
// 					UpstreamRequestTimeout: 10 * time.Second,
// 					Tags: map[string]string{
// 						"foo": "bar",
// 						"baz": "zzz",
// 					},
// 					Targets: []config.Target{
// 						{
// 							ServiceName:        "foo",
// 							SpyName:            "debugspy",
// 							ApplicationName:    "foo.app",
// 							SampleRate:         0,
// 							DetectSubprocesses: false,
// 							PyspyBlocking:      false,
// 							RbspyBlocking:      false,
// 							Tags: map[string]string{
// 								"foo": "bar",
// 								"baz": "zzz",
// 							},
// 						},
// 					},
// 				}))
// 			})

// 			It("parses tag flags in exec", func() {
// 				exampleFlagSet := flag.NewFlagSet("example flag set", flag.ExitOnError)
// 				var cfg config.Exec
// 				PopulateFlagSet(&cfg, exampleFlagSet)

// 				exampleCommand := &ffcli.Command{
// 					FlagSet: exampleFlagSet,
// 					Options: []ff.Option{
// 						ff.WithIgnoreUndefined(true),
// 						ff.WithConfigFileParser(parser),
// 						ff.WithConfigFileFlag("config"),
// 					},
// 					Exec: func(_ context.Context, args []string) error {
// 						return nil
// 					},
// 				}

// 				err := exampleCommand.ParseAndRun(context.Background(), []string{
// 					"-tag", "foo=bar",
// 					"-tag", "baz=qux",
// 				})

// 				Expect(err).ToNot(HaveOccurred())
// 				Expect(cfg).To(Equal(config.Exec{
// 					SpyName:                "auto",
// 					SampleRate:             100,
// 					DetectSubprocesses:     true,
// 					LogLevel:               "info",
// 					ServerAddress:          "http://localhost:4040",
// 					UpstreamThreads:        4,
// 					UpstreamRequestTimeout: 10 * time.Second,
// 					Tags: map[string]string{
// 						"foo": "bar",
// 						"baz": "qux",
// 					},
// 				}))
// 			})
// 		})
// 	})
// })
