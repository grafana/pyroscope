package distributor

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/distributor/inflight"
)

func TestAggregateReconcile(t *testing.T) {
	for _, growing := range []bool{false, true} {
		t.Run(fmt.Sprintf("growing=%v", growing), func(t *testing.T) {
			d := &Distributor{inflight: inflight.NewLimiter(0, nil)}
			var a, out *pendingAggregate
			var expected int64
			for i := 1; i < 3*aggregateReservationReconcileInterval; i++ {
				id := 0
				if growing {
					id = i
				}
				p := distinctStackRequest(id).Series[0].Profile.Profile
				size := int64(p.SizeVT())
				var err error
				a, err = d.mergeProfile(p, size, &out)(a)
				require.NoError(t, err)
				require.Same(t, a, out)
				expected += size
				if i%aggregateReservationReconcileInterval == 0 {
					expected = int64(a.merge.Profile().SizeVT())
				}
				require.Equal(t, expected, d.inflight.Bytes(), "addition %d", i)
			}
			a.release()
			a.release()
			a.reconcile()
			a.charge(d.inflight, 100)
			require.Zero(t, d.inflight.Bytes(), "released aggregate cannot be recharged")
		})
	}
}

func TestAggregateReconcileMergeError(t *testing.T) {
	d := &Distributor{inflight: inflight.NewLimiter(0, nil)}
	var a, out *pendingAggregate
	for i := 0; i < aggregateReservationReconcileInterval-1; i++ {
		p := distinctStackRequest(0).Series[0].Profile.Profile
		var err error
		a, err = d.mergeProfile(p, int64(p.SizeVT()), &out)(a)
		require.NoError(t, err)
	}
	p := distinctStackRequest(0).Series[0].Profile.Profile
	p.StringTable[1] = "incompatible-type"
	var err error
	a, err = d.mergeProfile(p, int64(p.SizeVT()), &out)(a)
	require.Error(t, err)
	require.Equal(t, aggregateReservationReconcileInterval-1, a.additions, "failed merge does not trigger reconciliation")
	out.release()
	require.Zero(t, d.inflight.Bytes())
}
