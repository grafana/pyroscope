package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/metastorev1connect"
	connectapi "github.com/grafana/pyroscope/pkg/api/connect"
)

func (c *phlareClient) metadataOperatorClient() metastorev1connect.OperatorServiceClient {
	return metastorev1connect.NewOperatorServiceClient(
		c.httpClient(),
		c.URL,
		append(
			connectapi.DefaultClientOptions(),
			c.protocolOption(),
		)...,
	)
}

type raftInfoParams struct {
	*phlareClient

	HumanFormat bool
}

func addRaftInfoParams(cmd commander) *raftInfoParams {
	params := &raftInfoParams{}
	params.phlareClient = addPhlareClient(cmd)

	cmd.Flag("human", "Human readable output").Short('H').BoolVar(&params.HumanFormat)

	return params
}

func raftInfo(ctx context.Context, params *raftInfoParams) error {
	client := params.phlareClient.metadataOperatorClient()

	res, err := client.Info(ctx, connect.NewRequest(&metastorev1.InfoRequest{}))
	if err != nil {
		return err
	}

	var s string
	switch {
	case params.HumanFormat:
		s = formatHumanRaftInfo(res.Msg)
	default:
		s, err = formatJSONRaftInfo(res.Msg)
		if err != nil {
			return err
		}
	}

	fmt.Println(s)
	return nil
}

func formatHumanRaftInfo(res *metastorev1.InfoResponse) string {
	maxKeyPadding := func(keys []string) int {
		max := 0
		for _, k := range keys {
			if len(k) > max {
				max = len(k)
			}
		}
		return max
	}

	appendPairs := func(sb *strings.Builder, pairs [][]string) {
		keys := make([]string, 0, len(pairs))
		for _, pair := range pairs {
			keys = append(keys, pair[0])
		}

		keyPadding := maxKeyPadding(keys)
		for _, pair := range pairs {
			key, value := pair[0], pair[1]
			sb.WriteString(fmt.Sprintf("%s:", key))
			sb.WriteString(strings.Repeat(" ", keyPadding-len(key)+1))
			sb.WriteString(fmt.Sprintf("%s\n", value))
		}
	}

	lastLeaderContact := "never"
	if res.LastLeaderContact != 0 {
		lastLeaderContact = time.UnixMilli(res.LastLeaderContact).Format(time.RFC3339)
	}

	var sb strings.Builder
	appendPairs(&sb, [][]string{
		{"ID", res.Id},
		{"State", res.State.String()},
		{"Leader ID", res.LeaderId},
		{"Last leader contact", lastLeaderContact},
		{"Term", fmt.Sprint(res.Term)},
		{"Suffrage", res.Suffrage.String()},
	})

	sb.WriteString("Log:\n")
	appendPairs(&sb, [][]string{
		{"  Commit index", fmt.Sprint(res.Log.CommitIndex)},
		{"  Applied index", fmt.Sprint(res.Log.AppliedIndex)},
		{"  Last index", fmt.Sprint(res.Log.LastIndex)},
		{"  FSM pending length", fmt.Sprint(res.Log.FsmPendingLength)},
	})

	sb.WriteString("Snapshot:\n")
	appendPairs(&sb, [][]string{
		{"  Last index", fmt.Sprint(res.Snapshot.LastIndex)},
		{"  Last term", fmt.Sprint(res.Snapshot.LastTerm)},
	})

	sb.WriteString("Protocol:\n")
	appendPairs(&sb, [][]string{
		{"  Version", fmt.Sprint(res.Protocol.Version)},
		{"  Min version", fmt.Sprint(res.Protocol.MinVersion)},
		{"  Max version", fmt.Sprint(res.Protocol.MaxVersion)},
		{"  Min snapshot version", fmt.Sprint(res.Protocol.MinSnapshotVersion)},
		{"  Max snapshot version", fmt.Sprint(res.Protocol.MaxSnapshotVersion)},
	})

	sb.WriteString("Peers:\n")
	for _, peer := range res.Peers {
		appendPairs(&sb, [][]string{
			{"  ID", peer.Id},
			{"  Address", peer.Address},
			{"  Suffrage", peer.Suffrage.String()},
		})
		sb.WriteString("\n") // Give some space between entries.
	}

	return strings.TrimSpace(sb.String())
}

func formatJSONRaftInfo(msg *metastorev1.InfoResponse) (string, error) {
	// Pretty print the protobuf json and don't omit default values.
	opts := protojson.MarshalOptions{
		Multiline:       true,
		Indent:          "  ",
		EmitUnpopulated: true,
	}

	bytes, err := opts.Marshal(msg.CloneMessageVT())
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}
