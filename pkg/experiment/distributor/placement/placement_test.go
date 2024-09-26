package placement

import (
	"testing"

	"github.com/grafana/dskit/ring"
	"github.com/stretchr/testify/assert"

	"github.com/grafana/pyroscope/pkg/iter"
)

func Test_ActiveInstances(t *testing.T) {
	for _, test := range []struct {
		description string
		instances   []ring.InstanceDesc
		expected    []ring.InstanceDesc
	}{
		{
			description: "empty",
		},
		{
			description: "all active",
			instances: []ring.InstanceDesc{
				{Addr: "a", State: ring.ACTIVE},
				{Addr: "b", State: ring.ACTIVE},
				{Addr: "c", State: ring.ACTIVE},
			},
			expected: []ring.InstanceDesc{
				{Addr: "a", State: ring.ACTIVE},
				{Addr: "b", State: ring.ACTIVE},
				{Addr: "c", State: ring.ACTIVE},
			},
		},
		{
			description: "active duplicate",
			instances: []ring.InstanceDesc{
				{Addr: "a", State: ring.ACTIVE},
				{Addr: "a", State: ring.ACTIVE},
			},
			expected: []ring.InstanceDesc{
				{Addr: "a", State: ring.ACTIVE},
			},
		},
		{
			description: "non-active duplicate",
			instances: []ring.InstanceDesc{
				{Addr: "a", State: ring.PENDING},
				{Addr: "a", State: ring.PENDING},
			},
		},
		{
			description: "mixed",
			instances: []ring.InstanceDesc{
				{Addr: "a", State: ring.PENDING},
				{Addr: "b", State: ring.ACTIVE},
				{Addr: "c", State: ring.JOINING},
				{Addr: "d", State: ring.LEAVING},
				{Addr: "c", State: ring.JOINING},
				{Addr: "e", State: ring.ACTIVE},
				{Addr: "e", State: ring.ACTIVE},
			},
			expected: []ring.InstanceDesc{
				{Addr: "b", State: ring.ACTIVE},
				{Addr: "e", State: ring.ACTIVE},
			},
		},
	} {
		active := ActiveInstances(iter.NewSliceIterator(test.instances))
		assert.Equal(t, test.expected, iter.MustSlice(active))
	}
}
