package main

import (
	"context"
	"encoding/json"
	"fmt"

	"connectrpc.com/connect"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/settings/v1/settingsv1connect"
	v1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	connectapi "github.com/grafana/pyroscope/pkg/api/connect"
)

const createRuleExampleMsg = `
		Example: 
			# Create a rule that records the total CPU usage of the garbage collector function for every service in the "emea" region:
			profilecli recording-rules create '{
				"matchers": [
					"{ __profile_type__=\"process_cpu:cpu:nanoseconds:cpu:nanoseconds\", region=\"emea\"}"
				],
				"metric_name": "profiles_recorded_cpu_usage_function_total_gc_nanoseconds",
				"group_by": [
					"service_name"
				],
				"function_name": "runtime.gcBgMarkWorker"
			}'`

func (c *phlareClient) recordingRulesClient() settingsv1connect.RecordingRulesServiceClient {
	return settingsv1connect.NewRecordingRulesServiceClient(
		c.httpClient(),
		c.URL,
		append(
			connectapi.DefaultClientOptions(),
			c.protocolOption(),
		)...,
	)
}

type recordingRule struct {
	Matchers       []string        `json:"matchers,omitempty"`
	MetricName     string          `json:"metric_name,omitempty"`
	GroupBy        []string        `json:"group_by,omitempty"`
	ExternalLabels []*v1.LabelPair `json:"external_labels,omitempty"`
	FunctionName   string          `json:"function_name,omitempty"`
}

func listRecordingRules(ctx context.Context, params *recordingRulesCmdParams) error {
	client := params.phlareClient.recordingRulesClient()
	req := settingsv1.ListRecordingRulesRequest{}
	resp, err := client.ListRecordingRules(ctx, connect.NewRequest(&req))
	if err != nil {
		return err
	}
	for _, r := range resp.Msg.Rules {
		rule := recordingRule{
			Matchers:       make([]string, 0),
			MetricName:     r.MetricName,
			GroupBy:        r.GroupBy,
			ExternalLabels: r.ExternalLabels,
		}
		for _, m := range r.Matchers {
			if m != "{}" {
				rule.Matchers = append(rule.Matchers, m)
			}
		}
		if r.StacktraceFilter != nil && r.StacktraceFilter.FunctionName != nil {
			rule.FunctionName = r.StacktraceFilter.FunctionName.FunctionName
		}
		fmt.Printf("Rule with Id %s", r.Id)
		if r.Provisioned {
			fmt.Print(" (backend provisioned - read only)")
		}
		fmt.Println()

		data, _ := json.MarshalIndent(rule, "", "  ")

		fmt.Println(string(data))
		fmt.Println()
	}
	return nil
}

func createRecordingRule(ctx context.Context, rule *string, params *recordingRulesCmdParams) error {
	client := params.phlareClient.recordingRulesClient()
	var newRule recordingRule
	err := json.Unmarshal([]byte(*rule), &newRule)
	if err != nil {
		return err
	}
	req := settingsv1.UpsertRecordingRuleRequest{
		MetricName:     newRule.MetricName,
		Matchers:       newRule.Matchers,
		GroupBy:        newRule.GroupBy,
		ExternalLabels: newRule.ExternalLabels,
	}
	if newRule.FunctionName != "" {
		req.StacktraceFilter = &settingsv1.StacktraceFilter{FunctionName: &settingsv1.StacktraceFilterFunctionName{
			FunctionName: newRule.FunctionName,
			MetricType:   settingsv1.MetricType_TOTAL,
		}}
	}
	resp, err := client.UpsertRecordingRule(ctx, connect.NewRequest(&req))
	if err != nil {
		return err
	}
	fmt.Println("New recorded rule created with id:", resp.Msg.Rule.Id)
	return nil
}

func deleteRecordingRule(ctx context.Context, id *string, params *recordingRulesCmdParams) error {
	client := params.phlareClient.recordingRulesClient()
	req := settingsv1.DeleteRecordingRuleRequest{
		Id: *id,
	}
	_, err := client.DeleteRecordingRule(ctx, connect.NewRequest(&req))
	if err != nil {
		return err
	}
	fmt.Println("Deleted recording rule with id:", *id)
	return nil
}

type recordingRulesCmdParams struct {
	*phlareClient
}

func addRecordingRulesListParams(recordingRulesListCmd commander) *recordingRulesCmdParams {
	params := new(recordingRulesCmdParams)
	params.phlareClient = addPhlareClient(recordingRulesListCmd)
	return params
}
