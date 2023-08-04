package tenant

import (
	"context"

	"github.com/grafana/dskit/user"
)

const ErrNoTenantID = user.ErrNoOrgID

func InjectTenantID(ctx context.Context, tenantID string) context.Context {
	return user.InjectOrgID(ctx, tenantID)
}
