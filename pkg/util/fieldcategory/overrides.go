// SPDX-License-Identifier: AGPL-3.0-only

package fieldcategory

import (
	"fmt"
	"sync"
)

type Category int

const (
	// Basic is the basic field category, and the default if none is defined.
	Basic Category = iota
	// Advanced is the advanced field category.
	Advanced
	// Experimental is the experimental field category.
	Experimental
)

func (c Category) String() string {
	switch c {
	case Basic:
		return "basic"
	case Advanced:
		return "advanced"
	case Experimental:
		return "experimental"
	default:
		panic(fmt.Sprintf("Unknown field category: %d", c))
	}
}

// Fields are primarily categorized via struct tags, but this can be impossible when third party libraries are involved
// Only categorize fields here when you can't otherwise, since struct tags are less likely to become stale
var (
	overridesMu sync.RWMutex
	overrides   = map[string]Category{}
)

func AddOverrides(o map[string]Category) {
	overridesMu.Lock()
	defer overridesMu.Unlock()
	for n, c := range o {
		overrides[n] = c
	}
}

func GetOverride(fieldName string) (category Category, ok bool) {
	overridesMu.RLock()
	defer overridesMu.RUnlock()
	category, ok = overrides[fieldName]
	return
}

func VisitOverrides(f func(name string)) {
	overridesMu.RLock()
	defer overridesMu.RUnlock()
	for override := range overrides {
		f(override)
	}
}
