// SPDX-License-Identifier: AGPL-3.0-only

package parse

import (
	"reflect"

	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/kv/memberlist"
	"github.com/grafana/dskit/server"

	"github.com/grafana/pyroscope/pkg/compactor"
	"github.com/grafana/pyroscope/pkg/distributor"
	"github.com/grafana/pyroscope/pkg/frontend"
	"github.com/grafana/pyroscope/pkg/ingester"
	"github.com/grafana/pyroscope/pkg/objstore/providers/azure"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/pkg/objstore/providers/gcs"
	"github.com/grafana/pyroscope/pkg/objstore/providers/s3"
	"github.com/grafana/pyroscope/pkg/objstore/providers/swift"
	"github.com/grafana/pyroscope/pkg/querier"
	"github.com/grafana/pyroscope/pkg/querier/worker"
	"github.com/grafana/pyroscope/pkg/scheduler"
	"github.com/grafana/pyroscope/pkg/storegateway"
	"github.com/grafana/pyroscope/pkg/validation"
)

// RootBlocks is an ordered list of root blocks. The order is the same order that will
// follow the markdown generation.
var RootBlocks = []RootBlock{
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
		Name:       "query_frontend",
		StructType: reflect.TypeOf(frontend.Config{}),
		Desc:       "The query_frontend block configures the query-frontend.",
	},
	{
		Name:       "frontend_worker",
		StructType: reflect.TypeOf(worker.Config{}),
		Desc:       "The frontend_worker block configures the frontend-worker.",
	},
	{
		Name:       "query_scheduler",
		StructType: reflect.TypeOf(scheduler.Config{}),
		Desc:       "The query_scheduler block configures the query-scheduler.",
	},
	{
		Name:       "store_gateway",
		StructType: reflect.TypeOf(storegateway.Config{}),
		Desc:       "The store_gateway block configures the store-gateway.",
	},
	{
		Name:       "compactor",
		StructType: reflect.TypeOf(compactor.Config{}),
		Desc:       "The compactor block configures the compactor.",
	},
	{
		Name:       "grpc_client",
		StructType: reflect.TypeOf(grpcclient.Config{}),
		Desc:       "The grpc_client block configures the gRPC client used to communicate between two Pyroscope components.",
	},
	{
		Name:       "memberlist",
		StructType: reflect.TypeOf(memberlist.KVConfig{}),
		Desc:       "The memberlist block configures the Gossip memberlist.",
	},
	{
		Name:       "limits",
		StructType: reflect.TypeOf(validation.Limits{}),
		Desc:       "The limits block configures default and per-tenant limits imposed by components.",
	},
	{
		Name:       "s3_storage_backend",
		StructType: reflect.TypeOf(s3.Config{}),
		Desc:       "The s3_backend block configures the connection to Amazon S3 object storage backend.",
	},
	{
		Name:       "gcs_storage_backend",
		StructType: reflect.TypeOf(gcs.Config{}),
		Desc:       "The gcs_backend block configures the connection to Google Cloud Storage object storage backend.",
	},
	{
		Name:       "azure_storage_backend",
		StructType: reflect.TypeOf(azure.Config{}),
		Desc:       "The azure_storage_backend block configures the connection to Azure object storage backend.",
	},
	{
		Name:       "swift_storage_backend",
		StructType: reflect.TypeOf(swift.Config{}),
		Desc:       "The swift_storage_backend block configures the connection to OpenStack Object Storage (Swift) object storage backend.",
	},
	{
		Name:       "filesystem_storage_backend",
		StructType: reflect.TypeOf(filesystem.Config{}),
		Desc:       "The filesystem_storage_backend block configures the usage of local file system as object storage backend.",
	},
}
