package pyroscope

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProfileDumpModuleDependencies(t *testing.T) {
	for _, tc := range []struct {
		architecture, target string
		recorder, cleaner    bool
	}{
		{"v1", Distributor, false, false},
		{"v1", Admin, false, false},
		{"v2", Distributor, true, false},
		{"v2", Admin, false, true},
		{"v1-v2-dual", Distributor, true, false},
		{"v1-v2-dual", Admin, false, true},
	} {
		t.Run(tc.architecture+"/"+tc.target, func(t *testing.T) {
			f := &Pyroscope{Cfg: newTestConfig(t, []string{"-architecture.storage=" + tc.architecture})}
			require.NoError(t, f.setupModuleManager())
			deps := f.ModuleManager.DependenciesForModule(tc.target)
			require.Equal(t, tc.recorder, slices.Contains(deps, ProfileDumpRecorder))
			require.Equal(t, tc.cleaner, slices.Contains(deps, ProfileDumpCleaner))
		})
	}
	f := &Pyroscope{}
	svc, err := f.initProfileDumpRecorder()
	require.NoError(t, err)
	require.Nil(t, svc)
	require.Nil(t, f.profileDumpRecorder)
	svc, err = f.initProfileDumpCleaner()
	require.NoError(t, err)
	require.Nil(t, svc)
}
