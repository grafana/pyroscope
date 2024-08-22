package distributor

import (
	"hash/fnv"

	v1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

// NewTenantServiceDatasetKey build a distribution key, where
func NewTenantServiceDatasetKey(tenant string, labels []*v1.LabelPair) Key {
	dataset := phlaremodel.Labels(labels).Get(phlaremodel.LabelNameServiceName)
	return Key{
		TenantID:    tenant,
		DatasetName: dataset,

		Tenant:       fnv64(tenant),
		Dataset:      fnv64(tenant, dataset),
		Distribution: fnv64(phlaremodel.LabelPairsString(labels)),
	}
}

func fnv64(keys ...string) uint64 {
	h := fnv.New64a()
	for _, k := range keys {
		_, _ = h.Write([]byte(k))
	}
	return h.Sum64()
}
