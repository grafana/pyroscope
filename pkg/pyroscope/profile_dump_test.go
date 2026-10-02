package pyroscope

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/modules"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	thanos "github.com/thanos-io/objstore"

	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/profiledump"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

type dumpLifecycleBucket struct {
	objstore.Bucket
	started, canceled, release chan struct{}
	closes                     atomic.Int32
}

func (b *dumpLifecycleBucket) Upload(ctx context.Context, _ string, r io.Reader, _ ...thanos.ObjectUploadOption) error {
	close(b.started)
	<-ctx.Done()
	close(b.canceled)
	<-b.release
	_, err := io.Copy(io.Discard, r)
	return err
}
func (b *dumpLifecycleBucket) Close() error { b.closes.Add(1); return nil }

func newDumpApplication(t *testing.T) (*Pyroscope, *dumpLifecycleBucket) {
	t.Helper()
	cfg := newTestConfig(t, nil)
	cfg.ProfileDump.Recorder.ShutdownDrain = time.Millisecond
	cfg.ShowBanner = false
	bucket := &dumpLifecycleBucket{Bucket: objstore.NewBucket(thanos.NewInMemBucket()), started: make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{})}
	until := time.Now().Add(time.Minute)
	probability := 1.0
	overrides := validation.MockOverrides(func(defaults *validation.Limits, tenants map[string]*validation.Limits) {
		l := *defaults
		l.ProfileDebugDump = &profiledump.TenantConfig{ActiveUntil: &until, Probability: &probability}
		require.NoError(t, l.Validate(cfg.ProfileDump, time.Now()))
		tenants["a"] = &l
	})
	f := &Pyroscope{Cfg: cfg, logger: &logger{Logger: log.NewNopLogger()}, reg: prometheus.NewRegistry(), Overrides: overrides,
		storageBucket: objstore.NewBorrowedBucket(bucket), closeStorageBucket: sync.OnceValue(bucket.Close)}
	return f, bucket
}

func TestProfileDumpLifecycle(t *testing.T) {
	f, bucket := newDumpApplication(t)
	svc, err := f.initProfileDumpRecorder()
	require.NoError(t, err)
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), svc))
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(bucket.release) }); require.NoError(t, f.stopStorage()) })
	outcome := f.profileDumpRecorder.Capture(context.Background(), "a", profiledump.Candidate{Metadata: profiledump.Metadata{SourceProtocol: profiledump.SourceConnect, NativeFormat: profiledump.FormatPprof, PayloadEncoding: "identity"}, Payload: []byte("opaque")})
	require.True(t, outcome.Enqueued)
	<-bucket.started
	require.NoError(t, f.storageBucket.Close())
	require.NoError(t, objstore.NewSSEBucketClient("a", f.storageBucket, f.Overrides).Close())
	require.Zero(t, bucket.closes.Load())
	stopped := make(chan error, 1)
	go func() { stopped <- services.StopAndAwaitTerminated(context.Background(), svc) }()
	<-bucket.canceled
	require.ErrorIs(t, f.profileDumpRecorder.Shutdown(context.Background()), context.DeadlineExceeded)
	select {
	case <-f.profileDumpRecorder.Done():
		t.Fatal("worker still owns the bucket")
	default:
	}
	select {
	case <-stopped:
		t.Fatal("service must wait for recorder Done")
	default:
	}
	storageStopped := make(chan error, 1)
	go func() { storageStopped <- f.stopStorage() }()
	require.Zero(t, bucket.closes.Load())
	release.Do(func() { close(bucket.release) })
	require.NoError(t, <-stopped)
	require.NoError(t, <-storageStopped)
	require.Equal(t, int32(1), bucket.closes.Load())
	require.NoError(t, f.stopStorage())
	require.Equal(t, int32(1), bucket.closes.Load())
}

func TestProfileDumpInitializationFailureCleanup(t *testing.T) {
	f, bucket := newDumpApplication(t)
	mm := modules.NewManager(log.NewNopLogger())
	mm.RegisterModule("recorder", f.initProfileDumpRecorder)
	failure := errors.New("later constructor failed")
	mm.RegisterModule("failure", func() (services.Service, error) { return nil, failure })
	require.NoError(t, mm.AddDependency("failure", "recorder"))
	f.ModuleManager, f.Cfg.Target = mm, []string{"failure"}
	require.ErrorIs(t, f.Run(), failure)
	select {
	case <-f.profileDumpRecorder.Done():
	default:
		t.Fatal("recorder leaked after startup failure")
	}
	require.Equal(t, int32(1), bucket.closes.Load())
}

