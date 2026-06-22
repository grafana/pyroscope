// SPDX-License-Identifier: AGPL-3.0-only

package parse

import (
	"reflect"

	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/kv/memberlist"
	"github.com/grafana/dskit/server"

	"github.com/grafana/pyroscope/v2/pkg/compactionworker"
	"github.com/grafana/pyroscope/v2/pkg/compactor"
	"github.com/grafana/pyroscope/v2/pkg/distributor"
	"github.com/grafana/pyroscope/v2/pkg/frontend"
	"github.com/grafana/pyroscope/v2/pkg/ingester"
	"github.com/grafana/pyroscope/v2/pkg/metastore"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/azure"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/gcs"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/s3"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/swift"
	"github.com/grafana/pyroscope/v2/pkg/querier"
	"github.com/grafana/pyroscope/v2/pkg/querier/worker"
	"github.com/grafana/pyroscope/v2/pkg/querybackend"
	"github.com/grafana/pyroscope/v2/pkg/scheduler"
	"github.com/grafana/pyroscope/v2/pkg/segmentwriter"
	placement "github.com/grafana/pyroscope/v2/pkg/segmentwriter/client/distributor/placement/adaptiveplacement"
	"github.com/grafana/pyroscope/v2/pkg/storegateway"
	"github.com/grafana/pyroscope/v2/pkg/symbolizer"
	"github.com/grafana/pyroscope/v2/pkg/usagestats"
	"github.com/grafana/pyroscope/v2/pkg/validation"
	"github.com/grafana/pyroscope/v2/pkg/validation/exporter"
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
		Name:       "segment_writer",
		StructType: reflect.TypeOf(segmentwriter.Config{}),
		Desc:       "The segment_writer block configures the segment-writer (V2 write path).",
	},
	{
		Name:       "metastore",
		StructType: reflect.TypeOf(metastore.Config{}),
		Desc:       "The metastore block configures the metastore (V2).",
	},
	{
		Name:       "compaction_worker",
		StructType: reflect.TypeOf(compactionworker.Config{}),
		Desc:       "The compaction_worker block configures the compaction-worker (V2).",
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
		Name:       "query_backend",
		StructType: reflect.TypeOf(querybackend.Config{}),
		Desc:       "The query_backend block configures the query-backend (V2 read path).",
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
		Name:       "adaptive_placement",
		StructType: reflect.TypeOf(placement.Config{}),
		Desc:       "The adaptive_placement block configures adaptive placement for the segment-writer (V2).",
	},
	{
		Name:       "symbolizer",
		StructType: reflect.TypeOf(symbolizer.Config{}),
		Desc:       "The symbolizer block configures the symbolizer (V2).",
	},
	{
		Name:       "overrides_exporter",
		StructType: reflect.TypeOf(exporter.Config{}),
		Desc:       "The overrides_exporter block configures the overrides exporter.",
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
	{
		Name:       "analytics",
		StructType: reflect.TypeOf(usagestats.Config{}),
		Desc:       "The analytics block configures usage statistics collection. For more details about usage statistics, refer to [Anonymous usage statistics reporting](../anonymous-usage-statistics-reporting)",
	},
}
