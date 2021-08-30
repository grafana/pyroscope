package command

import (
	"fmt"
	"io"
	"os"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/spf13/cobra"

	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
)

func newConvertCmd(cfg *config.Convert) *cobra.Command {
	vpr := newViper()
	convertCmd := &cobra.Command{
		Use:   "convert [flags] <input-file>",
		Short: "Convert between different profiling formats",
		RunE: createCmdRunFn(cfg, vpr, false, func(cmd *cobra.Command, args []string, logger config.LoggerFunc) error {
			var input io.Reader
			if len(args) == 0 {
				input = os.Stdin
			} else {
				logger("not implemented yet")
				return nil
			}

			parser := convert.ParseGroups
			switch cfg.Format {
			case "tree":
				t := tree.New()
				parser(input, func(name []byte, val int) {
					t.Insert(name, uint64(val))
				})

				t.SerializeNoDict(4096, os.Stdout)
			case "trie":
				t := transporttrie.New()
				parser(input, func(name []byte, val int) {
					t.Insert(name, uint64(val), true)
				})

				t.Serialize(os.Stdout)
			default:
				logger(fmt.Sprintf("unknown format: %s", cfg.Format))
			}

			return nil
		}),
		Hidden: true,
	}

	cli.PopulateFlagSet(cfg, convertCmd.Flags(), vpr)
	return convertCmd
}
