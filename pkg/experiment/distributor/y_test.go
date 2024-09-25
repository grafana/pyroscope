package distributor

import (
	"fmt"
	"math/rand"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/grafana/dskit/ring"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement"
	"github.com/grafana/pyroscope/pkg/testhelper"
)

func Test_Y(t *testing.T) {
	//	t.Skip()

	s := make(map[uint32]int)
	d := NewDistributor(defaultPlacement{})
	d.RingUpdateInterval = time.Hour

	r := testhelper.NewMockRing([]ring.InstanceDesc{
		{Addr: "a", Tokens: make([]uint32, 4)},
		{Addr: "b", Tokens: make([]uint32, 4)},
		{Addr: "c", Tokens: make([]uint32, 4)},
		{Addr: "d", Tokens: make([]uint32, 4)},
		{Addr: "e", Tokens: make([]uint32, 4)},
		{Addr: "f", Tokens: make([]uint32, 4)},
		{Addr: "g", Tokens: make([]uint32, 4)},
		{Addr: "h", Tokens: make([]uint32, 4)},

		//	{Addr: "j", Tokens: make([]uint32, 4)},
		//	{Addr: "h", Tokens: make([]uint32, 4)},
		//	{Addr: "i", Tokens: make([]uint32, 4)},
		//	{Addr: "k", Tokens: make([]uint32, 4)},
	}, 1)

	locs := make(map[string]int)
	instances := make(map[string]int)

	var total int
	const N = 128 << 10
	rnd := rand.New(rand.NewSource(randSeed))
	for i := 1; i < N; i++ {
		k := NewTenantServiceDatasetKey("tenant-a", []*typesv1.LabelPair{
			{Name: "service_name", Value: strconv.Itoa(rnd.Intn(16))},
			{Name: "label_name", Value: strconv.Itoa(i)},
		}...)
		p, err := d.Distribute(k, r)
		require.NoError(t, err)
		s[p.Shard]++
		total++

		p.Instances.Next()
		locs[fmt.Sprintf("%02d-%s", p.Shard, p.Instances.At().Addr)]++
		instances[p.Instances.At().Addr]++
	}

	var keys []uint32
	for k := range s {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	values := make([][2]int, len(keys))
	for i, k := range keys {
		values[i] = [2]int{int(k), s[k]}
	}

	for _, v := range values {
		p := 100 * float64(v[1]) / float64(total)
		t.Logf("%4d | %6.1f%%: %v\n", v[0], p, strings.Repeat("x", int(p)))
		// t.Logf("%4d | %3d | %6.1f%%: %v\n", v[0], v[0]%M, p, strings.Repeat("x", int(p)))
	}
	t.Log(">", len(values), strings.Repeat("-", 80))

	lk := lo.Keys(locs)
	sort.Strings(lk)
	for _, k := range lk {
		v := locs[k]
		p := 100 * float64(v) / float64(total)
		t.Logf("%4s | %6.1f%%: %v\n", k, p, strings.Repeat("x", int(p)))
	}
	t.Log(">", len(locs), strings.Repeat("-", 80))

	lk = lo.Keys(instances)
	sort.Strings(lk)
	for _, k := range lk {
		v := instances[k]
		p := 100 * float64(v) / float64(total)
		t.Logf("%4s | %6.1f%%: %v\n", k, p, strings.Repeat("x", int(p)))
	}
	t.Log(">", len(instances), strings.Repeat("-", 80))
}

type defaultPlacement struct{}

func (defaultPlacement) Place(placement.Key) *placement.Placement { return nil }

func (defaultPlacement) NumTenantShards(placement.Key, int) int { return 0 }

func (defaultPlacement) NumDatasetShards(placement.Key, int) int { return 7 }

func (defaultPlacement) PickShard(k placement.Key, n int) int {
	return int(k.Fingerprint % uint64(n))
}
