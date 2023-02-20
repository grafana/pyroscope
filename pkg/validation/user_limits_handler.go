// SPDX-License-Identifier: AGPL-3.0-only

package validation

import (
	"net/http"

	"github.com/grafana/dskit/tenant"

	"github.com/grafana/phlare/pkg/util"
)

type TenantLimitsResponse struct {
	// Write path limits
	IngestionRate            float64 `json:"ingestion_rate"`
	IngestionBurstSize       int     `json:"ingestion_burst_size"`
	MaxGlobalSeriesPerTenant int     `json:"max_global_series_per_user"`

	// todo
}

// TenantLimitsHandler handles user limits.
func TenantLimitsHandler(defaultLimits Limits, tenantLimits TenantLimits) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := tenant.TenantID(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		userLimits := tenantLimits.TenantLimits(userID)
		if userLimits == nil {
			userLimits = &defaultLimits
		}

		limits := TenantLimitsResponse{
			// Write path limits
			IngestionRate:            userLimits.IngestionRateMB,
			IngestionBurstSize:       int(userLimits.IngestionBurstSizeMB),
			MaxGlobalSeriesPerTenant: userLimits.MaxGlobalSeriesPerTenant,
		}

		util.WriteJSONResponse(w, limits)
	}
}
