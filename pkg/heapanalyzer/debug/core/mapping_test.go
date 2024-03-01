package core

import (
	"reflect"
	"testing"
)

func TestSplicedMemoryAdd(t *testing.T) {
	type region struct {
		min, max Address
		perm     Perm
		off      int64
	}
	tests := []struct {
		name string
		in   []region
		want []region
	}{
		{
			"Insert after",
			[]region{
				{min: 0 * pageSize, max: 1 * pageSize, off: 1},
				{min: 1 * pageSize, max: 2 * pageSize, off: 2},
			},
			[]region{
				{min: 0 * pageSize, max: 1 * pageSize, off: 1},
				{min: 1 * pageSize, max: 2 * pageSize, off: 2},
			},
		},
		{
			"Insert before",
			[]region{
				{min: 1 * pageSize, max: 2 * pageSize, off: 1},
				{min: 0 * pageSize, max: 1 * pageSize, off: 2},
			},
			[]region{
				{min: 0 * pageSize, max: 1 * pageSize, off: 2},
				{min: 1 * pageSize, max: 2 * pageSize, off: 1},
			},
		},
		{
			"Completely overwrite",
			[]region{
				{min: 1 * pageSize, max: 2 * pageSize, perm: Read, off: 2},
				{min: 1 * pageSize, max: 2 * pageSize, perm: Write, off: 1},
			},
			[]region{
				{min: 1 * pageSize, max: 2 * pageSize, perm: Write, off: 1},
			},
		},
		{
			"Overwrite end",
			[]region{
				{min: 0 * pageSize, max: 2 * pageSize, perm: Read, off: 0},
				{min: 1 * pageSize, max: 2 * pageSize, perm: Write, off: 1},
			},
			[]region{
				{min: 0 * pageSize, max: 1 * pageSize, perm: Read, off: 0},
				{min: 1 * pageSize, max: 2 * pageSize, perm: Write, off: 1},
			},
		},
		{
			"Overwrite start",
			[]region{
				{min: 0 * pageSize, max: 2 * pageSize, perm: Read, off: 0},
				{min: 0 * pageSize, max: 1 * pageSize, perm: Write, off: 1},
			},
			[]region{
				{min: 0 * pageSize, max: 1 * pageSize, perm: Write, off: 1},
				{min: 1 * pageSize, max: 2 * pageSize, perm: Read, off: int64(1 * pageSize)},
			},
		},
		{
			"Punch hole",
			[]region{
				{min: 10 * pageSize, max: 30 * pageSize, perm: Read, off: 2},
				{min: 15 * pageSize, max: 25 * pageSize, perm: Write, off: 1},
			},
			[]region{
				{min: 10 * pageSize, max: 15 * pageSize, perm: Read, off: 2},
				{min: 15 * pageSize, max: 25 * pageSize, perm: Write, off: 1},
				{min: 25 * pageSize, max: 30 * pageSize, perm: Read, off: int64(2 + 15*pageSize)},
			},
		},
		{
			"Overlap two",
			[]region{
				{min: 10 * pageSize, max: 14 * pageSize, perm: Read, off: 1},
				{min: 14 * pageSize, max: 18 * pageSize, perm: Write, off: 2},
				{min: 12 * pageSize, max: 16 * pageSize, perm: Exec, off: 3},
			},
			[]region{
				{min: 10 * pageSize, max: 12 * pageSize, perm: Read, off: 1},
				{min: 12 * pageSize, max: 16 * pageSize, perm: Exec, off: 3},
				{min: 16 * pageSize, max: 18 * pageSize, perm: Write, off: int64(2 + 2*pageSize)},
			},
		},
		{
			"Align max",
			[]region{
				{min: 10 * pageSize, max: 14*pageSize - 1, perm: Read, off: 1},
				{min: 14 * pageSize, max: 18*pageSize - 1, perm: Write, off: 2},
				{min: 12 * pageSize, max: 16*pageSize - 1, perm: Exec, off: 3},
			},
			[]region{
				{min: 10 * pageSize, max: 12 * pageSize, perm: Read, off: 1},
				{min: 12 * pageSize, max: 16 * pageSize, perm: Exec, off: 3},
				{min: 16 * pageSize, max: 18 * pageSize, perm: Write, off: int64(2 + 2*pageSize)},
			},
		},
		{
			"Align min",
			[]region{
				{min: 10*pageSize + 1, max: 14 * pageSize, perm: Read, off: 1},
				{min: 14*pageSize + 2, max: 18 * pageSize, perm: Write, off: 2},
				{min: 12*pageSize + 3, max: 16 * pageSize, perm: Exec, off: 3},
			},
			[]region{
				{min: 10 * pageSize, max: 12 * pageSize, perm: Read, off: 0},
				{min: 12 * pageSize, max: 16 * pageSize, perm: Exec, off: 0},
				{min: 16 * pageSize, max: 18 * pageSize, perm: Write, off: int64(2 * pageSize)},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mem := &splicedMemory{}
			for _, in := range test.in {
				mem.Add(in.min, in.max, in.perm, nil, in.off)
			}
			var got []region
			for _, m := range mem.mappings {
				got = append(got, region{m.min, m.max, m.perm, m.off})
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("mappings = %+v,\nwant %+v", got, test.want)
			}
		})
	}
}
