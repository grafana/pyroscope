package cli

import (
	"flag"
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
)

const timeFormat = "2006-01-02T15:04:05Z0700"

type arrayFlags []string

func (i *arrayFlags) String() string {
	return strings.Join(*i, ", ")
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
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

func PopulateFlagSet(obj interface{}, flagSet *flag.FlagSet, skip ...string) *SortedFlags {
	v := reflect.ValueOf(obj).Elem()
	t := reflect.TypeOf(v.Interface())
	num := t.NumField()

	installPrefix := getInstallPrefix()
	supportedSpies := strings.Join(spy.SupportedExecSpies(), ", ")

	deprecatedFields := []string{}

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
		if skipVal == "true" || slices.StringContains(skip, nameVal) {
			continue
		}

		if deprecatedVal == "true" {
			deprecatedFields = append(deprecatedFields, nameVal)
		}

		descVal = strings.ReplaceAll(descVal, "<supportedProfilers>", supportedSpies)

		switch field.Type {
		case reflect.TypeOf([]string{}):
			val := fieldV.Addr().Interface().(*[]string)
			val2 := (*arrayFlags)(val)
			flagSet.Var(val2, nameVal, descVal)
		case reflect.TypeOf(""):
			val := fieldV.Addr().Interface().(*string)
			defaultValStr = strings.ReplaceAll(defaultValStr, "<installPrefix>", installPrefix)
			defaultValStr = strings.ReplaceAll(defaultValStr, "<defaultAgentConfigPath>", defaultAgentConfigPath())
			defaultValStr = strings.ReplaceAll(defaultValStr, "<defaultAgentLogFilePath>", defaultAgentLogFilePath())
			flagSet.StringVar(val, nameVal, defaultValStr, descVal)
		case reflect.TypeOf(true):
			val := fieldV.Addr().Interface().(*bool)
			flagSet.BoolVar(val, nameVal, defaultValStr == "true", descVal)
		case reflect.TypeOf(time.Time{}):
			valTime := fieldV.Addr().Interface().(*time.Time)
			val := (*timeFlag)(valTime)
			flagSet.Var(val, nameVal, descVal)
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
		case reflect.TypeOf(bytesize.Byte):
			val := fieldV.Addr().Interface().(*bytesize.ByteSize)
			var defaultVal bytesize.ByteSize
			if defaultValStr != "" {
				var err error
				defaultVal, err = bytesize.Parse(defaultValStr)
				if err != nil {
					logrus.Fatalf("invalid default value: %q (%s)", defaultValStr, nameVal)
				}
			}
			*val = defaultVal
			flagSet.Var(val, nameVal, descVal)
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
		default:
			logrus.Fatalf("type %s is not supported", field.Type)
		}
	}
	return NewSortedFlags(obj, flagSet, deprecatedFields)
}
