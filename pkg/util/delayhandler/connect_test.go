package delayhandler

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/tenant"
)

func TestConnectInterceptor(t *testing.T) {
	now := time.Unix(1718211600, 0)
	tenantID := "tenant"

	tests := []struct {
		name         string
		configDelay  time.Duration
		cancelDelay  bool
		expectSleep  bool
		expectHeader bool
	}{
		{
			name:         "no delay",
			configDelay:  0,
			expectSleep:  false,
			expectHeader: false,
		},
		{
			name:         "with delay",
			configDelay:  100 * time.Millisecond,
			expectSleep:  true,
			expectHeader: true,
		},
		{
			name:         "cancelled delay",
			configDelay:  100 * time.Millisecond,
			cancelDelay:  true,
			expectSleep:  false,
			expectHeader: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeNowMock(t, []time.Time{now, now.Add(5 * time.Millisecond)})
			sleeps, cleanUpSleep := timeAfterMock()
			defer cleanUpSleep()

			limits := newMockLimits()
			limits.setDelay(tenantID, tt.configDelay)

			interceptor := NewConnect(limits)

			handler := connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				if tt.cancelDelay {
					CancelDelay(ctx)
				}
				return connect.NewResponse(&struct{}{}), nil
			})

			wrappedHandler := interceptor.WrapUnary(handler)
			req := connect.NewRequest(&struct{}{})
			ctx := tenant.InjectTenantID(context.Background(), tenantID)

			resp, err := wrappedHandler(ctx, req)

			require.NoError(t, err)
			require.NotNil(t, resp)

			if tt.expectSleep {
				require.Len(t, sleeps.values, 1)
				assert.Greater(t, sleeps.values[0], 80*time.Millisecond)
				assert.Less(t, sleeps.values[0], 120*time.Millisecond)
			} else {
				require.Len(t, sleeps.values, 0)
			}

			if tt.expectHeader {
				assert.Contains(t, resp.Header().Get("Server-Timing"), "artificial_delay")
			} else {
				assert.Empty(t, resp.Header().Get("Server-Timing"))
			}
		})
	}
}
