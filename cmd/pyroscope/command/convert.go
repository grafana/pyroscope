package command

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
)

func newConvertCmd(cfg *config.Convert) *cobra.Command {
	vpr := newViper()
	convertCmd := &cobra.Command{
		Use:    "convert [flags]",
		Short:  "Convert between different profiling formats",
		Hidden: true,

		DisableFlagParsing: true,
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, _ []string) error {
			return parse(os.Stdin, cfg.Format)
		}),
	}

	cli.PopulateFlagSet(cfg, convertCmd.Flags(), vpr)
	return convertCmd
}

func parse(input io.Reader, format string) error {
	parser := convert.ParseGroups
	switch format {
	case "tree":
		t := tree.New()
		parser(input, func(name []byte, val int) {
			t.Insert(name, uint64(val))
		})
		return t.SerializeNoDict(4096, os.Stdout)
	case "trie":
		t := transporttrie.New()
		parser(input, func(name []byte, val int) {
			t.Insert(name, uint64(val), true)
		})
		return t.Serialize(os.Stdout)
	default:
		return fmt.Errorf("unknown format: %s", format)
	}
}