func TestProfileDumpModuleDependencies(t *testing.T) {
	for _, architecture := range []string{"v1", "v2", "v1-v2-dual"} {
		t.Run(architecture, func(t *testing.T) {
			f := &Pyroscope{Cfg: newTestConfig(t, []string{"-architecture.storage=" + architecture})}
			require.NoError(t, f.setupModuleManager())
			require.Contains(t, f.deps[Distributor], ProfileDumpRecorder)
			require.ElementsMatch(t, []string{Storage, Overrides}, f.deps[ProfileDumpRecorder])
			require.NotContains(t, f.deps[Admin], ProfileDumpRecorder)
			require.Contains(t, f.deps[Admin], ProfileDumpCleaner)
			require.ElementsMatch(t, []string{Storage}, f.deps[ProfileDumpCleaner])
			require.False(t, f.ModuleManager.IsUserVisibleModule(ProfileDumpCleaner))
			for _, target := range []string{Admin, All} {
				require.Contains(t, f.ModuleManager.DependenciesForModule(target), ProfileDumpCleaner)
			}
			require.NotContains(t, f.ModuleManager.DependenciesForModule(Distributor), ProfileDumpCleaner)
		})
	}
	f, _ := newDumpApplication(t)
	f.storageBucket = nil
	svc, err := f.initProfileDumpRecorder()
	require.NoError(t, err)
	require.Nil(t, svc)
	require.Nil(t, f.profileDumpRecorder)
}

func TestProfileDumpRecorderConfiguration(t *testing.T) {
	cfg := newTestConfig(t, []string{"-profile-dump.workers=3"})
	require.Equal(t, 3, cfg.ProfileDump.Recorder.Workers)
	cfg.ProfileDump.Recorder.Workers = 0
	require.ErrorContains(t, cfg.Validate(), "profile_dump recorder")
	_, err := validation.LoadRuntimeConfigWithProfileDump(strings.NewReader("overrides: {}"), cfg.ProfileDump, time.Now())
	require.NoError(t, err, "runtime policy compilation is independent of process recorder validation")
}

func TestProfileDumpStorageOwnership(t *testing.T) {
	f, bucket := newDumpApplication(t)
	f.storageBucket, f.closeStorageBucket = nil, nil
	f.Cfg.Storage.Bucket.Backend = "filesystem"
	f.Cfg.Storage.Bucket.Filesystem.Directory = t.TempDir()
	f.Cfg.Storage.Bucket.Middlewares = append(f.Cfg.Storage.Bucket.Middlewares, func(b thanos.Bucket) (thanos.Bucket, error) {
		bucket.Bucket = objstore.NewBucket(b)
		return bucket, nil
	})
	svc, err := f.initStorage()
	require.NoError(t, err)
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), svc))
	require.NoError(t, f.storageBucket.Close())
	require.Zero(t, bucket.closes.Load())
	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), svc))
	require.Equal(t, int32(1), bucket.closes.Load())
	require.NoError(t, f.stopStorage())
	require.Equal(t, int32(1), bucket.closes.Load())
}

type dumpCleanupBucket struct {
	objstore.Bucket
	entered, canceled, release chan struct{}
	active, closes             atomic.Int32
}

func (b *dumpCleanupBucket) Delete(ctx context.Context, key string) error {
	b.active.Add(1)
	defer b.active.Add(-1)
	close(b.entered)
	<-ctx.Done()
	close(b.canceled)
	<-b.release
	return b.Bucket.Delete(context.Background(), key)
}
func (b *dumpCleanupBucket) Close() error {
	if b.active.Load() != 0 {
		return errors.New("storage closed while cleaner active")
	}
	b.closes.Add(1)
	return nil
}

