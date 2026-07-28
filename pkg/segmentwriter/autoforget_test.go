package segmentwriter

import (
	"context"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/kv/consul"
	"github.com/grafana/dskit/ring"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setInstanceHeartbeat(t *testing.T, kvStore *consul.Client, id string, ts time.Time) {
	t.Helper()
	require.NoError(t, kvStore.CAS(context.Background(), RingKey, func(in any) (out any, retry bool, err error) {
		desc := in.(*ring.Desc)
		instance := desc.Ingesters[id]
		instance.Timestamp = ts.Unix()
		desc.Ingesters[id] = instance
		return desc, true, nil
	}))
}

func newTestRingDesc(t *testing.T, kvStore *consul.Client, ids ...string) {
	t.Helper()
	require.NoError(t, kvStore.CAS(context.Background(), RingKey, func(in any) (out any, retry bool, err error) {
		desc := ring.NewDesc()
		for i, id := range ids {
			desc.AddIngester(id, "127.0.0.1", "", []uint32{uint32(i)}, ring.ACTIVE, time.Now(), false, time.Now(), ring.InstanceVersions{})
		}
		return desc, true, nil
	}))
}

func ringInstanceIDs(t *testing.T, kvStore *consul.Client) []string {
	t.Helper()
	v, err := kvStore.Get(context.Background(), RingKey)
	require.NoError(t, err)
	desc, ok := v.(*ring.Desc)
	require.True(t, ok)
	ids := make([]string, 0, len(desc.Ingesters))
	for id := range desc.Ingesters {
		ids = append(ids, id)
	}
	return ids
}

func TestAutoForget_removesStaleInstances(t *testing.T) {
	kvStore, closer := consul.NewInMemoryClient(ring.GetCodec(), log.NewNopLogger(), nil)
	t.Cleanup(func() { assert.NoError(t, closer.Close()) })

	const forgetPeriod = 4 * time.Minute
	newTestRingDesc(t, kvStore, "self", "fresh", "stale")
	setInstanceHeartbeat(t, kvStore, "stale", time.Now().Add(-forgetPeriod-time.Minute))

	f := newAutoForget(kvStore, "self", forgetPeriod, time.Minute, log.NewNopLogger())
	require.NoError(t, f.iteration(context.Background()))

	assert.ElementsMatch(t, []string{"self", "fresh"}, ringInstanceIDs(t, kvStore))
}

func TestAutoForget_skipsRoundWhenOwnHeartbeatIsStale(t *testing.T) {
	kvStore, closer := consul.NewInMemoryClient(ring.GetCodec(), log.NewNopLogger(), nil)
	t.Cleanup(func() { assert.NoError(t, closer.Close()) })

	const forgetPeriod = 4 * time.Minute
	newTestRingDesc(t, kvStore, "self", "stale")
	setInstanceHeartbeat(t, kvStore, "self", time.Now().Add(-forgetPeriod-time.Minute))
	setInstanceHeartbeat(t, kvStore, "stale", time.Now().Add(-forgetPeriod-time.Minute))

	f := newAutoForget(kvStore, "self", forgetPeriod, time.Minute, log.NewNopLogger())
	require.NoError(t, f.iteration(context.Background()))

	assert.ElementsMatch(t, []string{"self", "stale"}, ringInstanceIDs(t, kvStore))
}

func TestAutoForget_noRing(t *testing.T) {
	kvStore, closer := consul.NewInMemoryClient(ring.GetCodec(), log.NewNopLogger(), nil)
	t.Cleanup(func() { assert.NoError(t, closer.Close()) })

	f := newAutoForget(kvStore, "self", 4*time.Minute, time.Minute, log.NewNopLogger())
	require.NoError(t, f.iteration(context.Background()))
}
