package metastore

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

func TestDeletionFilterSemantics(t *testing.T) {
	for _, tt := range []struct {
		name, regex, service string
		matches              bool
	}{
		{"full match", "svc-a|svc-b", "svc-a", true},
		{"not substring", "svc-a", "prefix-svc-a", false},
		{"empty is not wildcard", "", "svc-a", false},
		{"empty matches empty", "", "", true},
		{"explicit wildcard", ".*", "svc-a", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			re, err := validateDeletionRequest(&metastorev1.DeleteDatasetsRequest{Tenant: "tenant-a", Filter: &metastorev1.DeleteFilter{ServiceNameRegex: tt.regex}})
			require.NoError(t, err)
			require.Equal(t, tt.matches, re.MatchString(tt.service))
		})
	}

	// Expressions that only become valid once wrapped in anchors would match
	// with different, non-anchored semantics; they must be rejected outright.
	for _, regex := range []string{"a)|(b", "svc-a)", "(svc-a", "["} {
		t.Run("rejects "+regex, func(t *testing.T) {
			_, err := validateDeletionRequest(&metastorev1.DeleteDatasetsRequest{Tenant: "tenant-a", Filter: &metastorev1.DeleteFilter{ServiceNameRegex: regex}})
			require.Equal(t, codes.InvalidArgument, status.Code(err))
		})
	}
}
