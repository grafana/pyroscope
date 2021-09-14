package profile

import "sort"

type Order int

const (
	NotSorted Order = iota
	ByValue
	ByStack
)

type byValue []Stack

func (s byValue) Len() int           { return len(s) }
func (s byValue) Less(i, j int) bool { return s[j].Value < s[i].Value }
func (s byValue) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

type byStackID []Stack

func (s byStackID) Len() int           { return len(s) }
func (s byStackID) Less(i, j int) bool { return s[j].ID < s[i].ID }
func (s byStackID) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func (p *Profile) Sort(o Order) *Profile {
	switch o {
	case ByStack:
		sort.Sort(byStackID(p.Stacks))
	case ByValue:
		sort.Sort(byValue(p.Stacks))
	}
	return p
}
