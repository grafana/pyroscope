package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	gprofile "github.com/google/pprof/profile"
	"github.com/grafana/dskit/runutil"
	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1/ingesterv1connect"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	"github.com/grafana/pyroscope/api/gen/proto/go/storegateway/v1/storegatewayv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/operations"
	"github.com/k0kubun/pp/v3"
	"github.com/klauspost/compress/gzip"
	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"
)

const (
	outputConsole = "console"
	outputRaw     = "raw"
	outputPprof   = "pprof="
)

func (c *phlareClient) queryClient() querierv1connect.QuerierServiceClient {
	return querierv1connect.NewQuerierServiceClient(
		c.httpClient(),
		c.URL,
	)
}

func (c *phlareClient) storeGatewayClient() storegatewayv1connect.StoreGatewayServiceClient {
	return storegatewayv1connect.NewStoreGatewayServiceClient(
		c.httpClient(),
		c.URL,
	)
}

func (c *phlareClient) ingesterClient() ingesterv1connect.IngesterServiceClient {
	return ingesterv1connect.NewIngesterServiceClient(
		c.httpClient(),
		c.URL,
	)
}

type queryParams struct {
	*phlareClient
	From  string
	To    string
	Query string
}

func (p *queryParams) parseFromTo() (from time.Time, to time.Time, err error) {
	from, err = operations.ParseTime(p.From)
	if err != nil {
		return time.Time{}, time.Time{}, errors.Wrap(err, "failed to parse from")
	}
	to, err = operations.ParseTime(p.To)
	if err != nil {
		return time.Time{}, time.Time{}, errors.Wrap(err, "failed to parse to")
	}

	if to.Before(from) {
		return time.Time{}, time.Time{}, errors.Wrap(err, "from cannot be after")
	}

	return from, to, nil
}

func addQueryParams(queryCmd commander) *queryParams {
	params := new(queryParams)
	params.phlareClient = addPhlareClient(queryCmd)

	queryCmd.Flag("from", "Beginning of the query.").Default("now-1h").StringVar(&params.From)
	queryCmd.Flag("to", "End of the query.").Default("now").StringVar(&params.To)
	queryCmd.Flag("query", "Label selector to query.").Default("{}").StringVar(&params.Query)
	return params
}

type queryMergeParams struct {
	*queryParams
	ProfileType string
}

func addQueryMergeParams(queryCmd commander) *queryMergeParams {
	params := new(queryMergeParams)
	params.queryParams = addQueryParams(queryCmd)
	queryCmd.Flag("profile-type", "Profile type to query.").Default("process_cpu:cpu:nanoseconds:cpu:nanoseconds").StringVar(&params.ProfileType)
	return params
}

func queryMerge(ctx context.Context, params *queryMergeParams, outputFlag string) (err error) {
	from, to, err := params.parseFromTo()
	if err != nil {
		return err
	}

	level.Info(logger).Log("msg", "query aggregated profile from profile store", "url", params.URL, "from", from, "to", to, "query", params.Query, "type", params.ProfileType)

	qc := params.phlareClient.queryClient()

	resp, err := qc.SelectMergeProfile(ctx, connect.NewRequest(&querierv1.SelectMergeProfileRequest{
		ProfileTypeID: params.ProfileType,
		Start:         from.UnixMilli(),
		End:           to.UnixMilli(),
		LabelSelector: params.Query,
	}))

	if err != nil {
		return errors.Wrap(err, "failed to query")
	}

	mypp := pp.New()
	mypp.SetColoringEnabled(isatty.IsTerminal(os.Stdout.Fd()))
	mypp.SetExportedOnly(true)

	if outputFlag == outputConsole {
		buf, err := resp.Msg.MarshalVT()
		if err != nil {
			return errors.Wrap(err, "failed to marshal protobuf")
		}

		p, err := gprofile.Parse(bytes.NewReader(buf))
		if err != nil {
			return errors.Wrap(err, "failed to parse profile")
		}

		fmt.Fprintln(output(ctx), p.String())
		return nil

	}

	if outputFlag == outputRaw {
		mypp.Print(resp.Msg)
		return nil
	}

	if strings.HasPrefix(outputFlag, outputPprof) {
		filePath := strings.TrimPrefix(outputFlag, outputPprof)
		if filePath == "" {
			return errors.New("no file path specified after pprof=")
		}
		buf, err := resp.Msg.MarshalVT()
		if err != nil {
			return errors.Wrap(err, "failed to marshal protobuf")
		}

		// open new file, fail when the file already exists
		f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return errors.Wrap(err, "failed to create pprof file")
		}
		defer runutil.CloseWithErrCapture(&err, f, "failed to close pprof file")

		gzipWriter := gzip.NewWriter(f)
		defer runutil.CloseWithErrCapture(&err, gzipWriter, "failed to close pprof gzip writer")

		if _, err := io.Copy(gzipWriter, bytes.NewReader(buf)); err != nil {
			return errors.Wrap(err, "failed to write pprof")
		}

		return nil
	}

	return errors.Errorf("unknown output %s", outputFlag)
}

