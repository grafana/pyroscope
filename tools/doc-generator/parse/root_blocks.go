// SPDX-License-Identifier: AGPL-3.0-only

package parse

import (
	"reflect"

	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/kv/memberlist"
	"github.com/weaveworks/common/server"

	"github.com/grafana/fire/pkg/agent"
	"github.com/grafana/fire/pkg/distributor"
	"github.com/grafana/fire/pkg/ingester"
	"github.com/grafana/fire/pkg/querier"
)

var (
	// RootBlocks is an ordered list of root blocks. The order is the same order that will
	// follow the markdown generation.
	RootBlocks = []RootBlock{
		{
			Name:       "agent",
			StructType: reflect.TypeOf(agent.Config{}),
			Desc:       "The agent block configures the pull-mode collection of profiles.",
		},
		{
			Name:       "server",
			StructType: reflect.TypeOf(server.Config{}),
			Desc:       "The server block configures the HTTP and gRPC server of the launched service(s).",
		},
		{
			Name:       "distributor",
			StructType: reflect.TypeOf(distributor.Config{}),
			Desc:       "The distributor block configures the distributor.",
		},
		{
			Name:       "ingester",
			StructType: reflect.TypeOf(ingester.Config{}),
			Desc:       "The ingester block configures the ingester.",
		},
		{
			Name:       "querier",
			StructType: reflect.TypeOf(querier.Config{}),
			Desc:       "The querier block configures the querier.",
		},
		{
			Name:       "grpc_client",
			StructType: reflect.TypeOf(grpcclient.Config{}),
			Desc:       "The grpc_client block configures the gRPC client used to communicate between two Mimir components.",
		},
		{
			Name:       "memberlist",
			StructType: reflect.TypeOf(memberlist.KVConfig{}),
			Desc:       "The memberlist block configures the Gossip memberlist.",
		},
	}
)
