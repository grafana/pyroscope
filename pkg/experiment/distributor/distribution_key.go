package distributor

import (
	"github.com/cespare/xxhash/v2"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

// NewTenantServiceDatasetKey builds a distribution key, where the dataset
// is the service name, and the fingerprint is the hash of the labels.
// The resulting key references the tenant and dataset strings.
func NewTenantServiceDatasetKey(tenant string, labels ...*typesv1.LabelPair) placement.Key {
	dataset := phlaremodel.Labels(labels).Get(phlaremodel.LabelNameServiceName)
	return placement.Key{
		TenantID:    tenant,
		DatasetName: dataset,

		Tenant:      xxhash.Sum64String(tenant),
		Dataset:     xxhash.Sum64String(dataset),
		Fingerprint: phlaremodel.Labels(labels).Hash(),
	}
}
