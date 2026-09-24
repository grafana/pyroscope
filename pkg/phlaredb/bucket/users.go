package bucket

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/thanos-io/objstore"

	phlareobjstore "github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/profiledump"
)

// PyroscopeInternalsPrefix is the bucket prefix under which all Pyroscope internal cluster-wide objects are stored.
// The object storage path delimiter (/) is appended to this prefix when building the full object path.
const PyroscopeInternalsPrefix = "__pyroscope_cluster"

// ListUsers returns all user IDs found scanning the root of the bucket.
func ListUsers(ctx context.Context, bucketClient objstore.Bucket) (users []string, err error) {
	// Collect users before processing them to keep the root listing cacheable.
	err = bucketClient.Iter(ctx, "", func(entry string) error {
		userID := strings.TrimSuffix(entry, "/")
		if strings.HasSuffix(entry, ".json") {
			return nil
		}
		if isUserIDReserved(userID) {
			return nil
		}

		users = append(users, userID)
		return nil
	})

	if err != nil {
		return nil, err
	}
	for i, userID := range users {
		if userID != strings.TrimSuffix(profiledump.ObjectPrefix, "/") {
			continue
		}
		// Capture storage can coexist with a real V1 tenant of the same name.
		present, err := hasProfileDumpV1Tenant(ctx, bucketClient, userID)
		if err != nil {
			return nil, fmt.Errorf("check V1 tenant in capture namespace: %w", err)
		}
		if !present {
			users = append(users[:i], users[i+1:]...)
		}
		break
	}
	return users, nil
}

func hasProfileDumpV1Tenant(ctx context.Context, bucketClient objstore.Bucket, userID string) (bool, error) {
	probeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	found := false
	tenantBucket := phlareobjstore.NewTenantBucketClient(userID, phlareobjstore.NewBucket(bucketClient), nil)
	b := tenantBucket.WithExpectedErrs(func(err error) bool {
		return (found && ctx.Err() == nil && errors.Is(err, context.Canceled)) || bucketClient.IsObjNotFoundErr(err)
	})
	err := b.Iter(probeCtx, "", func(string) error {
		if !found && probeCtx.Err() == nil {
			found = true
			cancel()
		}
		// Return nil so S3 can drain its listing producer after cancellation.
		// A callback error can strand that goroutine.
		return nil
	})
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if found && errors.Is(err, context.Canceled) {
		err = nil
	}
	if err != nil && !bucketClient.IsObjNotFoundErr(err) {
		return false, err
	}
	return found, nil
}

func isUserIDReserved(name string) bool {
	return name == PyroscopeInternalsPrefix
}
