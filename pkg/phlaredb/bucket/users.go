package bucket

import (
	"context"
	"strings"

	"github.com/thanos-io/objstore"
)

// PyroscopeInternalsPrefix is the bucket prefix under which all Pyroscope internal cluster-wide objects are stored.
// The object storage path delimiter (/) is appended to this prefix when building the full object path.
const PyroscopeInternalsPrefix = "__pyroscope_cluster"

// ListUsers returns all user IDs found scanning the root of the bucket.
func ListUsers(ctx context.Context, bucketClient objstore.Bucket) (users []string, err error) {
	// Iterate the bucket to find all users in the bucket. Due to how the bucket listing
	// caching works, it's more likely to have a cache hit if there's no delay while
	// iterating the bucket, so we do load all users in memory and later process them.
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

	return users, err
}

// isUserIDReserved returns whether the provided user ID is reserved and can't be used for storing metrics.
func isUserIDReserved(name string) bool {
	return name == PyroscopeInternalsPrefix
}
