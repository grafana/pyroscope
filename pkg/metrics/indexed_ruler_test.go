package metrics

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/services"
	"github.com/grafana/dskit/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
	settingsrecording "github.com/grafana/pyroscope/v2/pkg/settings/recording"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

type spyBucket struct {
	objstore.Bucket

	mu   sync.Mutex
	gets map[string]int
	fail func(name string) error
}

func newSpyBucket() *spyBucket {
	return &spyBucket{Bucket: objstore.NewInMemBucket(), gets: map[string]int{}}
}

func (b *spyBucket) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	b.mu.Lock()
	b.gets[name]++
	fail := b.fail
	b.mu.Unlock()
	if fail != nil {
		if err := fail(name); err != nil {
			return nil, err
		}
	}
	return b.Bucket.Get(ctx, name)
}

func (b *spyBucket) setFail(fn func(name string) error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fail = fn
}

func (b *spyBucket) getsOf(prefix string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for name, c := range b.gets {
		if strings.HasPrefix(name, prefix) {
			n += c
		}
	}
	return n
}

func failTenantFiles(name string) error {
	if strings.Contains(name, "/settings/") {
		return errors.New("unavailable")
	}
	return nil
}

type rulerEnv struct {
	bucket    *spyBucket
	settings  *settingsrecording.RecordingRules
	overrides *validation.Overrides
	ruler     *IndexedRuler
}

func newRulerEnv(t *testing.T, overrides func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits)) *rulerEnv {
	t.Helper()
	if overrides == nil {
		overrides = func(*validation.Limits, map[string]*validation.Limits) {}
	}
	env := &rulerEnv{
		bucket:    newSpyBucket(),
		overrides: validation.MockOverrides(overrides),
	}
	env.settings = settingsrecording.New(env.bucket, log.NewNopLogger(), env.overrides)
	env.ruler = NewIndexedRuler(
		IndexedRulerConfig{RefreshInterval: time.Hour, FetchTimeout: time.Second},
		env.bucket, env.overrides, log.NewNopLogger(), nil,
	)
	env.ruler.retry = 50 * time.Millisecond
	return env
}

func (e *rulerEnv) upsert(t *testing.T, tenant, metric, function string) string {
	t.Helper()
	req := &settingsv1.UpsertRecordingRuleRequest{
		MetricName: "profiles_recorded_" + metric,
		Matchers:   []string{`{__profile_type__="process_cpu:cpu:nanoseconds:cpu:nanoseconds"}`},
		GroupBy:    []string{"region"},
	}
	if function != "" {
		req.StacktraceFilter = &settingsv1.StacktraceFilter{
			FunctionName: &settingsv1.StacktraceFilterFunctionName{FunctionName: function},
		}
	}
	resp, err := e.settings.UpsertRecordingRule(user.InjectOrgID(context.Background(), tenant), connect.NewRequest(req))
	require.NoError(t, err)
	return resp.Msg.Rule.Id
}

func (e *rulerEnv) deleteRule(t *testing.T, tenant, id string) {
	t.Helper()
	_, err := e.settings.DeleteRecordingRule(user.InjectOrgID(context.Background(), tenant), connect.NewRequest(&settingsv1.DeleteRecordingRuleRequest{Id: id}))
	require.NoError(t, err)
}

func (e *rulerEnv) rebuildIndex(t *testing.T) {
	t.Helper()
	require.NoError(t, settingsrecording.NewIndexBuilder(e.bucket, log.NewNopLogger(), 0).Rebuild(context.Background()))
}

func (e *rulerEnv) start(t *testing.T) {
	t.Helper()
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), e.ruler))
	t.Cleanup(func() { _ = services.StopAndAwaitTerminated(context.Background(), e.ruler) })
}

func ruleIDs(rules []*model.RecordingRule) []string {
	ids := make([]string, len(rules))
	for i, r := range rules {
		ids[i] = r.ExternalLabels.Get(model.RuleIDLabel)
	}
	return ids
}

func mustRules(t *testing.T, r *IndexedRuler, tenant string) []*model.RecordingRule {
	t.Helper()
	rules, err := r.RecordingRules(tenant)
	require.NoError(t, err)
	return rules
}

