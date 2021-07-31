package cli

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/iancoleman/strcase"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
	"github.com/pyroscope-io/pyroscope/pkg/util/duration"
	"github.com/pyroscope-io/pyroscope/pkg/util/slices"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const timeFormat = "2006-01-02T15:04:05Z0700"

type arrayFlags []string

func (i *arrayFlags) String() string {
	if len(*i) == 0 {
		return "[]"
	}
	return strings.Join(*i, ", ")
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

func (i *arrayFlags) Type() string {
	t := reflect.TypeOf([]string{})
	return t.String()
}

type timeFlag time.Time

func (tf *timeFlag) String() string {
	v := time.Time(*tf)
	return v.Format(timeFormat)
}

func (tf *timeFlag) Set(value string) error {
	t2, err := time.Parse(timeFormat, value)
	if err != nil {
		var i int
		i, err = strconv.Atoi(value)
		if err != nil {
			return err
		}
		t2 = time.Unix(int64(i), 0)
	}

	t := (*time.Time)(tf)
	b, _ := t2.MarshalBinary()
	t.UnmarshalBinary(b)

	return nil
}

func (tf *timeFlag) Type() string {
	v := time.Time(*tf)
	t := reflect.TypeOf(v)
	return t.String()
}

type mapFlags map[string]string

func (m mapFlags) String() string {
	if len(m) == 0 {
		return "{}"
	}
	// Cast to map to avoid recursion.
	return fmt.Sprint((map[string]string)(m))
}

func (m *mapFlags) Set(s string) error {
	if len(s) == 0 {
		return nil
	}
	v := strings.Split(s, "=")
	if len(v) != 2 {
		return fmt.Errorf("invalid flag %s: should be in key=value format", s)
	}
	if *m == nil {
		*m = map[string]string{v[0]: v[1]}
	} else {
		(*m)[v[0]] = v[1]
	}
	return nil
}

func (m *mapFlags) Type() string {
	t := reflect.TypeOf(map[string]string{})
	return t.String()
}

type options struct {
	replacements   map[string]string
	skip           []string
	skipDeprecated bool
}

type FlagOption func(*options)

func WithSkip(n ...string) FlagOption {
	return func(o *options) {
		o.skip = append(o.skip, n...)
	}
}

// WithSkipDeprecated specifies that fields marked as deprecated won't be parsed.
// By default PopulateFlagSet parses them but not shows in Usage; setting this
// option to true causes PopulateFlagSet to skip parsing.
func WithSkipDeprecated(ok bool) FlagOption {
	return func(o *options) {
		o.skipDeprecated = ok
	}
}

func WithReplacement(k, v string) FlagOption {
	return func(o *options) {
		o.replacements[k] = v
	}
}

type durFlag time.Duration

func (df *durFlag) String() string {
	v := time.Duration(*df)
	return v.String()
}

func (df *durFlag) Set(value string) error {
	d, err := duration.ParseDuration(value)
	if err != nil {
		return err
	}

	*df = durFlag(d)

	return nil
}

func (df *durFlag) Type() string {
	v := time.Duration(*df)
	t := reflect.TypeOf(v)
	return t.String()
}

type byteSizeFlag bytesize.ByteSize

func (bs *byteSizeFlag) String() string {
	v := bytesize.ByteSize(*bs)
	return v.String()
}

func (bs *byteSizeFlag) Set(value string) error {
	d, err := bytesize.Parse(value)
	if err != nil {
		return err
	}

	*bs = byteSizeFlag(d)

	return nil
}

func (bs *byteSizeFlag) Type() string {
	v := bytesize.ByteSize(*bs)
	t := reflect.TypeOf(v)
	return t.String()
}

func PopulateFlagSet(obj interface{}, flagSet *pflag.FlagSet, opts ...FlagOption) *pflag.FlagSet {
	v := reflect.ValueOf(obj).Elem()
	t := reflect.TypeOf(v.Interface())

	o := &options{
		replacements: map[string]string{
			"<installPrefix>":           getInstallPrefix(),
			"<defaultAgentConfigPath>":  defaultAgentConfigPath(),
			"<defaultAgentLogFilePath>": defaultAgentLogFilePath(),
			"<supportedProfilers>":      strings.Join(spy.SupportedExecSpies(), ", "),
		},
	}
	for _, option := range opts {
		option(o)
	}

	visitFields(flagSet, "", t, v, o)

	return flagSet
}

func visitFields(flagSet *pflag.FlagSet, prefix string, t reflect.Type, v reflect.Value, o *options) {
	num := t.NumField()
	for i := 0; i < num; i++ {
		field := t.Field(i)
		fieldV := v.Field(i)

		if !(fieldV.IsValid() && fieldV.CanSet()) {
			continue
		}

		defaultValStr := field.Tag.Get("def")
		descVal := field.Tag.Get("desc")
		skipVal := field.Tag.Get("skip")
		deprecatedVal := field.Tag.Get("deprecated")
		nameVal := field.Tag.Get("name")
		if nameVal == "" {
			nameVal = strcase.ToKebab(field.Name)
		}
		if prefix != "" {
			nameVal = prefix + "." + nameVal
		}

		if skipVal == "true" || slices.StringContains(o.skip, nameVal) {
			continue
		}

		for old, n := range o.replacements {
			descVal = strings.ReplaceAll(descVal, old, n)
		}

		if fieldV.Kind() == reflect.Slice && field.Type.Elem().Kind() == reflect.Struct {
			flagSet.Var(new(arrayFlags), nameVal, descVal)
			continue
		}

		switch field.Type {
		// TODO: Add support for nested struct visit and populate
		case reflect.TypeOf([]string{}):
			val := fieldV.Addr().Interface().(*[]string)
			val2 := (*arrayFlags)(val)
			flagSet.Var(val2, nameVal, descVal)
			// setting empty defaults to allow viper.Unmarshal to recognize this field
			viper.SetDefault(nameVal, []string{})
		case reflect.TypeOf(map[string]string{}):
			val := fieldV.Addr().Interface().(*map[string]string)
			val2 := (*mapFlags)(val)
			flagSet.Var(val2, nameVal, descVal)
			// setting empty defaults to allow viper.Unmarshal to recognize this field
			viper.SetDefault(nameVal, map[string]string{})
		case reflect.TypeOf(""):
			val := fieldV.Addr().Interface().(*string)
			for old, n := range o.replacements {
				defaultValStr = strings.ReplaceAll(defaultValStr, old, n)
			}
			flagSet.StringVar(val, nameVal, defaultValStr, descVal)
			viper.SetDefault(nameVal, defaultValStr)
		case reflect.TypeOf(true):
			val := fieldV.Addr().Interface().(*bool)
			flagSet.BoolVar(val, nameVal, defaultValStr == "true", descVal)
			viper.SetDefault(nameVal, defaultValStr == "true")
		case reflect.TypeOf(time.Time{}):
			valTime := fieldV.Addr().Interface().(*time.Time)
			val := (*timeFlag)(valTime)
			flagSet.Var(val, nameVal, descVal)
			// setting empty defaults to allow viper.Unmarshal to recognize this field
			viper.SetDefault(nameVal, time.Time{})
		case reflect.TypeOf(time.Second):
			valDur := fieldV.Addr().Interface().(*time.Duration)
			val := (*durFlag)(valDur)

			var defaultVal time.Duration
			if defaultValStr != "" {
				var err error
				defaultVal, err = duration.ParseDuration(defaultValStr)
				if err != nil {
					logrus.Fatalf("invalid default value: %q (%s)", defaultValStr, nameVal)
				}
			}
			*val = (durFlag)(defaultVal)

			flagSet.Var(val, nameVal, descVal)
			viper.SetDefault(nameVal, defaultVal)
		case reflect.TypeOf(bytesize.Byte):
			valByteSize := fieldV.Addr().Interface().(*bytesize.ByteSize)
			val := (*byteSizeFlag)(valByteSize)
			var defaultVal bytesize.ByteSize
			if defaultValStr != "" {
				var err error
				defaultVal, err = bytesize.Parse(defaultValStr)
				if err != nil {
					logrus.Fatalf("invalid default value: %q (%s)", defaultValStr, nameVal)
				}
			}

			*val = (byteSizeFlag)(defaultVal)
			flagSet.Var(val, nameVal, descVal)
			viper.SetDefault(nameVal, defaultVal)
		case reflect.TypeOf(1):
			val := fieldV.Addr().Interface().(*int)
			var defaultVal int
			if defaultValStr == "" {
				defaultVal = 0
			} else {
				var err error
				defaultVal, err = strconv.Atoi(defaultValStr)
				if err != nil {
					logrus.Fatalf("invalid default value: %q (%s)", defaultValStr, nameVal)
				}
			}
			flagSet.IntVar(val, nameVal, defaultVal, descVal)
			viper.SetDefault(nameVal, defaultVal)
		case reflect.TypeOf(1.00):
			val := fieldV.Addr().Interface().(*float64)
			var defaultVal float64
			if defaultValStr == "" {
				defaultVal = 0.00
			} else {
				var err error
				defaultVal, err = strconv.ParseFloat(defaultValStr, 64)
				if err != nil {
					logrus.Fatalf("invalid default value: %q (%s)", defaultValStr, nameVal)
				}
			}
			flagSet.Float64Var(val, nameVal, defaultVal, descVal)
			viper.SetDefault(nameVal, defaultVal)
		case reflect.TypeOf(uint64(1)):
			val := fieldV.Addr().Interface().(*uint64)
			var defaultVal uint64
			if defaultValStr == "" {
				defaultVal = uint64(0)
			} else {
				var err error
				defaultVal, err = strconv.ParseUint(defaultValStr, 10, 64)
				if err != nil {
					logrus.Fatalf("invalid default value: %q (%s)", defaultValStr, nameVal)
				}
			}
			flagSet.Uint64Var(val, nameVal, defaultVal, descVal)
			viper.SetDefault(nameVal, defaultVal)
		case reflect.TypeOf(uint(1)):
			val := fieldV.Addr().Interface().(*uint)
			var defaultVal uint
			if defaultValStr == "" {
				defaultVal = uint(0)
			} else {
				out, err := strconv.ParseUint(defaultValStr, 10, 64)
				if err != nil {
					logrus.Fatalf("invalid default value: %q (%s)", defaultValStr, nameVal)
				}
				defaultVal = uint(out)
			}
			flagSet.UintVar(val, nameVal, defaultVal, descVal)
			viper.SetDefault(nameVal, defaultVal)
		default:
			if field.Type.Kind() == reflect.Struct {
				visitFields(flagSet, nameVal, field.Type, fieldV, o)
				continue
			}

			logrus.Fatalf("type %s is not supported", field.Type)
		}

		if deprecatedVal == "true" {
			// TODO: We could specify which flag to use instead but would add code complexity
			flagSet.MarkDeprecated(nameVal, "repalce this flag as it will be removed in future versions")
		}
	}
}
