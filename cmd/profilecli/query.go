package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log/level"
	gprofile "github.com/google/pprof/profile"
	"github.com/grafana/dskit/runutil"
	"github.com/k0kubun/pp/v3"
	"github.com/klauspost/compress/gzip"
	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"

	querierv1 "github.com/grafana/phlare/api/gen/proto/go/querier/v1"
	"github.com/grafana/phlare/api/gen/proto/go/querier/v1/querierv1connect"
)

const (
	outputConsole = "console"
	outputRaw     = "raw"
	outputPprof   = "pprof="
)

var userAgentHeader = fmt.Sprintf("phlare/%s", version.Version)

func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return t, nil
	}

	// try if it is a relative time
	d, rerr := parseRelativeTime(s)
	if rerr == nil {
		return time.Now().Add(-d), nil
	}

	// if not return first error
	return time.Time{}, err

}

func parseRelativeTime(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "now" {
		return 0, nil
	}
	s = strings.TrimPrefix(s, "now-")

	d, err := model.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	return time.Duration(d), nil
}

type phlareClient struct {
	TenantID string
	URL      string
	client   *http.Client
}

type authRoundTripper struct {
	tenantID string
	next     http.RoundTripper
}

func (a *authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if a.tenantID != "" {
		req.Header.Set("X-Scope-OrgID", a.tenantID)
	}
	req.Header.Set("User-Agent", userAgentHeader)
	return a.next.RoundTrip(req)
}

func (c *phlareClient) httpClient() *http.Client {
	if c.client == nil {
		c.client = &http.Client{Transport: &authRoundTripper{
			tenantID: c.TenantID,
			next:     http.DefaultTransport,
		}}
	}
	return c.client
}

func (c *phlareClient) queryClient() querierv1connect.QuerierServiceClient {
	return querierv1connect.NewQuerierServiceClient(
		c.httpClient(),
		c.URL,
	)
}

type queryParams struct {
	*phlareClient
	From        string
	To          string
	ProfileType string
	Query       string
}

func (p *queryParams) parseFromTo() (from time.Time, to time.Time, err error) {
	from, err = parseTime(p.From)
	if err != nil {
		return time.Time{}, time.Time{}, errors.Wrap(err, "failed to parse from")
	}
	to, err = parseTime(p.To)
	if err != nil {
		return time.Time{}, time.Time{}, errors.Wrap(err, "failed to parse to")
	}

	if to.Before(from) {
		return time.Time{}, time.Time{}, errors.Wrap(err, "from cannot be after")
	}

	return from, to, nil
}

type flagger interface {
	Flag(name, help string) *kingpin.FlagClause
}

func addPhlareClient(queryCmd flagger) *phlareClient {
	client := &phlareClient{}

	queryCmd.Flag("url", "URL of the profile store.").Default("http://localhost:4100").StringVar(&client.URL)
	queryCmd.Flag("tenant-id", "The tenant ID to be used for the X-Scope-OrgID header.").Default("").StringVar(&client.TenantID)
	return client
}

func addQueryParams(queryCmd flagger) *queryParams {
	var (
		params = &queryParams{}
	)
	params.phlareClient = addPhlareClient(queryCmd)

	queryCmd.Flag("from", "Beginning of the query.").Default("now-1h").StringVar(&params.From)
	queryCmd.Flag("to", "End of the query.").Default("now").StringVar(&params.To)
	queryCmd.Flag("profile-type", "Profile type to query.").Default("process_cpu:cpu:nanoseconds:cpu:nanoseconds").StringVar(&params.ProfileType)
	queryCmd.Flag("query", "Label selector to query.").Default("{}").StringVar(&params.Query)
	return params
}

func queryMerge(ctx context.Context, params *queryParams, outputFlag string) (err error) {
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