func TestIndexedRuler_TenantsOutsideTheIndexDoNoIO(t *testing.T) {
	env := newRulerEnv(t, nil)
	env.upsert(t, "tenant-a", "a", "")
	env.rebuildIndex(t)
	env.start(t)

	before := env.bucket.getsOf("")
	for range 100 {
		rules, err := env.ruler.RecordingRules("tenant-z")
		require.NoError(t, err)
		assert.Empty(t, rules)
	}
	assert.Equal(t, before, env.bucket.getsOf(""))
}

func TestIndexedRuler_PrefetchesIndexedTenants(t *testing.T) {
	env := newRulerEnv(t, nil)
	id := env.upsert(t, "tenant-a", "a", "handle")
	env.upsert(t, "tenant-b", "b", "")
	env.rebuildIndex(t)
	env.start(t)

	fetched := env.bucket.getsOf("tenant-")
	rules := mustRules(t, env.ruler, "tenant-a")
	require.Len(t, rules, 1)
	assert.Equal(t, []string{id}, ruleIDs(rules))
	assert.Equal(t, "handle", rules[0].FunctionName)
	assert.Len(t, mustRules(t, env.ruler, "tenant-b"), 1)
	assert.Equal(t, fetched, env.bucket.getsOf("tenant-"))
}

func TestIndexedRuler_MergesConfigRules(t *testing.T) {
	configRule := &settingsv1.RecordingRule{
		Id:          "provisioned",
		MetricName:  "profiles_recorded_provisioned",
		ProfileType: "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
		Matchers:    []string{`{__profile_type__="process_cpu:cpu:nanoseconds:cpu:nanoseconds"}`},
	}
	env := newRulerEnv(t, func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {
		l := validation.MockDefaultLimits()
		l.RecordingRules = validation.RecordingRules{configRule}
		tenantLimits["tenant-a"] = l
		tenantLimits["tenant-config-only"] = l
	})
	id := env.upsert(t, "tenant-a", "a", "")
	env.rebuildIndex(t)
	env.start(t)

	assert.Equal(t, []string{"provisioned", id}, ruleIDs(mustRules(t, env.ruler, "tenant-a")))
	assert.Equal(t, []string{"provisioned"}, ruleIDs(mustRules(t, env.ruler, "tenant-config-only")))
}

func TestIndexedRuler_ParsedRulesAreReused(t *testing.T) {
	env := newRulerEnv(t, nil)
	env.upsert(t, "tenant-a", "a", "")
	env.rebuildIndex(t)
	env.start(t)

	first := mustRules(t, env.ruler, "tenant-a")
	second := mustRules(t, env.ruler, "tenant-a")
	require.Len(t, first, 1)
	assert.Same(t, first[0], second[0])
}

func TestIndexedRuler_PicksUpChangesOnRefresh(t *testing.T) {
	env := newRulerEnv(t, nil)
	first := env.upsert(t, "tenant-a", "a", "")
	env.rebuildIndex(t)
	env.start(t)
	require.Equal(t, []string{first}, ruleIDs(mustRules(t, env.ruler, "tenant-a")))

	second := env.upsert(t, "tenant-a", "b", "")
	env.upsert(t, "tenant-c", "c", "")
	env.rebuildIndex(t)
	require.NoError(t, env.ruler.refresh(context.Background()))

	assert.ElementsMatch(t, []string{first, second}, ruleIDs(mustRules(t, env.ruler, "tenant-a")))
	assert.Len(t, mustRules(t, env.ruler, "tenant-c"), 1)

	env.deleteRule(t, "tenant-a", first)
	env.deleteRule(t, "tenant-a", second)
	env.rebuildIndex(t)
	require.NoError(t, env.ruler.refresh(context.Background()))
	assert.Empty(t, mustRules(t, env.ruler, "tenant-a"))
}

func TestIndexedRuler_FetchesOutdatedRulesSynchronously(t *testing.T) {
	env := newRulerEnv(t, nil)
	first := env.upsert(t, "tenant-a", "a", "")
	env.rebuildIndex(t)
	env.start(t)
	require.Equal(t, []string{first}, ruleIDs(mustRules(t, env.ruler, "tenant-a")))

	second := env.upsert(t, "tenant-a", "b", "")
	stored, err := settingsrecording.ReadStoredRules(context.Background(), env.bucket, log.NewNopLogger(), "tenant-a")
	require.NoError(t, err)
	env.ruler.index.Store(&settingsrecording.Index{Tenants: map[string]int64{"tenant-a": stored.Generation}})

	assert.ElementsMatch(t, []string{first, second}, ruleIDs(mustRules(t, env.ruler, "tenant-a")))
}

