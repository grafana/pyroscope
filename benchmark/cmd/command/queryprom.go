package command

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/pyroscope-io/pyroscope/benchmark/config"
	"github.com/pyroscope-io/pyroscope/benchmark/promquery"
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/spf13/cobra"
)

func newPromQuery(cfg *config.PromQuery) *cobra.Command {
	vpr := newViper()
	promQuery := &cobra.Command{
		Use:   "promquery [flags]",
		Short: "queries prometheus",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return errors.New("requires 2 arguments 'from' and 'end'")
			}
			return nil
		},
		RunE: createCmdRunFn(cfg, vpr, func(_ *cobra.Command, args []string) error {
			start, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return err
			}
			end, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return err
			}

			err, first, second := promquery.QueryRange(time.Unix(start, 0), time.Unix(start, end))
			if err != nil {
				return err
			}

			fmt.Println("first", first)
			fmt.Println("second", second)

			return nil
		}),
	}

	cli.PopulateFlagSet(cfg, promQuery.Flags(), vpr)
	return promQuery
}
