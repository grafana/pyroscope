package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log/level"
	tempopb "github.com/simonswine/tempopb"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"golang.org/x/sync/errgroup"
)

var userAgent string = "profilecli/unknown"

func init() {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	userAgent = fmt.Sprintf("profilecli/%s", bi.Main.Version)
}

type queryTracerParams struct {
	File     []string
	Delay    time.Duration
	Interval time.Duration
	TempoURL string
}

func addQueryTracerParams(cmd commander) *queryTracerParams {
	params := &queryTracerParams{}
	cmd.Flag("interval", "Interval for how often to query Tempo").Default("5m").DurationVar(&params.Interval)
	cmd.Flag("delay", "Allow for a delay between ingestion and availability in the query path.").Default("0s").DurationVar(&params.Delay)
	cmd.Flag("tempo-url", "Tempo URL").Default("http://localhost:3100/").StringVar(&params.TempoURL)
	cmd.Flag("filter-sensitive", "Filters potentially sensitive fields from the output").Default("false").BoolVar(&filterSensitive)
	cmd.Flag("file", "File to read traces in protobuf directly from").StringsVar(&params.File)
	return params
}

var filterSensitive bool

type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

type SensitiveString string

func (s SensitiveString) MarshalJSON() ([]byte, error) {
	if !filterSensitive {
		return json.Marshal(string(s))
	}
	if len(s) == 0 {
		return json.Marshal("")
	}
	return json.Marshal("***")
}

type TraceResult struct {
	Namespace    string    `json:"namespace,omitempty"`
	Cluster      string    `json:"cluster,omitempty"`
	TraceID      string    `json:"trace_id,omitempty"`
	StartTime    time.Time `json:"request_start_time,omitempty"`
	EndTime      time.Time `json:"request_end_time,omitempty"`
	Duration     Duration  `json:"duration,omitempty"`
	Status       int       `json:"status,omitempty"`
	ResponseSize int       `json:"response_size,omitempty"`
	Query        struct {
		Method      string          `json:"method,omitempty"`
		StartTime   time.Time       `json:"start_time,omitempty"`
		EndTime     time.Time       `json:"end_time,omitempty"`
		Duration    Duration        `json:"duration,omitempty"`
		Selector    SensitiveString `json:"selector,omitempty"`
		ProfileType string          `json:"profile_type,omitempty"`
	} `json:"query,omitempty"`
	Stats struct {
		Profiles        int `json:"profiles,omitempty"`
		ProfilesFetched int `json:"profiles_fetched,omitempty"`
	} `json:"stats,omitempty"`
	Grafana struct {
		TenantID   SensitiveString `json:"tenant_id,omitempty"`
		User       SensitiveString `json:"user,omitempty"`
		Datasource SensitiveString `json:"datasource,omitempty"`
		Plugin     string          `json:"plugin,omitempty"`
	} `json:"grafana,omitempty"`
}

