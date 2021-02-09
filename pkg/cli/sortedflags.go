package cli

import (
	"flag"
	"reflect"
	"sort"

	"github.com/iancoleman/strcase"
	"github.com/peterbourgon/ff/v3/ffcli"
)

// SortedFlags is needed because by default there's no way to provide order for flags
//   this is kind of an ugly workaround
type SortedFlags struct {
	orderMap map[string]int
	flags    []*flag.Flag
}

func (sf *SortedFlags) Len() int {
	return len(sf.flags)
}

func (sf *SortedFlags) Swap(i, j int) {
	sf.flags[i], sf.flags[j] = sf.flags[j], sf.flags[i]
}

func (sf *SortedFlags) Less(i, j int) bool {
	return sf.orderMap[sf.flags[i].Name] < sf.orderMap[sf.flags[j].Name]
}

func (sf *SortedFlags) VisitAll(cb func(*flag.Flag)) {
	for _, v := range sf.flags {
		cb(v)
	}
}

func (sf *SortedFlags) printUsage(c *ffcli.Command) string {
	return gradientBanner() + "\n" + DefaultUsageFunc(sf, c)
}

func NewSortedFlags(obj interface{}, fs *flag.FlagSet) *SortedFlags {
	v := reflect.ValueOf(obj).Elem()
	t := reflect.TypeOf(v.Interface())
	num := t.NumField()

	res := SortedFlags{
		orderMap: make(map[string]int),
		flags:    []*flag.Flag{},
	}

	for i := 0; i < num; i++ {
		field := t.Field(i)
		nameVal := field.Tag.Get("name")
		if nameVal == "" {
			nameVal = strcase.ToKebab(field.Name)
		}
		// orderVal := field.Tag.Get("order")
		// order, _ := strconv.Atoi(orderVal)
		order := i
		res.orderMap[nameVal] = order
	}
	fs.VisitAll(func(f *flag.Flag) {
		res.flags = append(res.flags, f)
	})

	sort.Sort(&res)
	return &res
}