func TestIndexedRuler_FetchesTenantsMissingFromTheCache(t *testing.T) {
	env := newRulerEnv(t, nil)
	env.upsert(t, "tenant-a", "a", "")
	env.rebuildIndex(t)
	env.bucket.setFail(failTenantFiles)
	env.start(t)
	env.bucket.setFail(nil)

	time.Sleep(env.ruler.retry + 10*time.Millisecond)
	assert.Len(t, mustRules(t, env.ruler, "tenant-a"), 1)
}

func TestIndexedRuler_UnreadableRules(t *testing.T) {
	t.Run("without a previous version it fails and backs off", func(t *testing.T) {
		env := newRulerEnv(t, nil)
		env.upsert(t, "tenant-a", "a", "")
		env.rebuildIndex(t)
		env.bucket.setFail(failTenantFiles)
		env.start(t)

		before := env.bucket.getsOf("tenant-a/")
		for range 20 {
			_, err := env.ruler.RecordingRules("tenant-a")
			require.Error(t, err)
		}
		assert.Equal(t, before, env.bucket.getsOf("tenant-a/"))

		env.bucket.setFail(nil)
		require.Eventually(t, func() bool {
			rules, err := env.ruler.RecordingRules("tenant-a")
			return err == nil && len(rules) == 1
		}, time.Second, 10*time.Millisecond)
	})

	t.Run("with a previous version it keeps serving it", func(t *testing.T) {
		env := newRulerEnv(t, nil)
		first := env.upsert(t, "tenant-a", "a", "")
		env.rebuildIndex(t)
		env.start(t)

		env.upsert(t, "tenant-a", "b", "")
		stored, err := settingsrecording.ReadStoredRules(context.Background(), env.bucket, log.NewNopLogger(), "tenant-a")
		require.NoError(t, err)
		env.ruler.index.Store(&settingsrecording.Index{Tenants: map[string]int64{"tenant-a": stored.Generation}})
		env.bucket.setFail(failTenantFiles)

		rules, err := env.ruler.RecordingRules("tenant-a")
		require.NoError(t, err)
		assert.Equal(t, []string{first}, ruleIDs(rules))
	})
}

func TestIndexedRuler_ConfigRulesSurviveWhenTheIndexIsEmpty(t *testing.T) {
	configRule := &settingsv1.RecordingRule{
		MetricName:  "profiles_recorded_provisioned",
		ProfileType: "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
		Matchers:    []string{`{__profile_type__="process_cpu:cpu:nanoseconds:cpu:nanoseconds"}`},
	}
	env := newRulerEnv(t, func(defaults *validation.Limits, _ map[string]*validation.Limits) {
		defaults.RecordingRules = validation.RecordingRules{configRule}
	})
	env.start(t)

	assert.Len(t, mustRules(t, env.ruler, "any-tenant"), 1)
}

func TestIndexedRuler_StartingFailsWhenTheIndexCannotBeLoaded(t *testing.T) {
	env := newRulerEnv(t, nil)
	env.bucket.setFail(func(name string) error {
		if name == settingsrecording.IndexPath {
			return errors.New("unavailable")
		}
		return nil
	})

	require.Error(t, services.StartAndAwaitRunning(context.Background(), env.ruler))
}

func TestIndexedRuler_StartsWithoutAnIndex(t *testing.T) {
	env := newRulerEnv(t, nil)
	env.start(t)

	assert.Empty(t, mustRules(t, env.ruler, "tenant-a"))
}

func TestIndexedRuler_IgnoresOutdatedIndex(t *testing.T) {
	env := newRulerEnv(t, nil)
	env.upsert(t, "tenant-a", "a", "")
	env.rebuildIndex(t)
	env.start(t)
	require.Len(t, mustRules(t, env.ruler, "tenant-a"), 1)

	outdated := &settingsrecording.Index{GeneratedAt: 1, Tenants: map[string]int64{}}
	require.NoError(t, settingsrecording.NewIndexStore(env.bucket).Store(context.Background(), outdated))
	require.NoError(t, env.ruler.refresh(context.Background()))

	assert.Len(t, mustRules(t, env.ruler, "tenant-a"), 1)
}
