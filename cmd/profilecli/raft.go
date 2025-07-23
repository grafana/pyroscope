package main

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	connectapi "github.com/grafana/pyroscope/pkg/api/connect"
	"github.com/grafana/pyroscope/pkg/metastore/raftnode/raftnodepb"
	"github.com/grafana/pyroscope/pkg/metastore/raftnode/raftnodepb/raftnodepbconnect"
)

func (c *phlareClient) metadataOperatorClient() raftnodepbconnect.RaftNodeServiceClient {
	return raftnodepbconnect.NewRaftNodeServiceClient(
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
	client := params.metadataOperatorClient()

	res, err := client.NodeInfo(ctx, connect.NewRequest(&raftnodepb.NodeInfoRequest{}))
	if err != nil {
		return err
	}

	var s string
	switch {
	case params.HumanFormat:
		s = formatHumanRaftInfo(res.Msg.Node)
	default:
		s, err = formatJSONRaftInfo(res.Msg.Node)
		if err != nil {
			return err
		}
	}

	fmt.Println(s)
	return nil
}

func formatHumanRaftInfo(node *raftnodepb.NodeInfo) string {
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
			fmt.Fprintf(sb, "%s:", key)
			sb.WriteString(strings.Repeat(" ", keyPadding-len(key)+1))
			fmt.Fprintf(sb, "%s\n", value)
		}
	}

	var sb strings.Builder
	appendPairs(&sb, [][]string{
		{"ID", node.ServerId},
		{"Address", node.AdvertisedAddress},
		{"State", node.State},
		{"Leader ID", node.LeaderId},
	})

	sb.WriteString("Log:\n")
	appendPairs(&sb, [][]string{
		{"  Commit index", fmt.Sprint(node.CommitIndex)},
		{"  Applied index", fmt.Sprint(node.AppliedIndex)},
		{"  Last index", fmt.Sprint(node.LastIndex)},
	})

	sb.WriteString("Stats:\n")
	for i := range node.Stats.Name {
		appendPairs(&sb, [][]string{
			{"  " + node.Stats.Name[i], node.Stats.Value[i]},
		})
	}

	sb.WriteString("Peers:\n")
	for _, peer := range node.Peers {
		appendPairs(&sb, [][]string{
			{"  ID", peer.ServerId},
			{"  Address", peer.ServerAddress},
			{"  Suffrage", peer.Suffrage},
		})
		sb.WriteString("\n") // Give some space between entries.
	}

	return strings.TrimSpace(sb.String())
}

func formatJSONRaftInfo(node *raftnodepb.NodeInfo) (string, error) {
	// Pretty print the protobuf json and don't omit default values.
	opts := protojson.MarshalOptions{
		Multiline:       true,
		Indent:          "  ",
		EmitUnpopulated: true,
	}

	bytes, err := opts.Marshal(node)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}
