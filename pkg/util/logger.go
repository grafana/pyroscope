package util

import (
	"context"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/tenant"
	"github.com/grafana/dskit/tracing"
)

// Logger is a global logger to use only where you cannot inject a logger.
var Logger = log.NewNopLogger()

// LoggerWithUserID returns a Logger that has information about the current user in
// its details.
func LoggerWithUserID(tenantID string, l log.Logger) log.Logger {
	// See note in WithContext.
	return log.With(l, "tenant", tenantID)
}

// LoggerWithUserIDs returns a Logger that has information about the current user or
// users (separated by "|") in its details.
func LoggerWithUserIDs(tenantIDs []string, l log.Logger) log.Logger {
	return log.With(l, "tenant", tenant.JoinTenantIDs(tenantIDs))
}

// LoggerWithTraceID returns a Logger that has information about the traceID in
// its details.
func LoggerWithTraceID(traceID string, l log.Logger) log.Logger {
	// See note in WithContext.
	return log.With(l, "traceID", traceID)
}

// LoggerWithContext returns a Logger that has information about the current user or users
// and trace in its details.
//
// e.g.
//
//	log = util.WithContext(ctx, log)
//	# level=error tenant=user-1|user-2 traceID=123abc msg="Could not chunk chunks" err="an error"
//	level.Error(log).Log("msg", "Could not chunk chunks", "err", err)
func LoggerWithContext(ctx context.Context, l log.Logger) log.Logger {
	// Weaveworks uses "orgs" and "orgID" to represent Cortex users,
	// even though the code-base generally uses `userID` to refer to the same thing.
	userIDs, err := tenant.TenantIDs(ctx)
	if err == nil {
		l = LoggerWithUserIDs(userIDs, l)
	}

	traceID, ok := tracing.ExtractSampledTraceID(ctx)
	if !ok {
		return l
	}

	return LoggerWithTraceID(traceID, l)
}

// WithSourceIPs returns a Logger that has information about the source IPs in
// its details.
func WithSourceIPs(sourceIPs string, l log.Logger) log.Logger {
	return log.With(l, "sourceIPs", sourceIPs)
}
