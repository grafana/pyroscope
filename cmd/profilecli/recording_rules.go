package main

import (
	"context"
	"fmt"
	"os"

	"connectrpc.com/connect"
	"gopkg.in/yaml.v3"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/settings/v1/settingsv1connect"
	v1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	connectapi "github.com/grafana/pyroscope/pkg/api/connect"
)

const createRuleExampleMsg = `
		Example:
			# Create a rule that records the total CPU usage of the garbage collector function for every service in the "emea" region:
			profilecli recording-rules create -f rule.yaml

			# rule.yaml:
			matchers:
			  - '{ __profile_type__="process_cpu:cpu:nanoseconds:cpu:nanoseconds", region="emea"}'
			metric_name: profiles_recorded_cpu_usage_function_total_gc_nanoseconds
			group_by:
			  - service_name
			function_name: runtime.gcBgMarkWorker`

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
	Matchers       []string        `yaml:"matchers,omitempty" json:"matchers,omitempty"`
	MetricName     string          `yaml:"metric_name,omitempty" json:"metric_name,omitempty"`
	GroupBy        []string        `yaml:"group_by,omitempty" json:"group_by,omitempty"`
	ExternalLabels []*v1.LabelPair `yaml:"external_labels,omitempty" json:"external_labels,omitempty"`
	FunctionName   string          `yaml:"function_name,omitempty" json:"function_name,omitempty"`
}

// convertToRecordingRule converts a protobuf RecordingRule to the local recordingRule struct
func convertToRecordingRule(r *settingsv1.RecordingRule) recordingRule {
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
	return rule
}

// formatAndPrintRecordingRule converts a protobuf RecordingRule to YAML and prints it
func formatAndPrintRecordingRule(r *settingsv1.RecordingRule) error {
	rule := convertToRecordingRule(r)

	fmt.Printf("Rule with Id %s", r.Id)
	if r.Provisioned {
		fmt.Print(" (backend provisioned - read only)")
	}
	fmt.Println()

	data, err := yaml.Marshal(rule)
	if err != nil {
		return fmt.Errorf("failed to marshal rule to YAML: %w", err)
	}

	fmt.Println(string(data))
	return nil
}

func listRecordingRules(ctx context.Context, params *recordingRulesCmdParams) error {
	client := params.recordingRulesClient()
	req := settingsv1.ListRecordingRulesRequest{}
	resp, err := client.ListRecordingRules(ctx, connect.NewRequest(&req))
	if err != nil {
		return err
	}
	for _, r := range resp.Msg.Rules {
		if err := formatAndPrintRecordingRule(r); err != nil {
			return err
		}
	}
	return nil
}

func getRecordingRule(ctx context.Context, id *string, outputFile *string, params *recordingRulesCmdParams) error {
	client := params.recordingRulesClient()
	req := settingsv1.GetRecordingRuleRequest{
		Id: *id,
	}
	resp, err := client.GetRecordingRule(ctx, connect.NewRequest(&req))
	if err != nil {
		return err
	}

	r := resp.Msg.Rule

	// If output file is specified, write to file
	if outputFile != nil && *outputFile != "" {
		rule := convertToRecordingRule(r)
		data, err := yaml.Marshal(rule)
		if err != nil {
			return fmt.Errorf("failed to marshal rule to YAML: %w", err)
		}

		err = os.WriteFile(*outputFile, data, 0644)
		if err != nil {
			return fmt.Errorf("failed to write to file: %w", err)
		}

		fmt.Printf("Rule with Id %s written to %s\n", r.Id, *outputFile)
		return nil
	}

	// Otherwise, print to stdout
	return formatAndPrintRecordingRule(r)
}

func createRecordingRule(ctx context.Context, filePath *string, params *recordingRulesCmdParams) error {
	client := params.recordingRulesClient()

	// Read the rule from file
	data, err := os.ReadFile(*filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	var newRule recordingRule
	err = yaml.Unmarshal(data, &newRule)
	if err != nil {
		return fmt.Errorf("failed to parse file: %w", err)
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
	client := params.recordingRulesClient()
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
