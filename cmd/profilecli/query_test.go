package main

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryProfileParams_ProfileIDValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		profileIDs []string
		wantErr    bool
	}{
		{
			name:       "valid UUID",
			profileIDs: []string{"550e8400-e29b-41d4-a716-446655440000"},
			wantErr:    false,
		},
		{
			name:       "valid UUID v4",
			profileIDs: []string{uuid.New().String()},
			wantErr:    false,
		},
		{
			name:       "multiple valid UUIDs",
			profileIDs: []string{"550e8400-e29b-41d4-a716-446655440000", uuid.New().String()},
			wantErr:    false,
		},
		{
			name:       "invalid UUID - span ID",
			profileIDs: []string{"deadbeef12345678"},
			wantErr:    true,
		},
		{
			name:       "invalid UUID - random string",
			profileIDs: []string{"not-a-uuid"},
			wantErr:    true,
		},
		{
			name:       "one valid one invalid",
			profileIDs: []string{"550e8400-e29b-41d4-a716-446655440000", "not-a-uuid"},
			wantErr:    true,
		},
		{
			name:       "empty slice is valid (not provided)",
			profileIDs: nil,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			params := &queryProfileParams{ProfileIDs: tt.profileIDs}
			err := validateQueryProfileParams(params)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "--profile-id must be a valid UUID")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestQueryProfileParams_MutualExclusion(t *testing.T) {
	t.Parallel()

	// --profile-id and --span-selector cannot be used together.
	err := validateQueryProfileParams(&queryProfileParams{
		ProfileIDs:   []string{"550e8400-e29b-41d4-a716-446655440000"},
		SpanSelector: []string{"deadbeef12345678"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--profile-id and --span-selector cannot be used together")

	// No conflict when only profile-id is set.
	err = validateQueryProfileParams(&queryProfileParams{
		ProfileIDs: []string{"550e8400-e29b-41d4-a716-446655440000"},
	})
	require.NoError(t, err)

	// No conflict when only span-selector is set.
	err = validateQueryProfileParams(&queryProfileParams{
		SpanSelector: []string{"deadbeef12345678"},
	})
	require.NoError(t, err)

	// --span-selector and --stacktrace-selector cannot be used together.
	err = validateQueryProfileParams(&queryProfileParams{
		SpanSelector:       []string{"deadbeef12345678"},
		StacktraceSelector: []string{"main"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--span-selector and --stacktrace-selector cannot be used together")
}
