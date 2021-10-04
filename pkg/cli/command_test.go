package cli

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

type SubStruct struct {
	Bar string `mapstructure:"bar" def:"def-value"`
}

type TestConfig struct {
	// regular field
	Foo string `mapstructure:"foo" def:"def-value"`
	// nested field
	FooStruct SubStruct `mapstructure:"foo-struct"`
	// config file path
	Config string `mapstructure:"config"`
	// lists
	Foos []string `mapstructure:"foos" def:"def-1,def-2"`
	// other types
	Bar      int               `mapstructure:"bar"`
	Baz      time.Duration     `mapstructure:"baz"`
	FooBytes bytesize.ByteSize `mapstructure:"foo-bytes"`
	FooDur   time.Duration     `mapstructure:"foo-dur"`
}

func (cfg TestConfig) Path() string {
	return cfg.Config
}

func runTestNoChecks(args []string, env map[string]string, cb func(*TestConfig)) (bool, error) {
	cfg := &TestConfig{
		FooStruct: SubStruct{},
	}

	prevValues := make(map[string]string)
	for k, v := range env {
		prevValues[k] = os.Getenv(k)
		os.Setenv(k, v)
	}
	defer func() {
		for k := range env {
			os.Setenv(k, prevValues[k])
		}
	}()

	ran := false
	vpr := NewViper("PYROSCOPE")
	cmd := &cobra.Command{
		RunE: CreateCmdRunFn(cfg, vpr, func(cmd *cobra.Command, args []string) error {
			cb(cfg)
			ran = true
			return nil
		}),
	}
	cmd.SetArgs(args)

	PopulateFlagSet(cfg, cmd.Flags(), vpr)

	err := cmd.Execute()
	return ran, err
}

func runTest(args []string, env map[string]string, cb func(*TestConfig)) {
	ran, err := runTestNoChecks(args, env, cb)

	Expect(err).ToNot(HaveOccurred())
	Expect(ran).To(BeTrue())
}

// runTest([]string{"--foo arg-value"}, map[string]string{}, func(cfg *TestConfig) {
// 	Expect(cfg.Foo).To(Equal("arg-value"))
// })
// runTest([]string{"-foo=arg-value"}, map[string]string{}, func(cfg *TestConfig) {
// 	Expect(cfg.Foo).To(Equal("arg-value"))
// })
// runTest([]string{"--foo=arg-value"}, map[string]string{}, func(cfg *TestConfig) {
// 	Expect(cfg.Foo).To(Equal("arg-value"))
// })

