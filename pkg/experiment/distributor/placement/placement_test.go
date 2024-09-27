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
				{Id: "a", State: ring.ACTIVE},
				{Id: "b", State: ring.ACTIVE},
				{Id: "c", State: ring.ACTIVE},
			},
			expected: []ring.InstanceDesc{
				{Id: "a", State: ring.ACTIVE},
				{Id: "b", State: ring.ACTIVE},
				{Id: "c", State: ring.ACTIVE},
			},
		},
		{
			description: "active duplicate",
			instances: []ring.InstanceDesc{
				{Id: "a", State: ring.ACTIVE},
				{Id: "a", State: ring.ACTIVE},
			},
			expected: []ring.InstanceDesc{
				{Id: "a", State: ring.ACTIVE},
			},
		},
		{
			description: "non-active duplicate",
			instances: []ring.InstanceDesc{
				{Id: "a", State: ring.PENDING},
				{Id: "a", State: ring.PENDING},
			},
		},
		{
			description: "mixed",
			instances: []ring.InstanceDesc{
				{Id: "a", State: ring.PENDING},
				{Id: "b", State: ring.ACTIVE},
				{Id: "c", State: ring.JOINING},
				{Id: "d", State: ring.LEAVING},
				{Id: "c", State: ring.JOINING},
				{Id: "e", State: ring.ACTIVE},
				{Id: "e", State: ring.ACTIVE},
			},
			expected: []ring.InstanceDesc{
				{Id: "b", State: ring.ACTIVE},
				{Id: "e", State: ring.ACTIVE},
			},
		},
	} {
		active := ActiveInstances(iter.NewSliceIterator(test.instances))
		assert.Equal(t, test.expected, iter.MustSlice(active))
	}
}
