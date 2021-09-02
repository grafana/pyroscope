package command

import (
	"fmt"
	"time"

	"github.com/pyroscope-io/pyroscope/benchmark/config"
	"github.com/pyroscope-io/pyroscope/benchmark/promquery"
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/spf13/cobra"
)

func newPromQuery(cfg *config.PromQuery) *cobra.Command {
	vpr := newViper()
	promQuery := &cobra.Command{
		// TODO(eh-am): call it 'promquery instant' or something
		Use:   "promquery [flags]",
		Short: "queries prometheus",
		Args: func(cmd *cobra.Command, args []string) error {
			//			if len(args) != 2 {
			//				return errors.New("requires 2 arguments 'from' and 'end'")
			//			}
			return nil
		},
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, args []string) error {
			query := args[0]
			t := time.Now()

			pq := promquery.New(cfg)

			err, first, second := pq.Instant(query, t)
			if err != nil {
				return err
			}

			fmt.Println("first", first)
			fmt.Println("second", second)
			//
			return nil
		}),
	}

	cli.PopulateFlagSet(cfg, promQuery.Flags(), vpr)
	return promQuery
}
