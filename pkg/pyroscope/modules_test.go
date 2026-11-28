package pyroscope

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/modules"
	"github.com/grafana/dskit/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every architecture must leave exactly one kind of seed writer; every other target follows.
func TestInitUsageReport_SeedLeader(t *testing.T) {
	for _, tc := range []struct {
		architecture string
		target       string
		wantLeader   bool
	}{
		{architecture: "v2", target: All, wantLeader: true},
		{architecture: "v2", target: SegmentWriter, wantLeader: true},
		// Distributor reaches SegmentWriterRing via SegmentWriterClient, but never SegmentWriter.
		{architecture: "v2", target: Distributor, wantLeader: false},

		{architecture: "v1-v2-dual", target: Ingester, wantLeader: true},
		{architecture: "v1-v2-dual", target: SegmentWriter, wantLeader: true},

		// v1 drops the segment writer from All, leaving the ingester.
		{architecture: "v1", target: All, wantLeader: true},
		{architecture: "v1", target: Distributor, wantLeader: false},
	} {
		t.Run(tc.architecture+"/"+tc.target, func(t *testing.T) {
			f := &Pyroscope{
				Cfg:    newTestConfig(t, []string{"-architecture.storage=" + tc.architecture, "-target=" + tc.target}),
				logger: &logger{Logger: log.NewNopLogger()},
			}
			// Without a configured object store, initUsageReport writes a filesystem bucket here.
			f.Cfg.PhlareDB.DataPath = t.TempDir()
			require.NoError(t, f.setupModuleManager())

			svc, err := f.initUsageReport()
			require.NoError(t, err)
			require.NotNil(t, svc, "usage report service should be constructed")

			assert.Equal(t, tc.wantLeader, f.Cfg.Analytics.Leader)
		})
	}
}

func TestSetupModuleManager_V2_ColocatedDependencies(t *testing.T) {
	for _, tc := range []struct {
		name         string
		args         []string
		wantContains map[string][]string
		wantExcludes map[string][]string
	}{
		{
			name: "v2/all orders distributor -> segment-writer -> metastore and compaction-worker -> metastore",
			args: []string{"-architecture.storage=v2", "-target=all"},
			wantContains: map[string][]string{
				Distributor:           {SegmentWriter, Metastore},
				SegmentWriter:         {Metastore},
				CompactionWorker:      {Metastore},
				QueryFrontend:         {Metastore},
				MetastoreAdmin:        {Metastore},
				QueryDiagnosticsAdmin: {Metastore},
			},
		},
		{
			name: "v1-v2-dual/all orders v2 modules before metastore",
			args: []string{"-architecture.storage=v1-v2-dual", "-target=all"},
			wantContains: map[string][]string{
				Distributor:      {SegmentWriter, Metastore},
				SegmentWriter:    {Metastore},
				CompactionWorker: {Metastore},
				QueryFrontend:    {Metastore},
			},
		},
		{
			name: "v1/all does not add v2 dependencies",
			args: []string{"-architecture.storage=v1", "-target=all"},
			wantExcludes: map[string][]string{
				Distributor:   {SegmentWriter, Metastore},
				QueryFrontend: {Metastore},
			},
		},
		{
			name: "v2/distributor single-target stays decoupled",
			args: []string{"-architecture.storage=v2", "-target=distributor"},
			wantExcludes: map[string][]string{
				Distributor: {SegmentWriter, Metastore},
			},
		},
		{
			name: "v2/segment-writer single-target stays decoupled",
			args: []string{"-architecture.storage=v2", "-target=segment-writer"},
			wantExcludes: map[string][]string{
				SegmentWriter: {Metastore},
			},
		},
		{
			name: "v2/compaction-worker single-target stays decoupled",
			args: []string{"-architecture.storage=v2", "-target=compaction-worker"},
			wantExcludes: map[string][]string{
				CompactionWorker: {Metastore},
			},
		},
		{
			name: "v2/query-frontend single-target stays decoupled",
			args: []string{"-architecture.storage=v2", "-target=query-frontend"},
			wantExcludes: map[string][]string{
				QueryFrontend: {Metastore},
			},
		},
		{
			name: "v2/partial co-location orders only active modules",
			args: []string{"-architecture.storage=v2", "-target=metastore,segment-writer"},
			wantContains: map[string][]string{
				SegmentWriter: {Metastore},
			},
			wantExcludes: map[string][]string{
				Distributor: {SegmentWriter, Metastore},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &Pyroscope{
				Cfg:    newTestConfig(t, tc.args),
				logger: &logger{Logger: log.NewNopLogger()},
			}
			require.NoError(t, f.setupModuleManager())

			for mod, expected := range tc.wantContains {
				deps := f.ModuleManager.DependenciesForModule(mod)
				for _, dep := range expected {
					assert.True(t, slices.Contains(deps, dep), "expected %s to depend on %s (deps=%v)", mod, dep, deps)
				}
			}
			for mod, excluded := range tc.wantExcludes {
				deps := f.ModuleManager.DependenciesForModule(mod)
				for _, dep := range excluded {
					assert.False(t, slices.Contains(deps, dep), "expected %s not to depend on %s (deps=%v)", mod, dep, deps)
				}
			}
		})
	}
}