func TestProfileDumpCleanerStorageOrder(t *testing.T) {
	for _, target := range []string{Admin, "combined"} {
		for _, failureMode := range []string{"stop", "startup", "running"} {
			t.Run(target+"/"+failureMode, func(t *testing.T) {
				f, _ := newDumpApplication(t)
				b := &dumpCleanupBucket{Bucket: objstore.NewBucket(thanos.NewInMemBucket()), entered: make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{})}
				f.storageBucket, f.closeStorageBucket = objstore.NewBorrowedBucket(b), sync.OnceValue(b.Close)
				key, _, err := profiledump.NewObjectKey("a", time.Now().Add(-8*24*time.Hour), profiledump.FormatPprof)
				require.NoError(t, err)
				require.NoError(t, b.Upload(context.Background(), key, strings.NewReader("opaque")))
				mm := modules.NewManager(log.NewNopLogger())
				mm.RegisterModule(Storage, func() (services.Service, error) {
					return services.NewIdleService(nil, func(error) error { return f.stopStorage() }), nil
				})
				mm.RegisterModule(ProfileDumpCleaner, f.initProfileDumpCleaner)
				mm.RegisterModule(ProfileDumpRecorder, f.initProfileDumpRecorder)
				mm.RegisterModule(Admin, func() (services.Service, error) { return services.NewIdleService(nil, nil), nil })
				mm.RegisterModule("combined", nil)
				require.NoError(t, mm.AddDependency(ProfileDumpCleaner, Storage))
				require.NoError(t, mm.AddDependency(ProfileDumpRecorder, Storage))
				require.NoError(t, mm.AddDependency(Admin, ProfileDumpCleaner))
				require.NoError(t, mm.AddDependency("combined", Admin, ProfileDumpRecorder))
				failure := errors.New("synthetic service failure")
				selectedTarget := target
				if failureMode != "stop" {
					mm.RegisterModule("failure", func() (services.Service, error) {
						fail := func(context.Context) error { <-b.entered; return failure }
						if failureMode == "startup" {
							return services.NewIdleService(fail, nil), nil
						}
						return services.NewBasicService(nil, fail, nil), nil
					})
					require.NoError(t, mm.AddDependency("failure", target))
					selectedTarget = "failure"
				}
				serviceMap, err := mm.InitModuleServices(selectedTarget)
				require.NoError(t, err)
				all := make([]services.Service, 0, len(serviceMap))
				for _, svc := range serviceMap {
					all = append(all, svc)
				}
				manager, err := services.NewManager(all...)
				require.NoError(t, err)
				manager.AddListener(services.NewManagerListener(nil, nil, func(services.Service) { manager.StopAsync() }))
				var release sync.Once
				t.Cleanup(func() {
					release.Do(func() { close(b.release) })
					manager.StopAsync()
					require.NoError(t, manager.AwaitStopped(context.Background()))
					require.NoError(t, f.stopStorage())
				})
				require.NoError(t, manager.StartAsync(context.Background()))
				<-b.entered
				if failureMode == "stop" {
					manager.StopAsync()
				}
				<-b.canceled
				require.Zero(t, b.closes.Load())
				require.EqualValues(t, 1, b.active.Load())
				// The initialization-failure fallback also waits on the same borrower.
				stopped := make(chan error, 1)
				go func() { stopped <- f.stopStorage() }()
				select {
				case <-stopped:
					t.Fatal("storage teardown passed active cleanup")
				default:
				}
				release.Do(func() { close(b.release) })
				require.NoError(t, manager.AwaitStopped(context.Background()))
				require.NoError(t, <-stopped)
				require.EqualValues(t, 1, b.closes.Load())
				require.NoError(t, f.stopStorage())
				require.EqualValues(t, 1, b.closes.Load())
				if f.profileDumpRecorder != nil {
					<-f.profileDumpRecorder.Done()
				}
			})
		}
	}
}

func TestProfileDumpCleanerInitializationFailure(t *testing.T) {
	f, bucket := newDumpApplication(t)
	mm := modules.NewManager(log.NewNopLogger())
	mm.RegisterModule(ProfileDumpCleaner, f.initProfileDumpCleaner)
	mm.RegisterModule(ProfileDumpRecorder, f.initProfileDumpRecorder)
	failure := errors.New("constructor failed")
	mm.RegisterModule("failure", func() (services.Service, error) { return nil, failure })
	require.NoError(t, mm.AddDependency("failure", ProfileDumpCleaner, ProfileDumpRecorder))
	f.ModuleManager, f.Cfg.Target = mm, []string{"failure"}
	require.ErrorIs(t, f.Run(), failure)
	require.Equal(t, services.Terminated, f.profileDumpCleaner.State())
	<-f.profileDumpRecorder.Done()
	require.EqualValues(t, 1, bucket.closes.Load())
	require.NoError(t, f.stopStorage())
	require.EqualValues(t, 1, bucket.closes.Load())
}

func TestProfileDumpCleanerConfiguration(t *testing.T) {
	cfg := newTestConfig(t, []string{"-profile-dump.retention=24h", "-profile-dump.cleanup-max-entries=17"})
	require.Equal(t, 24*time.Hour, cfg.ProfileDump.Cleaner.Retention)
	require.Equal(t, 17, cfg.ProfileDump.Cleaner.MaxEntries)
	cfg.ProfileDump.Cleaner.Retention = 0
	require.ErrorContains(t, cfg.Validate(), "profile_dump cleaner")
	f, _ := newDumpApplication(t)
	f.storageBucket = nil
	svc, err := f.initProfileDumpCleaner()
	require.NoError(t, err)
	require.Nil(t, svc)
	require.Nil(t, f.profileDumpCleaner)
}
