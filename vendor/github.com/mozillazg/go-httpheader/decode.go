package httpheader

import (
	"fmt"
	"net/http"
	"net/textproto"
	"reflect"
	"strconv"
	"time"
)

// Decoder is an interface implemented by any type that wishes to decode
// itself from Header fields in a non-standard way.
type Decoder interface {
	DecodeHeader(header http.Header, key string) error
}

// Decode expects to be passed an http.Header and a struct, and parses
// header into the struct recursively using the same rules as Header (see above)
func Decode(header http.Header, v interface{}) error {
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Ptr || val.IsNil() {
		return fmt.Errorf("v should be point and should not be nil")
	}

	for val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return fmt.Errorf("v is not a struct %+v", val.Kind())
	}
	return parseValue(header, val)
}

func parseValue(header http.Header, val reflect.Value) error {
	typ := val.Type()
	for i := 0; i < typ.NumField(); i++ {
		sf := typ.Field(i)
		if sf.PkgPath != "" && !sf.Anonymous { // unexported
			continue
		}

		sv := val.Field(i)
		tag := sf.Tag.Get(tagName)
		if tag == "-" {
			continue
		}
		name, opts := parseTag(tag)
		if name == "" {
			if sf.Anonymous && sv.Kind() == reflect.Struct {
				continue
			}
			name = sf.Name
		}

		if opts.Contains("omitempty") && header.Get(name) == "" {
			continue
		}

		// Decoder interface
		addr := sv
		if addr.Kind() != reflect.Ptr && addr.Type().Name() != "" && addr.CanAddr() {
			addr = addr.Addr()
		}
		if addr.Type().NumMethod() > 0 && addr.CanInterface() {
			if m, ok := addr.Interface().(Decoder); ok {
				if err := m.DecodeHeader(header, name); err != nil {
					return err
				}
				continue
			}
		}

		if sv.Kind() == reflect.Ptr {
			valArr, exist := headerValues(header, name)
			if !exist {
				continue
			}
			ve := reflect.New(sv.Type().Elem())
			if err := fillValues(ve, opts, valArr); err != nil {
				return err
			}
			sv.Set(ve)
			continue
		}

		if sv.Type() == timeType {
			valArr, exist := headerValues(header, name)
			if !exist {
				continue
			}
			if err := fillValues(sv, opts, valArr); err != nil {
				return err
			}
			continue
		}

		if sv.Kind() == reflect.Struct {
			if err := parseValue(header, sv); err != nil {
				return err
			}
			continue
		}

		valArr, exist := headerValues(header, name)
		if !exist {
			continue
		}
		if err := fillValues(sv, opts, valArr); err != nil {
			return err
		}
	}
	return nil
}

func fillValues(sv reflect.Value, opts tagOptions, valArr []string) error {
	var err error
	var value string
	if len(valArr) > 0 {
		value = valArr[0]
	}
	for sv.Kind() == reflect.Ptr {
		sv = sv.Elem()
	}

	switch sv.Kind() {
	case reflect.Bool:
		var v bool
		if opts.Contains("int") {
			v = value != "0"
		} else {
			v = value != "false"
		}
		sv.SetBool(v)
		return nil
	case reflect.String:
		sv.SetString(value)
		return nil
	case reflect.Uint, reflect.Uint64:
		var v uint64
		if v, err = strconv.ParseUint(value, 10, 64); err != nil {
			return err
		}
		sv.SetUint(v)
		return nil
	case reflect.Uint8:
		var v uint64
		if v, err = strconv.ParseUint(value, 10, 8); err != nil {
			return err
		}
		sv.SetUint(v)
		return nil
	case reflect.Uint16:
		var v uint64
		if v, err = strconv.ParseUint(value, 10, 16); err != nil {
			return err
		}
		sv.SetUint(v)
		return nil
	case reflect.Uint32:
		var v uint64
		if v, err = strconv.ParseUint(value, 10, 32); err != nil {
			return err
		}
		sv.SetUint(v)
		return nil
	case reflect.Int, reflect.Int64:
		var v int64
		if v, err = strconv.ParseInt(value, 10, 64); err != nil {
			return err
		}
		sv.SetInt(v)
		return nil
	case reflect.Int8:
		var v int64
		if v, err = strconv.ParseInt(value, 10, 8); err != nil {
			return err
		}
		sv.SetInt(v)
		return nil
	case reflect.Int16:
		var v int64
		if v, err = strconv.ParseInt(value, 10, 16); err != nil {
			return err
		}
		sv.SetInt(v)
		return nil
	case reflect.Int32:
		var v int64
		if v, err = strconv.ParseInt(value, 10, 32); err != nil {
			return err
		}
		sv.SetInt(v)
		return nil
	case reflect.Float32:
		var v float64
		if v, err = strconv.ParseFloat(value, 32); err != nil {
			return err
		}
		sv.SetFloat(v)
		return nil
	case reflect.Float64:
		var v float64
		if v, err = strconv.ParseFloat(value, 64); err != nil {
			return err
		}
		sv.SetFloat(v)
		return nil
	case reflect.Slice:
		v := reflect.MakeSlice(sv.Type(), len(valArr), len(valArr))
		for i, s := range valArr {
			eleV := reflect.New(sv.Type().Elem()).Elem()
			if err := fillValues(eleV, opts, []string{s}); err != nil {
				return err
			}
			v.Index(i).Set(eleV)
		}
		sv.Set(v)
		return nil
	case reflect.Array:
		v := reflect.Indirect(reflect.New(reflect.ArrayOf(sv.Len(), sv.Type().Elem())))
		length := len(valArr)
		if sv.Len() < length {
			length = sv.Len()
		}
		for i := 0; i < length; i++ {
			eleV := reflect.New(sv.Type().Elem()).Elem()
			if err := fillValues(eleV, opts, []string{valArr[i]}); err != nil {
				return err
			}
			v.Index(i).Set(eleV)
		}
		sv.Set(v)
		return nil
	case reflect.Interface:
		v := reflect.ValueOf(valArr)
		sv.Set(v)
		return nil
	}

	if sv.Type() == timeType {
		var v time.Time
		if opts.Contains("unix") {
			u, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return err
			}
			v = time.Unix(u, 0).UTC()
		} else {
			v, err = time.Parse(http.TimeFormat, value)
			if err != nil {
				return err
			}
		}
		sv.Set(reflect.ValueOf(v))
		return nil
	}

	// sv.Set(reflect.ValueOf(value))
	return nil
}

func headerValues(h http.Header, key string) ([]string, bool) {
	vs, ok := textproto.MIMEHeader(h)[textproto.CanonicalMIMEHeaderKey(key)]
	return vs, ok
}
