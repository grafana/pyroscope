package metrics

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/model"
)

type fakeRulesClient struct {
	calls atomic.Int32
	fn    func(call int32) ([]*model.RecordingRule, error)
}

func (f *fakeRulesClient) RecordingRules(string) ([]*model.RecordingRule, error) {
	return f.fn(f.calls.Add(1))
}

func newTestRuler(c RecordingRulesClient, expiry, retry time.Duration) *CachedRemoteRuler {
	r, _ := NewCachedRemoteRuler(c, log.NewNopLogger())
	cr := r.(*CachedRemoteRuler)
	cr.expiry = expiry
	cr.retry = retry
	return cr
}

func TestCachedRemoteRuler_FirstLoadIsSingleFlight(t *testing.T) {
	release := make(chan struct{})
	client := &fakeRulesClient{fn: func(int32) ([]*model.RecordingRule, error) {
		<-release
		return defaultRecordingRules, nil
	}}
	ruler := newTestRuler(client, time.Hour, time.Hour)

	var wg sync.WaitGroup
	results := make([][]*model.RecordingRule, 8)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = rulesOf(t, ruler)
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	assert.EqualValues(t, 1, client.calls.Load())
	for _, r := range results {
		assert.Equal(t, defaultRecordingRules, r)
	}
}

func TestCachedRemoteRuler_ServesStaleWhileRefreshing(t *testing.T) {
	release := make(chan struct{})
	client := &fakeRulesClient{fn: func(call int32) ([]*model.RecordingRule, error) {
		if call == 1 {
			return defaultRecordingRules, nil
		}
		<-release
		return overriddenRecordingRules, nil
	}}
	ruler := newTestRuler(client, 10*time.Millisecond, time.Hour)

	require.Equal(t, defaultRecordingRules, rulesOf(t, ruler))
	time.Sleep(20 * time.Millisecond)

	for range 5 {
		assert.Equal(t, defaultRecordingRules, rulesOf(t, ruler))
	}
	require.Eventually(t, func() bool { return client.calls.Load() == 2 }, time.Second, 5*time.Millisecond)

	close(release)
	require.Eventually(t, func() bool {
		rules, err := ruler.RecordingRules("t")
		return err == nil && assert.ObjectsAreEqual(overriddenRecordingRules, rules)
	}, time.Second, 5*time.Millisecond)
	assert.EqualValues(t, 2, client.calls.Load())
}

func TestCachedRemoteRuler_FailureBacksOff(t *testing.T) {
	client := &fakeRulesClient{fn: func(int32) ([]*model.RecordingRule, error) {
		return nil, errors.New("unavailable")
	}}
	ruler := newTestRuler(client, time.Hour, time.Hour)

	for range 5 {
		rules, err := ruler.RecordingRules("t")
		assert.Nil(t, rules)
		assert.Error(t, err)
	}
	assert.EqualValues(t, 1, client.calls.Load())
}

func TestCachedRemoteRuler_RefreshFailureKeepsStaleValue(t *testing.T) {
	client := &fakeRulesClient{fn: func(call int32) ([]*model.RecordingRule, error) {
		if call == 1 {
			return defaultRecordingRules, nil
		}
		return nil, errors.New("unavailable")
	}}
	ruler := newTestRuler(client, 10*time.Millisecond, time.Hour)

	require.Equal(t, defaultRecordingRules, rulesOf(t, ruler))
	time.Sleep(20 * time.Millisecond)

	assert.Equal(t, defaultRecordingRules, rulesOf(t, ruler))
	require.Eventually(t, func() bool { return client.calls.Load() == 2 }, time.Second, 5*time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	for range 5 {
		assert.Equal(t, defaultRecordingRules, rulesOf(t, ruler))
	}
	assert.EqualValues(t, 2, client.calls.Load())
}

func TestCachedRemoteRuler_RecoversAfterFailure(t *testing.T) {
	fail := atomic.Bool{}
	fail.Store(true)
	client := &fakeRulesClient{fn: func(int32) ([]*model.RecordingRule, error) {
		if fail.Load() {
			return nil, errors.New("unavailable")
		}
		return defaultRecordingRules, nil
	}}
	ruler := newTestRuler(client, time.Hour, 10*time.Millisecond)

	_, err := ruler.RecordingRules("t")
	assert.Error(t, err)

	fail.Store(false)
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, defaultRecordingRules, rulesOf(t, ruler))
}

func rulesOf(t *testing.T, r Ruler) []*model.RecordingRule {
	t.Helper()
	rules, err := r.RecordingRules("t")
	require.NoError(t, err)
	return rules
}
