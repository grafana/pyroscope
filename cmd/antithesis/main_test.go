package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommandName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{name: "container default", args: []string{"antithesis"}, want: "idle"},
		{name: "container idle argument", args: []string{"antithesis", "idle"}, want: "idle"},
		{name: "readiness probe", args: []string{"antithesis", "ready"}, want: "ready"},
		{name: "container command argument", args: []string{"antithesis", "first_seed"}, want: "first_seed"},
		{name: "composer symlink", args: []string{"/opt/antithesis/test/v1/core/first_seed"}, want: "first_seed"},
		{name: "singleton smoke symlink", args: []string{"/opt/antithesis/test/v1/core/singleton_driver_smoke"}, want: "singleton_driver_smoke"},
		{name: "unknown composer symlink", args: []string{"/opt/antithesis/test/v1/core/first_unknown"}, wantErr: true},
		{name: "unknown argument", args: []string{"antithesis", "unknown"}, wantErr: true},
		{name: "missing argv zero", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := commandName(tt.args)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestIdleReturnsWhenContextIsCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.NoError(t, Idle(ctx))
}
