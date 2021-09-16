package command

import (
	"context"
	"math"
	"math/rand"
	"net/http"
	"strconv"

	"github.com/pyroscope-io/pyroscope/benchmark/internal/config"
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/rakyll/hey/requester"
	"github.com/spf13/cobra"
)

func newHotRod(cfg *config.Hotrod) *cobra.Command {
	vpr := newViper()
	cmd := &cobra.Command{
		Use:   "hotrod [flags]",
		Short: "generates load against the hotrod app",
		Args: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, args []string) error {
			randomGen := rand.New(rand.NewSource(int64(cfg.RandSeed)))
			ctx := context.Background()

			req, _ := genRequest(randomGen, ctx, cfg.HotrodAddress)

			w := &requester.Work{
				// effectively infinite
				N:   math.MaxInt32,
				C:   cfg.Workers,
				QPS: cfg.QPS,
				// it seems we need to generate a basic request
				// otherwise it panics
				// https://github.com/rakyll/hey/blob/master/requester/requester.go#L241
				Request: req,
				RequestFunc: func() *http.Request {
					// TODO do something with the error
					req, _ := genRequest(randomGen, ctx, cfg.HotrodAddress)
					return req
				},
			}
			_ = w
			w.Init()

			w.Run()
			return nil
		}),
	}

	cli.PopulateFlagSet(cfg, cmd.Flags(), vpr)
	return cmd
}

func genRequest(randomGen *rand.Rand, ctx context.Context, address string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address+"/dispatch", nil)
	if err != nil {
		return nil, err
	}

	customerId := genCustomerId(randomGen)
	q := req.URL.Query()
	q.Add("customer", customerId)
	req.URL.RawQuery = q.Encode()

	return req, nil
}

func genCustomerId(randomGen *rand.Rand) string {
	min := 0
	max := 3
	r := randomGen.Intn((max - min + 1) + min)

	switch r {
	case 0:
		return "123"
	case 1:
		return "392"
	case 2:
		return "731"
	case 3:
		return "567"
	default:
		panic("invalid random number not between 0 and 3: " + strconv.Itoa(r))
	}
}