func (r *TraceResult) LogFields(fields []interface{}) []interface{} {
	d, err := json.Marshal(r)
	if err != nil {
		panic(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(d, &m); err != nil {
		panic(err)
	}
	return mapToLogFields(m, fields, "")
}

func mapToLogFields(m map[string]interface{}, fields []interface{}, prefix string) []interface{} {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch m[k].(type) {
		case map[string]interface{}:
			fields = mapToLogFields(m[k].(map[string]interface{}), fields, prefix+k+"_")
		default:
			fields = append(fields, prefix+k, m[k])
		}
	}
	return fields
}

func httpGetOrPost(spanName string) bool {
	return strings.HasPrefix(spanName, "HTTP GET - ") || strings.HasPrefix(spanName, "HTTP POST - ")
}

func parseTrace(ctx context.Context, traceID string, data []byte) (*TraceResult, error) {
	var traceResult TraceResult
	traceResult.TraceID = traceID

	// Read the existing address book.
	trace := &tempopb.Trace{}

	if err := trace.UnmarshalVT(data); err != nil {
		return nil, fmt.Errorf("failed to parse trace: %w", err)
	}

	setK8SMeta := func(attrs []*commonv1.KeyValue) {
		for _, attr := range attrs {
			if len(traceResult.Cluster) == 0 && attr.Key == "cluster" {
				traceResult.Cluster = attr.Value.GetStringValue()
			}
			if len(traceResult.Namespace) == 0 && attr.Key == "namespace" {
				traceResult.Namespace = attr.Value.GetStringValue()
			}
		}
	}

	var spansByServiceName = map[string][]*tracev1.Span{
		"grafana":                  nil,
		"pyroscope-query-frontend": nil,
		"pyroscope-gateway":        nil,
		"pyroscope-ingester":       nil,
		"pyroscope-store-gateway":  nil,
	}
	for _, batch := range trace.Batches {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		for _, attr := range batch.Resource.Attributes {
			if attr.Key == "service.name" {
				// check by service.name map
				serviceName := attr.Value.GetStringValue()
				_, ok := spansByServiceName[attr.Value.GetStringValue()]
				if ok {
					// add spans to the map
					for _, span := range batch.ScopeSpans {
						spansByServiceName[serviceName] = append(spansByServiceName[serviceName], span.Spans...)
					}
				}

				if serviceName == "pyroscope-gateway" {
					setK8SMeta(batch.Resource.Attributes)
				}
			}
		}
	}

	// find gateway span for duration, status code and response size
	firstFound := false
	for _, span := range spansByServiceName["pyroscope-gateway"] {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !firstFound && httpGetOrPost(span.Name) {
			//traceResult.TraceID = span.TraceId
			traceResult.StartTime = time.Unix(0, int64(span.StartTimeUnixNano))
			traceResult.EndTime = time.Unix(0, int64(span.EndTimeUnixNano))
			traceResult.Duration = Duration(traceResult.EndTime.Sub(traceResult.StartTime))

			for _, attr := range span.Attributes {
				if attr.Key == "http.status_code" {
					traceResult.Status = int(attr.Value.GetIntValue())
				}
				if attr.Key == "http.response_size" {
					traceResult.ResponseSize = int(attr.Value.GetIntValue())
				}
			}
			firstFound = true
			continue
		}
		if span.Name == "Auth/Authenticate" {
			for _, attr := range span.Attributes {
				if attr.Key == "instance_id" {
					traceResult.Grafana.TenantID = SensitiveString(attr.Value.GetStringValue())
				}
			}
		}
	}

	// get the query parameters
	firstFound = false
	timeFormat := "2006-01-02 15:04:05.999 -0700 MST"
	for _, span := range spansByServiceName["pyroscope-query-frontend"] {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !firstFound && httpGetOrPost(span.Name) {
			for _, attr := range span.Attributes {
				if attr.Key == "http.url" {
					method := strings.SplitN(attr.Value.GetStringValue(), "?", 2)
					traceResult.Query.Method = method[0]
				}
				if attr.Key == "start" {
					traceResult.Query.StartTime, _ = time.Parse(timeFormat, attr.Value.GetStringValue())
				}
				if attr.Key == "end" {
					traceResult.Query.EndTime, _ = time.Parse(timeFormat, attr.Value.GetStringValue())
				}
				if attr.Key == "selector" {
					traceResult.Query.Selector = SensitiveString(attr.Value.GetStringValue())
				}
				if attr.Key == "profile_type" {
					traceResult.Query.ProfileType = attr.Value.GetStringValue()
				}
			}
			firstFound = true
		}
	}
	if !traceResult.Query.StartTime.IsZero() && !traceResult.Query.EndTime.IsZero() {
		traceResult.Query.Duration = Duration(traceResult.Query.EndTime.Sub(traceResult.Query.StartTime))
	}

	// get the user name from grafana
	for _, span := range spansByServiceName["grafana"] {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if span.Name == "PluginClient.queryData" || span.Name == "PluginClient.callResource" {
			for _, attr := range span.Attributes {
				if attr.Key == "user" {
					traceResult.Grafana.User = SensitiveString(attr.Value.GetStringValue())
				}
				if attr.Key == "plugin_id" {
					traceResult.Grafana.Plugin = attr.Value.GetStringValue()
				}
				if attr.Key == "datasource_name" {
					traceResult.Grafana.Datasource = SensitiveString(attr.Value.GetStringValue())
				}
			}
			break
		}
	}

	storeSpans := append(spansByServiceName["pyroscope-ingester"], spansByServiceName["pyroscope-store-gateway"]...)
	found := false
	for _, span := range storeSpans {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if span.Name == "RepeatedRowColumnIterator" {
			found = false
			for _, attr := range span.Attributes {
				if attr.Key == "column" && attr.Value.GetStringValue() == "Samples.list.element.Value" {
					found = true
					break
				}
			}
			if !found {
				continue
			}
			for _, ev := range span.Events {
				for _, attr := range ev.Attributes {
					if attr.Key == "rows_fetched" {
						traceResult.Stats.ProfilesFetched += int(attr.Value.GetIntValue())
					}
					if attr.Key == "rows_read" {
						traceResult.Stats.Profiles += int(attr.Value.GetIntValue())
					}
				}
			}
		}
	}

	return &traceResult, nil
}

func queryTracer(ctx context.Context, params *queryTracerParams) (err error) {
	if len(params.File) > 0 {
		fields := make([]interface{}, 0, 64)

		for _, f := range params.File {
			data, err := os.ReadFile(f)
			if err != nil {
				return err
			}
			result, err := parseTrace(ctx, "", data)
			if err != nil {
				return err
			}
			b, _ := json.Marshal(result)
			mapD := map[string]interface{}{}
			if err := json.Unmarshal(b, &mapD); err != nil {
				return err
			}

			fields = fields[:4]
			fields[0] = "msg"
			fields[1] = "query successfully traced"
			fields[2] = "file"
			fields[3] = f
			fields = result.LogFields(fields)
			level.Info(logger).Log(fields...)
		}
		return nil
	}

	client := http.DefaultClient
	end := time.Now().Add(-params.Delay)
	start := end.Add(-params.Interval)

	traceQL := `{
    ( span.http.url= "/querier.v1.QuerierService/SelectMergeStacktraces" ||
      span.http.url= "/querier.v1.QuerierService/SelectMergeProfile" || 
      name= "HTTP GET - pyroscope_render_diff" ||
      name= "HTTP GET - pyroscope_render"
    ) && span.profile_type!="deadmans_switch:made_up:profilos:made_up:profilos"
 }`

	tempoURL, err := url.Parse(params.TempoURL)
	if err != nil {
		return err
	}
	originalPath := tempoURL.Path

	tempoURL.Path = filepath.Join(originalPath, "/api/search")

	level.Info(logger).Log("msg", "search for matching traces", "start", start, "end", end)

	tempoParams := url.Values{}
	tempoParams.Add("start", strconv.FormatInt(start.Unix(), 10))
	tempoParams.Add("end", strconv.FormatInt(end.Unix(), 10))
	tempoParams.Add("limit", "10000")
	tempoParams.Add("q", traceQL)
	tempoURL.RawQuery = tempoParams.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", tempoURL.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("Tempo returned status code [%d]: %s ", resp.StatusCode, body)
	}

	var respBody struct {
		Traces []struct {
			TraceID           string `json:"traceID"`
			RootServiceName   string `json:"rootServiceName"`
			RootTraceName     string `json:"rootTraceName"`
			StartTimeUnixNano string `json:"startTimeUnixNano"`
			DurationMs        int64  `json:"durationMs"`
		} `json:"traces"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return err
	}

	level.Info(logger).Log("msg", fmt.Sprintf("found %d matching traces", len(respBody.Traces)))

	// now fetch traces in parallel
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(runtime.GOMAXPROCS(-1))

	results := make(chan *TraceResult)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		fields := make([]interface{}, 0, 64)
		for r := range results {
			fields = fields[:2]
			fields[0] = "msg"
			fields[1] = "query successfully traced"
			fields = r.LogFields(fields)
			level.Info(logger).Log(fields...)
		}
	}()
	defer wg.Wait()
	defer close(results)

	for _, t := range respBody.Traces {
		startTimeUnixNanos, err := strconv.ParseInt(t.StartTimeUnixNano, 10, 64)
		if err != nil {
			return err
		}

		start = time.Unix(0, startTimeUnixNanos)
		end = start.Add(time.Duration(t.DurationMs) * time.Millisecond)

		tempoURL.Path = filepath.Join(originalPath, "/api/traces", t.TraceID)
		tempoParams := url.Values{}
		tempoParams.Add("start", strconv.FormatInt(start.Unix(), 10))
		tempoParams.Add("end", strconv.FormatInt(end.Unix()+1, 10))
		tempoParams.Add("limit", "10000")
		tempoParams.Add("q", traceQL)
		tempoURL.RawQuery = tempoParams.Encode()

		var (
			url     = tempoURL.String()
			traceID = t.TraceID
		)
		g.Go(func() error {
			req, err := http.NewRequestWithContext(gctx, "GET", url, nil)
			if err != nil {
				return err
			}
			req.Header.Set("Accept", "application/protobuf")
			req.Header.Set("User-Agent", userAgent)

			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
				return fmt.Errorf("Tempo returned status code [%d]: %s ", resp.StatusCode, body)
			}

			data, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}

			result, err := parseTrace(gctx, traceID, data)
			if err != nil {
				level.Error(logger).Log("msg", "failed to parse trace", "trace_id", traceID, "err", err)
			}

			results <- result

			return nil
		})
	}

	return g.Wait()
}