var _ = Describe("CreateCmdRunFn", func() {
	Context("config file", func() {
		Context("config file is set via an argument", func() {
			It("sets value from config file", func() {
				runTest([]string{"--config", "testdata/clitest.yml"}, map[string]string{}, func(cfg *TestConfig) {
					Expect(cfg.Foo).To(Equal("config-value"))
				})
			})
		})
		Context("config file is set via an argument", func() {
			It("sets value from config file", func() {
				runTest([]string{}, map[string]string{"PYROSCOPE_CONFIG": "testdata/clitest.yml"}, func(cfg *TestConfig) {
					Expect(cfg.Foo).To(Equal("config-value"))
				})
			})
		})
		Context("config file that doesn't exist", func() {
			It("sets value from config file", func() {
				ran, err := runTestNoChecks([]string{}, map[string]string{"PYROSCOPE_CONFIG": "testdata/doesntexist"}, func(cfg *TestConfig) {
					Expect(cfg.Foo).To(Equal("config-value"))
				})
				Expect(err).To(HaveOccurred())
				Expect(ran).ToNot(BeTrue())
			})
		})
		Context("config file that doesn't have yaml extension", func() {
			It("sets value from config file", func() {
				runTest([]string{}, map[string]string{"PYROSCOPE_CONFIG": "testdata/clitest.non-yml-extension"}, func(cfg *TestConfig) {
					Expect(cfg.Foo).To(Equal("config-value"))
				})
			})
		})
	})
	Context("configuration sources", func() {
		Context("with no arguments or env variables or config", func() {
			It("sets default value provided via `def` value tag", func() {
				runTest([]string{}, map[string]string{}, func(cfg *TestConfig) {
					Expect(cfg.Foo).To(Equal("def-value"))
				})
			})
		})

		Context("when arguments provided", func() {
			It("sets value from argument", func() {
				runTest([]string{"--foo", "arg-value"}, map[string]string{}, func(cfg *TestConfig) {
					Expect(cfg.Foo).To(Equal("arg-value"))
				})
			})
		})

		Context("when env variables provided", func() {
			It("sets value from env variable", func() {
				runTest([]string{""}, map[string]string{"PYROSCOPE_FOO": "env-value"}, func(cfg *TestConfig) {
					Expect(cfg.Foo).To(Equal("env-value"))
				})
			})
		})

		Context("when config file is provided", func() {
			It("sets value from config file", func() {
				runTest([]string{"--config", "testdata/clitest.yml"}, map[string]string{}, func(cfg *TestConfig) {
					Expect(cfg.Foo).To(Equal("config-value"))
				})
			})
		})

		Context("config precendence", func() {
			It("arguments are most important", func() {
				runTest([]string{"--config", "testdata/clitest.yml", "--foo", "arg-value"}, map[string]string{"PYROSCOPE_FOO": "env-value"}, func(cfg *TestConfig) {
					Expect(cfg.Foo).To(Equal("arg-value"))
				})
			})
			It("env variables are second most important", func() {
				runTest([]string{"--config", "testdata/clitest.yml"}, map[string]string{"PYROSCOPE_FOO": "env-value"}, func(cfg *TestConfig) {
					Expect(cfg.Foo).To(Equal("env-value"))
				})
			})
		})
	})

	Context("substructs", func() {
		Context("with no arguments or env variables or config", func() {
			It("sets default value provided via `def` value tag", func() {
				runTest([]string{}, map[string]string{}, func(cfg *TestConfig) {
					Expect(cfg.FooStruct.Bar).To(Equal("def-value"))
				})
			})
		})

		Context("when arguments provided", func() {
			It("sets value from argument", func() {
				runTest([]string{"--foo-struct.bar", "arg-value"}, map[string]string{}, func(cfg *TestConfig) {
					Expect(cfg.FooStruct.Bar).To(Equal("arg-value"))
				})
			})
		})

		Context("when env variables provided", func() {
			It("sets value from env variable", func() {
				runTest([]string{""}, map[string]string{"PYROSCOPE_FOO_STRUCT_BAR": "env-value"}, func(cfg *TestConfig) {
					Expect(cfg.FooStruct.Bar).To(Equal("env-value"))
				})
			})
		})

		Context("when config file is provided", func() {
			It("sets value from config file", func() {
				runTest([]string{"--config", "testdata/clitest.yml"}, map[string]string{}, func(cfg *TestConfig) {
					Expect(cfg.FooStruct.Bar).To(Equal("config-value"))
				})
			})
		})
	})

	Context("lists", func() {
		Context("when arguments provided", func() {
			Context("with no arguments or env variables or config", func() {
				// TODO: support default values
				// It("sets default value provided via `def` value tag", func() {
				// 	runTest([]string{}, map[string]string{}, func(cfg *TestConfig) {
				// 		Expect(cfg.Foos).To(Equal([]string{"def-1", "def-2"}))
				// 	})
				// })
			})

			Context("when arguments provided", func() {
				It("sets value from argument", func() {
					runTest([]string{"--foos", "arg-1,arg-2"}, map[string]string{}, func(cfg *TestConfig) {
						Expect(cfg.Foos).To(Equal([]string{"arg-1", "arg-2"}))
					})
				})
			})

			Context("when env variables provided", func() {
				It("sets value from env variable", func() {
					runTest([]string{""}, map[string]string{"PYROSCOPE_FOOS": "env-1,env-2"}, func(cfg *TestConfig) {
						Expect(cfg.Foos).To(Equal([]string{"env-1", "env-2"}))
					})
				})
			})

			Context("when config file is provided", func() {
				It("sets value from config file", func() {
					runTest([]string{"--config", "testdata/clitest.yml"}, map[string]string{}, func(cfg *TestConfig) {
						Expect(cfg.Foos).To(Equal([]string{"config-1", "config-2"}))
					})
				})
			})
		})
	})
})
