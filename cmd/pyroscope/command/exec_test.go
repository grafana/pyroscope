package command

import (
	"errors"
	"io"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

type cmdArgsTestCase struct {
	description  string
	inputArgs    []string
	expectedArgs []string
	err          error
}

var appArgs = []string{"app", "-f", "1", "-b", "arg"}

func execArgs(args ...string) []string { return append(args, appArgs...) }

func TestExecCommand(t *testing.T) {
	RegisterFailHandler(Fail)

	testCases := []cmdArgsTestCase{
		{
			description: "no_arguments",
			inputArgs:   []string{},
		},
		{
			description: "help_flag",
			inputArgs:   []string{"--help"},
		},
		{
			description:  "delimiter",
			inputArgs:    []string{cli.OptionsEnd},
			expectedArgs: []string{},
		},
		{
			description: "unknown_flag",
			inputArgs:   []string{"--non-existing_flag"},
			err:         errors.New("unknown flag: --non-existing_flag"),
		},
		{
			description:  "exec_no_arguments",
			inputArgs:    appArgs,
			expectedArgs: appArgs,
		},
		{
			description:  "exec_separated",
			inputArgs:    execArgs("-spy-name", "debugspy", cli.OptionsEnd),
			expectedArgs: appArgs,
		},
		{
			description:  "exec_flags_mixed",
			expectedArgs: appArgs,
			inputArgs: execArgs(
				"--spy-name=debugspy",
				"--application-name", "app",
				"--no-logging",
				"-server-address=http://localhost:4040",
				"-log-level", "debug",
				"-no-logging",
			),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			g := NewGomegaWithT(t)
			cfg := new(config.Exec)
			vpr := newViper()
			var cmdArgs []string
			cmd := &cobra.Command{
				SilenceErrors:      true,
				DisableFlagParsing: true,
				RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, args []string) error {
					cmdArgs = args
					return nil
				}),
			}

			cmd.SetUsageFunc(printUsageMessage)
			cmd.SetHelpFunc(printHelpMessage)
			cmd.SetArgs(testCase.inputArgs)
			cmd.SetOut(io.Discard)
			cli.PopulateFlagSet(cfg, cmd.Flags(), vpr)

			err := cmd.Execute()
			if testCase.err == nil {
				g.Expect(err).To(BeNil())
				g.Expect(cmdArgs).To(Equal(testCase.expectedArgs))
				return
			}

			g.Expect(err).To(Equal(testCase.err))
		})
	}
}
