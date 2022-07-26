package command

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"

	"github.com/google/pprof/profile"
	"github.com/spf13/cobra"
	"golang.org/x/tools/go/packages"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func newTestCmd(cfg *config.Test) *cobra.Command {
	vpr := newViper()

	var cmd *cobra.Command
	cmd = &cobra.Command{
		Use:   "test",
		Short: "profile tests",
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, _ []string) error {
			fmt.Println(cfg)
			printUsageMessage(cmd)
			return nil
		}),
	}

	cmd.AddCommand(newTestGoCmd(cfg))

	return cmd
}

func newTestGoCmd(cfg *config.Test) *cobra.Command {
	vpr := newViper()

	var cmd *cobra.Command
	cmd = &cobra.Command{
		Use:   "go [flags] <args>",
		Short: "Run go(lang) tests and generate a flamegraph",
		Long: `
'pyroscope test go' runs 'go test' under the hood for each individual package,
profiles it, then merges into a single profile file.
		`,
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, args []string) error {
			//			dir := ""
			//			if len(args) > 0 && args[0] != "" {
			//				dir = args[0]
			//			}

			// TODO(eh-am): this doesn't quite work
			pkgs, err := packages.Load(&packages.Config{}, args...)
			if err != nil {
				return err
			}
			fmt.Println("packages")
			fmt.Println(pkgs)

			tempDir, err := ioutil.TempDir("", "profiles")
			if err != nil {
				return err
			}
			defer os.RemoveAll(tempDir)

			fmt.Println("tempDir", tempDir)

			fmt.Println("cdf args")
			fmt.Println(cfg.Args)

			// Generate profiles
			// TODO(eh-am): do this concurrently
			for i, pkg := range pkgs {
				fmt.Println("Running", "go test", pkg.String())

				args := []string{"test", pkg.String(), fmt.Sprintf("-outputdir=%s", tempDir), fmt.Sprintf("-cpuprofile=%d.cpu", i)}
				args = append(args, cfg.Args...)

				fmt.Println("running with args", args)
				// TODO(eh-am): add extra arguments
				output, err := exec.Command("go", args...).Output()
				if err != nil {
					fmt.Println(string(output))

					if !cfg.IgnoreTestErrors {
						return err
					}
				}
				fmt.Println(string(output))
			}

			// Load profiles into memory

			fmt.Println("loading profiles into memory")
			var profiles []*profile.Profile
			// TODO(eh-am):
			for i := range pkgs {
				filename := path.Join(tempDir, fmt.Sprintf("%d.cpu", i))
				f, err := os.Open(filename)
				if err != nil {
					// It may be possible that the module doesn't have any tests
					// And therefore did not generate any profiles
					fmt.Println("file not found, ignoring", filename)
					continue
				}

				p, err := profile.Parse(f)
				if err != nil {
					// TODO(eh-am): it may be possible to try to parse an empty file?
					fmt.Println("failed to parse profile", err)
					//					return err
				} else {
					profiles = append(profiles, p)
				}
			}

			fmt.Println("merging")
			merged, err := profile.Merge(profiles)
			if err != nil {
				return err
			}

			out, err := os.OpenFile("merged.pb.gz", os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return err
			}

			err = merged.Write(out)
			if err != nil {
				return err
			}

			fmt.Println("Generated merged.pb.gz file")
			return nil
		}),
	}

	cli.PopulateFlagSet(cfg, cmd.Flags(), vpr)

	return cmd
}
