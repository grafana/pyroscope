package cli

import (
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/iancoleman/strcase"
	"github.com/mitchellh/mapstructure"
	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/pyroscope-io/pyroscope/pkg/adhoc/util"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
	"github.com/pyroscope-io/pyroscope/pkg/util/duration"
	"github.com/pyroscope-io/pyroscope/pkg/util/slices"
)

const timeFormat = "2006-01-02T15:04:05Z0700"

type arrayFlags []string

func (i *arrayFlags) String() string {
	if len(*i) == 0 {
		return "[]"
	}
	return strings.Join(*i, ",")
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

func (*arrayFlags) Type() string {
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

func (*mapFlags) Type() string {
	t := reflect.TypeOf(map[string]string{})
	return t.String()
}

func Unmarshal(vpr *viper.Viper, cfg interface{}) error {
	return vpr.Unmarshal(cfg, viper.DecodeHook(
		mapstructure.ComposeDecodeHookFunc(
			// Function to add a special type for «env. mode»
			stringToByteSize,
			// Function to support net.IP
			mapstructure.StringToIPHookFunc(),
			// Appended by the two default functions
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		),
	))
}

func stringToByteSize(_, t reflect.Type, data interface{}) (interface{}, error) {
	if t != reflect.TypeOf(bytesize.Byte) {
		return data, nil
	}
	stringData, ok := data.(string)
	if !ok {
		return data, nil
	}
	return bytesize.Parse(stringData)
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

type roleFlag model.Role

func (rf *roleFlag) String() string {
	return model.Role(*rf).String()
}

func (rf *roleFlag) Set(value string) error {
	d, err := model.ParseRole(value)
	if err != nil {
		return err
	}
	*rf = roleFlag(d)
	return nil
}

func (rf *roleFlag) Type() string {
	return reflect.TypeOf(model.Role(*rf)).String()
}

type sameSiteFlag http.SameSite

func (sf *sameSiteFlag) String() string {
	switch http.SameSite(*sf) {
	case http.SameSiteLaxMode:
		return "Lax"
	case http.SameSiteStrictMode:
		return "Strict"
	default:
		return "None"
	}
}

func (sf *sameSiteFlag) Set(value string) error {
	v, err := parseSameSite(value)
	if err != nil {
		return err
	}
	*sf = sameSiteFlag(v)
	return nil
}

func parseSameSite(s string) (http.SameSite, error) {
	switch strings.ToLower(s) {
	case "lax":
		return http.SameSiteLaxMode, nil
	case "strict":
		return http.SameSiteStrictMode, nil
	case "none":
		return http.SameSiteNoneMode, nil
	default:
		return http.SameSiteDefaultMode, fmt.Errorf("unknown SameSite ")
	}
}

func (sf *sameSiteFlag) Type() string {
	return reflect.TypeOf(http.SameSite(*sf)).String()
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

func PopulateFlagSet(obj interface{}, flagSet *pflag.FlagSet, vpr *viper.Viper, opts ...FlagOption) *pflag.FlagSet {
	v := reflect.ValueOf(obj).Elem()
	t := reflect.TypeOf(v.Interface())

	o := &options{
		replacements: map[string]string{
			"<installPrefix>":           getInstallPrefix(),
			"<defaultAdhocDataPath>":    util.DataDirectory(),
			"<defaultAgentConfigPath>":  defaultAgentConfigPath(),
			"<defaultAgentLogFilePath>": defaultAgentLogFilePath(),
			"<supportedProfilers>":      strings.Join(spy.SupportedExecSpies(), ", "),
		},
	}
	for _, option := range opts {
		option(o)
	}

	visitFields(flagSet, vpr, "", t, v, o)

	return flagSet
}

//revive:disable-next-line:argument-limit,cognitive-complexity necessary complexity
func visitFields(flagSet *pflag.FlagSet, vpr *viper.Viper, prefix string, t reflect.Type, v reflect.Value, o *options) {
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
		case reflect.TypeOf([]string{}):
			val := fieldV.Addr().Interface().(*[]string)
			val2 := (*arrayFlags)(val)
			flagSet.Var(val2, nameVal, descVal)
			// setting empty defaults to allow vpr.Unmarshal to recognize this field
			vpr.SetDefault(nameVal, []string{})
		case reflect.TypeOf(map[string]string{}):
			val := fieldV.Addr().Interface().(*map[string]string)
			val2 := (*mapFlags)(val)
			flagSet.Var(val2, nameVal, descVal)
			// setting empty defaults to allow vpr.Unmarshal to recognize this field
			vpr.SetDefault(nameVal, map[string]string{})
		case reflect.TypeOf(""):
			val := fieldV.Addr().Interface().(*string)
			for old, n := range o.replacements {
				defaultValStr = strings.ReplaceAll(defaultValStr, old, n)
			}
			flagSet.StringVar(val, nameVal, defaultValStr, descVal)
			vpr.SetDefault(nameVal, defaultValStr)
		case reflect.TypeOf(true):
			val := fieldV.Addr().Interface().(*bool)
			flagSet.BoolVar(val, nameVal, defaultValStr == "true", descVal)
			vpr.SetDefault(nameVal, defaultValStr == "true")
		case reflect.TypeOf(time.Time{}):
			valTime := fieldV.Addr().Interface().(*time.Time)
			val := (*timeFlag)(valTime)
			flagSet.Var(val, nameVal, descVal)
			// setting empty defaults to allow vpr.Unmarshal to recognize this field
			vpr.SetDefault(nameVal, time.Time{})
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
			vpr.SetDefault(nameVal, defaultVal)
		case reflect.TypeOf(model.InvalidRole):
			valRole := fieldV.Addr().Interface().(*model.Role)
			val := (*roleFlag)(valRole)
			var defaultVal model.Role
			if defaultValStr != "" {
				var err error
				defaultVal, err = model.ParseRole(defaultValStr)
				if err != nil {
					logrus.Fatalf("invalid default value: %q (%s)", defaultValStr, nameVal)
				}
			}
			*val = (roleFlag)(defaultVal)
		case reflect.TypeOf(http.SameSiteStrictMode):
			valP := fieldV.Addr().Interface().(*http.SameSite)
			val := (*sameSiteFlag)(valP)
			var defaultVal http.SameSite
			if defaultValStr != "" {
				var err error
				defaultVal, err = parseSameSite(defaultValStr)
				if err != nil {
					logrus.Fatalf("invalid default value: %q (%s)", defaultValStr, nameVal)
				}
			}
			*val = (sameSiteFlag)(defaultVal)
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
			vpr.SetDefault(nameVal, defaultVal)
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
			vpr.SetDefault(nameVal, defaultVal)
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
			vpr.SetDefault(nameVal, defaultVal)
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
			vpr.SetDefault(nameVal, defaultVal)
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
			vpr.SetDefault(nameVal, defaultVal)
		case reflect.TypeOf(config.MetricsExportRules{}):
			flagSet.Var(new(mapFlags), nameVal, descVal)
			vpr.SetDefault(nameVal, config.MetricsExportRules{})
		case reflect.TypeOf([]config.Target{}):
			flagSet.Var(new(arrayFlags), nameVal, descVal)
			vpr.SetDefault(nameVal, []config.Target{})
		default:
			if field.Type.Kind() == reflect.Struct {
				visitFields(flagSet, vpr, nameVal, field.Type, fieldV, o)
				continue
			}

			// A stub for unknown types. This is required for generated configs and
			// documentation (when a parameter can not be set via flag but present
			// in the configuration). Empty value is shown as '{}'.
			flagSet.Var(new(mapFlags), nameVal, descVal)
			vpr.SetDefault(nameVal, nil)
		}

		if deprecatedVal == "true" {
			// TODO: We could specify which flag to use instead but would add code complexity
			flagSet.MarkDeprecated(nameVal, "replace this flag as it will be removed in future versions")
		}
	}
}
