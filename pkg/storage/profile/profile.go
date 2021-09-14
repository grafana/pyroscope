package profile

import (
	"sync"
)

const initialSize = 128

type Profile struct {
	sync.RWMutex
	Stacks []Stack
}

type Stack struct{ ID, Value uint64 }

func New() *Profile { return &Profile{Stacks: make([]Stack, 0, initialSize)} }

func (p *Profile) Merge(m ...*Profile) *Profile {
	// N.B: we should use "shared" profiles for merges.
	for _, n := range m {
		p.Stacks = append(p.Stacks, n.Stacks...)
	}
	if len(p.Stacks) == 0 {
		return p
	}
	// The following should be refactored so that "deduplication"
	// happens during the sort. It also may make sense implementing
	// type-specific sorting.
	p.Sort(ByStack)
	j := 0
	for i := 1; i < len(p.Stacks); i++ {
		if p.Stacks[j].ID == p.Stacks[i].ID {
			p.Stacks[j].Value += p.Stacks[i].Value
			continue
		}
		j++
		p.Stacks[j] = p.Stacks[i]
	}
	p.Stacks = p.Stacks[:j+1]
	return p
}

func (p *Profile) Clone(m, d uint64) *Profile {
	c := &Profile{Stacks: make([]Stack, len(p.Stacks))}
	for i := 0; i < len(p.Stacks); i++ {
		c.Stacks[i].ID = p.Stacks[i].ID
		c.Stacks[i].Value = p.Stacks[i].Value * m / d
	}
	return c
}