func TestModuleManager_V2_ShutdownOrder(t *testing.T) {
	f := &Pyroscope{
		Cfg:    newTestConfig(t, []string{"-architecture.storage=v2", "-target=all"}),
		logger: &logger{Logger: log.NewNopLogger()},
	}
	require.NoError(t, f.setupModuleManager())

	mm := modules.NewManager(log.NewNopLogger())

	var (
		mu             sync.Mutex
		events         []string
		stoppingTarget = map[string]chan struct{}{
			Distributor:      make(chan struct{}),
			SegmentWriter:    make(chan struct{}),
			CompactionWorker: make(chan struct{}),
			Metastore:        make(chan struct{}),
		}
		releaseStop = map[string]chan struct{}{
			Distributor:      make(chan struct{}),
			SegmentWriter:    make(chan struct{}),
			CompactionWorker: make(chan struct{}),
			Metastore:        make(chan struct{}),
		}
	)

	recordStop := func(name string) func(error) error {
		return func(error) error {
			mu.Lock()
			events = append(events, "stop-start:"+name)
			mu.Unlock()

			if ch, ok := stoppingTarget[name]; ok {
				close(ch)
				<-releaseStop[name]
			}

			mu.Lock()
			events = append(events, "stop-end:"+name)
			mu.Unlock()
			return nil
		}
	}

	register := func(mod string) {
		if mm.IsModuleRegistered(mod) {
			return
		}
		if mod == All {
			mm.RegisterModule(mod, nil)
			return
		}
		modName := mod
		mm.RegisterModule(modName, func() (services.Service, error) {
			return services.NewIdleService(nil, recordStop(modName)), nil
		})
	}
	for mod, targets := range f.deps {
		register(mod)
		for _, target := range targets {
			register(target)
		}
	}
	for mod, targets := range f.deps {
		require.NoError(t, mm.AddDependency(mod, targets...))
	}

	svcMap, err := mm.InitModuleServices(All)
	require.NoError(t, err)

	var allServices []services.Service
	for _, svc := range svcMap {
		allServices = append(allServices, svc)
	}
	sm, err := services.NewManager(allServices...)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, sm.StartAsync(ctx))
	require.NoError(t, sm.AwaitHealthy(ctx))

	sm.StopAsync()

	// Distributor and CompactionWorker may enter stopping first; SegmentWriter and Metastore must wait.
	<-stoppingTarget[Distributor]
	<-stoppingTarget[CompactionWorker]

	select {
	case <-stoppingTarget[SegmentWriter]:
		t.Fatal("segment-writer started stopping before distributor terminated")
	case <-stoppingTarget[Metastore]:
		t.Fatal("metastore started stopping before distributor and compaction-worker terminated")
	case <-time.After(30 * time.Millisecond):
	}

	// Release distributor; segment-writer should now stop while metastore still waits.
	close(releaseStop[Distributor])
	<-stoppingTarget[SegmentWriter]

	select {
	case <-stoppingTarget[Metastore]:
		t.Fatal("metastore started stopping before segment-writer and compaction-worker terminated")
	case <-time.After(30 * time.Millisecond):
	}

	// Release segment-writer while compaction-worker is still draining; metastore must still wait.
	close(releaseStop[SegmentWriter])
	select {
	case <-stoppingTarget[Metastore]:
		t.Fatal("metastore started stopping before compaction-worker terminated")
	case <-time.After(30 * time.Millisecond):
	}

	// Release compaction-worker; metastore can now stop.
	close(releaseStop[CompactionWorker])
	<-stoppingTarget[Metastore]
	close(releaseStop[Metastore])

	require.NoError(t, sm.AwaitStopped(ctx))

	assertStoppedBefore := func(first, second string) {
		t.Helper()
		firstEnd := slices.Index(events, "stop-end:"+first)
		secondStart := slices.Index(events, "stop-start:"+second)
		require.NotEqual(t, -1, firstEnd, "missing stop-end for %s", first)
		require.NotEqual(t, -1, secondStart, "missing stop-start for %s", second)
		assert.Less(t, firstEnd, secondStart, "%s must stop before %s starts stopping", first, second)
	}
	assertStoppedBefore(Distributor, SegmentWriter)
	assertStoppedBefore(SegmentWriter, Metastore)
	assertStoppedBefore(CompactionWorker, Metastore)
	assertStoppedBefore(Metastore, MetastoreClient)
	assertStoppedBefore(Metastore, Server)
}
