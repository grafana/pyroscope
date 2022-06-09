package parquet

// The SortConfig type carries configuration options used to generate sorting
// functions.
//
// SortConfig implements the SortOption interface so it can be used directly as
// argument to the SortFuncOf function, for example:
//
//	sortFunc := parquet.SortFuncOf(columnType, &parquet.SortConfig{
//		Descending: true,
//		NullsFirst: true,
//	})
//
type SortConfig struct {
	MaxRepetitionLevel int
	MaxDefinitionLevel int
	Descending         bool
	NullsFirst         bool
}

// Apply applies options to c.
func (c *SortConfig) Apply(options ...SortOption) {
	for _, opt := range options {
		opt.ConfigureSort(c)
	}
}

// ConfigureSort satisfies the SortOption interface.
func (c *SortConfig) ConfigureSort(config *SortConfig) {
	*c = *config
}

// SortMaxRepetitionLevel constructs a configuration option which sets the
// maximum repetition level known to a sorting function.
//
// Defaults to zero, which represents a non-repeated column.
func SortMaxRepetitionLevel(level int) SortOption {
	return sortingOption(func(c *SortConfig) { c.MaxRepetitionLevel = level })
}

// SortMaxDefinitionLevel constructs a configuration option which sets the
// maximum definition level known to a sorting function.
//
// Defaults to zero, which represents a non-nullable column.
func SortMaxDefinitionLevel(level int) SortOption {
	return sortingOption(func(c *SortConfig) { c.MaxDefinitionLevel = level })
}

// SortDescending constructs a configuration option which inverts the order of a
// sorting function.
//
// Defaults to false, which means values are sorted in ascending order.
func SortDescending(descending bool) SortOption {
	return sortingOption(func(c *SortConfig) { c.Descending = descending })
}

// SortNullsFirst constructs a configuration option which places the null values
// first or last.
//
// Defaults to false, which means null values are placed last.
func SortNullsFirst(nullsFirst bool) SortOption {
	return sortingOption(func(c *SortConfig) { c.NullsFirst = nullsFirst })
}

// SortOption is an interface implemented by types that carry configuration
// options for sorting functions.
type SortOption interface {
	ConfigureSort(*SortConfig)
}

type sortingOption func(*SortConfig)

func (f sortingOption) ConfigureSort(c *SortConfig) { f(c) }

// SortFunc is a function type which compares two sets of column values.
//
// Slices with exactly one value must be passed to the function when comparing
// values of non-repeated columns. For repeated columns, there may be zero or
// more values in each slice, and the parameters may have different lengths.
//
// SortFunc is a low-level API which is usually useful to construct customize
// implementations of the RowGroup interface.
type SortFunc func(a, b []Value) int

// SortFuncOf constructs a sorting function for values of the given type.
//
// The list of options contains the configuration used to construct the sorting
// function.
func SortFuncOf(t Type, options ...SortOption) SortFunc {
	config := new(SortConfig)
	config.Apply(options...)
	return sortFuncOf(t, config)
}

func sortFuncOf(t Type, config *SortConfig) (sort SortFunc) {
	sort = sortFuncOfRequired(t)

	if config.Descending {
		sort = sortFuncOfDescending(sort)
	}

	switch {
	case makeRepetitionLevel(config.MaxRepetitionLevel) > 0:
		sort = sortFuncOfRepeated(sort, config)
	case makeDefinitionLevel(config.MaxDefinitionLevel) > 0:
		sort = sortFuncOfOptional(sort, config)
	}

	return sort
}

//go:noinline
func sortFuncOfDescending(sort SortFunc) SortFunc {
	return func(a, b []Value) int { return -sort(a, b) }
}

func sortFuncOfOptional(sort SortFunc, config *SortConfig) SortFunc {
	if config.NullsFirst {
		return sortFuncOfOptionalNullsFirst(sort)
	} else {
		return sortFuncOfOptionalNullsLast(sort)
	}
}

//go:noinline
func sortFuncOfOptionalNullsFirst(sort SortFunc) SortFunc {
	return func(a, b []Value) int {
		switch {
		case a[0].IsNull():
			if b[0].IsNull() {
				return 0
			}
			return -1
		case b[0].IsNull():
			return +1
		default:
			return sort(a, b)
		}
	}
}

//go:noinline
func sortFuncOfOptionalNullsLast(sort SortFunc) SortFunc {
	return func(a, b []Value) int {
		switch {
		case a[0].IsNull():
			if b[0].IsNull() {
				return 0
			}
			return +1
		case b[0].IsNull():
			return -1
		default:
			return sort(a, b)
		}
	}
}

//go:noinline
func sortFuncOfRepeated(sort SortFunc, config *SortConfig) SortFunc {
	sort = sortFuncOfOptional(sort, config)
	return func(a, b []Value) int {
		n := len(a)
		if n > len(b) {
			n = len(b)
		}

		for i := 0; i < n; i++ {
			k := sort(a[i:i+1], b[i:i+1])
			if k != 0 {
				return k
			}
		}

		return len(a) - len(b)
	}
}

//go:noinline
func sortFuncOfRequired(t Type) SortFunc {
	return func(a, b []Value) int { return t.Compare(a[0], b[0]) }
}
