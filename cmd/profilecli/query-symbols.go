package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"connectrpc.com/connect"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

type querySymbolsParams struct {
	*queryParams
	Symbols []string
	Output  string
}

func addQuerySymbolsParams(queryCmd commander) *querySymbolsParams {
	params := new(querySymbolsParams)
	params.queryParams = addQueryParams(queryCmd)
	queryCmd.Flag("symbol", "Exact Function.Name to search for. Repeatable for multiple symbols.").Required().StringsVar(&params.Symbols)
	queryCmd.Flag("output", "Output format, one of: table, json.").Default("table").StringVar(&params.Output)
	return params
}

func querySymbols(ctx context.Context, params *querySymbolsParams) error {
	from, to, err := params.parseFromTo()
	if err != nil {
		return err
	}
	client := params.phlareClient.queryFrontendClient()
	resp, err := client.SymbolServices(ctx, connect.NewRequest(&queryv1.SymbolServicesRequest{
		StartTime:     from.UnixMilli(),
		EndTime:       to.UnixMilli(),
		LabelSelector: params.Query,
		SymbolNames:   params.Symbols,
	}))
	if err != nil {
		return fmt.Errorf("failed to query symbols: %w", err)
	}
	logDiagnostics(params.phlareClient, resp.Header())
	return outputSymbolServices(ctx, resp.Msg, params.Symbols, from, to, params.Output)
}

func outputSymbolServices(ctx context.Context, resp *queryv1.SymbolServicesResponse, symbols []string, from, to time.Time, format string) error {
	switch format {
	case outputJSON:
		return outputSymbolServicesJSON(ctx, resp, symbols, from, to)
	case "table":
		return outputSymbolServicesTable(ctx, resp)
	default:
		return fmt.Errorf("unknown output %s", format)
	}
}

func outputSymbolServicesJSON(ctx context.Context, resp *queryv1.SymbolServicesResponse, symbols []string, from, to time.Time) error {
	type jsonService struct {
		ServiceName  string   `json:"service_name"`
		ProfileTypes []string `json:"profile_types"`
	}
	type jsonResult struct {
		SymbolName string        `json:"symbol_name"`
		Services   []jsonService `json:"services"`
	}
	type jsonOutput struct {
		Symbols  []string     `json:"symbols"`
		From     time.Time    `json:"from"`
		To       time.Time    `json:"to"`
		Complete bool         `json:"complete"`
		Results  []jsonResult `json:"results"`
	}
	out := jsonOutput{
		Symbols:  append([]string(nil), symbols...),
		From:     from,
		To:       to,
		Complete: resp.GetComplete(),
		Results:  make([]jsonResult, 0, len(resp.GetResults())),
	}
	for _, result := range resp.GetResults() {
		jr := jsonResult{SymbolName: result.GetSymbolName(), Services: make([]jsonService, 0, len(result.GetServices()))}
		for _, service := range result.GetServices() {
			jr.Services = append(jr.Services, jsonService{
				ServiceName:  service.GetServiceName(),
				ProfileTypes: append([]string(nil), service.GetProfileTypes()...),
			})
		}
		out.Results = append(out.Results, jr)
	}
	enc := json.NewEncoder(output(ctx))
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func outputSymbolServicesTable(ctx context.Context, resp *queryv1.SymbolServicesResponse) error {
	if !resp.GetComplete() {
		fmt.Fprintln(output(ctx), "WARNING: response is incomplete because some blocks did not have a symbol Bloom index")
	}
	table := newTableWriter(output(ctx))
	table.SetHeader([]string{"SYMBOL", "SERVICE_NAME", "PROFILE_TYPES"})
	results := append([]*queryv1.SymbolServicesResult(nil), resp.GetResults()...)
	sort.Slice(results, func(i, j int) bool { return results[i].GetSymbolName() < results[j].GetSymbolName() })
	for _, result := range results {
		services := append([]*queryv1.SymbolService(nil), result.GetServices()...)
		sort.Slice(services, func(i, j int) bool { return services[i].GetServiceName() < services[j].GetServiceName() })
		for _, service := range services {
			profileTypes := append([]string(nil), service.GetProfileTypes()...)
			sort.Strings(profileTypes)
			table.Append([]string{result.GetSymbolName(), service.GetServiceName(), strings.Join(profileTypes, ",")})
		}
	}
	table.Render()
	return nil
}