type querySeriesParams struct {
	*queryParams
	LabelNames []string
	APIType    string
}

func addQuerySeriesParams(queryCmd commander) *querySeriesParams {
	params := new(querySeriesParams)
	params.queryParams = addQueryParams(queryCmd)
	queryCmd.Flag("label-names", "Filter returned labels to the supplied label names. Without any filter all labels are returned.").StringsVar(&params.LabelNames)
	queryCmd.Flag("api-type", "Which API type to query (querier, ingester or store-gateway).").Default("querier").StringVar(&params.APIType)
	return params
}

func querySeries(ctx context.Context, params *querySeriesParams) (err error) {
	from, to, err := params.parseFromTo()
	if err != nil {
		return err
	}

	level.Info(logger).Log("msg", fmt.Sprintf("query series from %s", params.APIType), "url", params.URL, "from", from, "to", to, "labelNames", fmt.Sprintf("%q", params.LabelNames))

	var result []*typesv1.Labels
	switch params.APIType {
	case "querier":
		qc := params.phlareClient.queryClient()
		resp, err := qc.Series(ctx, connect.NewRequest(&querierv1.SeriesRequest{
			Start:      from.UnixMilli(),
			End:        to.UnixMilli(),
			Matchers:   []string{params.Query},
			LabelNames: params.LabelNames,
		}))
		if err != nil {
			return errors.Wrap(err, "failed to query")
		}
		result = resp.Msg.LabelsSet
	case "ingester":
		ic := params.phlareClient.ingesterClient()
		resp, err := ic.Series(ctx, connect.NewRequest(&ingestv1.SeriesRequest{
			Start:      from.UnixMilli(),
			End:        to.UnixMilli(),
			Matchers:   []string{params.Query},
			LabelNames: params.LabelNames,
		}))
		if err != nil {
			return errors.Wrap(err, "failed to query")
		}
		result = resp.Msg.LabelsSet
	case "store-gateway":
		sc := params.phlareClient.storeGatewayClient()
		resp, err := sc.Series(ctx, connect.NewRequest(&ingestv1.SeriesRequest{
			Start:      from.UnixMilli(),
			End:        to.UnixMilli(),
			Matchers:   []string{params.Query},
			LabelNames: params.LabelNames,
		}))
		if err != nil {
			return errors.Wrap(err, "failed to query")
		}
		result = resp.Msg.LabelsSet
	default:
		return errors.Errorf("unknown api type %s", params.APIType)
	}

	enc := json.NewEncoder(os.Stdout)
	m := make(map[string]interface{})
	for _, s := range result {
		for k := range m {
			delete(m, k)
		}
		for _, l := range s.Labels {
			m[l.Name] = l.Value
		}
		if err := enc.Encode(m); err != nil {
			return err
		}
	}

	return nil

}
