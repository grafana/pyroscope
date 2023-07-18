// SPDX-License-Identifier: AGPL-3.0-only

package validation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/weaveworks/common/user"
)

func TestTenantLimitsHandler(t *testing.T) {
	defaults := Limits{
		IngestionRateMB:      100,
		IngestionBurstSizeMB: 10,
	}

	tenantLimits := make(map[string]*Limits)
	testLimits := defaults
	testLimits.IngestionRateMB = 200
	tenantLimits["test-with-override"] = &testLimits

	for _, tc := range []struct {
		name               string
		orgID              string
		expectedStatusCode int
		expectedLimits     TenantLimitsResponse
	}{
		{
			name:               "Authenticated user with override",
			orgID:              "test-with-override",
			expectedStatusCode: http.StatusOK,
			expectedLimits: TenantLimitsResponse{
				IngestionRate:      200,
				IngestionBurstSize: 10,
			},
		},
		{
			name:               "Authenticated user without override",
			orgID:              "test-no-override",
			expectedStatusCode: http.StatusOK,
			expectedLimits: TenantLimitsResponse{
				IngestionRate:      100,
				IngestionBurstSize: 10,
			},
		},
		{
			name:               "Unauthenticated user",
			orgID:              "",
			expectedStatusCode: http.StatusUnauthorized,
			expectedLimits:     TenantLimitsResponse{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := TenantLimitsHandler(defaults, NewMockTenantLimits(tenantLimits))
			request := httptest.NewRequest("GET", "/api/v1/user_limits", nil)
			if tc.orgID != "" {
				ctx := user.InjectOrgID(context.Background(), tc.orgID)
				request = request.WithContext(ctx)
			}

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			require.Equal(t, tc.expectedStatusCode, recorder.Result().StatusCode)

			if recorder.Result().StatusCode == http.StatusOK {
				var response TenantLimitsResponse
				decoder := json.NewDecoder(recorder.Result().Body)
				require.NoError(t, decoder.Decode(&response))
				require.Equal(t, tc.expectedLimits, response)
			}
		})
	}
}
