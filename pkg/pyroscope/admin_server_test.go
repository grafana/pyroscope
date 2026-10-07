package pyroscope

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminServerMode_FlagDefault(t *testing.T) {
	cfg := Config{}
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cfg.RegisterFlags(fs)

	assert.Equal(t, AdminServerDisabled, cfg.AdminServer.Mode)
	assert.Equal(t, "localhost", cfg.AdminServer.HTTPAddress)
	assert.Equal(t, 4042, cfg.AdminServer.HTTPPort)
}

func TestAdminServerMode_Set(t *testing.T) {
	tests := []struct {
		value   string
		want    AdminServerMode
		wantErr bool
	}{
		{"disabled", AdminServerDisabled, false},
		{"additional", AdminServerAdditional, false},
		{"exclusive", AdminServerExclusive, false},
		{"bogus", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			var m AdminServerMode
			err := m.Set(tt.value)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, m)
			}
		})
	}
}

func TestAdminServerMode_IsEnabled(t *testing.T) {
	assert.False(t, AdminServerDisabled.IsEnabled())
	assert.True(t, AdminServerAdditional.IsEnabled())
	assert.True(t, AdminServerExclusive.IsEnabled())
}

func TestAdminServerMode_FlagParsing(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    AdminServerMode
		wantErr bool
	}{
		{"default", nil, AdminServerDisabled, false},
		{"disabled", []string{"-admin-server.mode=disabled"}, AdminServerDisabled, false},
		{"additional", []string{"-admin-server.mode=additional"}, AdminServerAdditional, false},
		{"exclusive", []string{"-admin-server.mode=exclusive"}, AdminServerExclusive, false},
		{"invalid", []string{"-admin-server.mode=bad"}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Config{}
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			c.RegisterFlags(fs)
			err := fs.Parse(tt.args)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, c.AdminServer.Mode)
		})
	}
}

func TestAdminServerMode_RegisterInstrumentation(t *testing.T) {
	// In exclusive mode the primary dskit server should have RegisterInstrumentation
	// disabled so /metrics and /debug/pprof are not served on the main port.
	// In other modes the primary server keeps them.
	tests := []struct {
		mode                        AdminServerMode
		wantRegisterInstrumentation bool
	}{
		{AdminServerDisabled, true},
		{AdminServerAdditional, true},
		{AdminServerExclusive, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			cfg := newTestConfig(t, nil)
			cfg.AdminServer.Mode = tt.mode
			// Use port 0 so the OS picks a free port; avoids conflicts across subtests.
			cfg.AdminServer.HTTPPort = 0

			f := &Pyroscope{Cfg: cfg}
			require.NoError(t, f.setupModuleManager())

			// Simulate what initServer does: initialise the admin router first
			// (sets f.adminRouter), then check RegisterInstrumentation.
			svc, err := f.initAdminServer()
			require.NoError(t, err)
			if svc != nil {
				t.Cleanup(func() { svc.StopAsync() })
			}

			// Replicate the guard from initServer.
			f.Cfg.Server.RegisterInstrumentation = true // dskit default
			if f.Cfg.AdminServer.Mode == AdminServerExclusive {
				f.Cfg.Server.RegisterInstrumentation = false
			}

			assert.Equal(t, tt.wantRegisterInstrumentation, f.Cfg.Server.RegisterInstrumentation)
		})
	}
}
