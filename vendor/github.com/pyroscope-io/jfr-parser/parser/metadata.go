package parser

import (
	"fmt"
	"strconv"

	"github.com/pyroscope-io/jfr-parser/reader"
)

type Element interface {
	SetAttribute(key, value string) error
	AppendChild(name string) Element
}

type AnnotationMetadata struct {
	Class  int64
	Values map[string]string
}

func (a *AnnotationMetadata) SetAttribute(key, value string) (err error) {
	switch key {
	case "class":
		a.Class, err = strconv.ParseInt(value, 10, 64)
	default:
		if a.Values == nil {
			a.Values = make(map[string]string)
		}
		a.Values[key] = value
	}
	return err
}

func (a AnnotationMetadata) AppendChild(string) Element { return nil }

// TODO: Proper attribute support for SettingMetadata
type SettingMetadata struct {
	Values map[string]string
}

func (s *SettingMetadata) SetAttribute(key, value string) error {
	if s.Values == nil {
		s.Values = make(map[string]string)
	}
	s.Values[key] = value
	return nil
}

func (s SettingMetadata) AppendChild(string) Element { return nil }

type FieldMetadata struct {
	Class                int64
	Name                 string
	ConstantPool         bool
	Dimension            int32
	Annotations          []AnnotationMetadata
	isBaseType           bool
	parseBaseTypeAndDrop func(r reader.Reader) error
}

func (f *FieldMetadata) SetAttribute(key, value string) (err error) {
	switch key {
	case "name":
		f.Name = value
	case "class":
		f.Class, err = strconv.ParseInt(value, 10, 64)
	case "constantPool":
		f.ConstantPool, err = parseBool(value)
	case "dimension":
		var n int64
		n, err = strconv.ParseInt(value, 10, 32)
		f.Dimension = int32(n)
	}
	return nil
}

func (f *FieldMetadata) AppendChild(name string) Element {
	switch name {
	case "annotation":
		f.Annotations = append(f.Annotations, AnnotationMetadata{})
		return &f.Annotations[len(f.Annotations)-1]
	}
	return nil
}

type ClassMetadata struct {
	ID           int64
	Name         string
	SuperType    string
	SimpleType   bool
	Fields       []FieldMetadata
	Settings     []SettingMetadata
	Annotations  []AnnotationMetadata
	numConstants int
	typeFn       func() ParseResolvable
	eventFn      func() Parseable
}

func (c *ClassMetadata) SetAttribute(key, value string) (err error) {
	switch key {
	case "id":
		c.ID, err = strconv.ParseInt(value, 10, 64)
	case "name":
		c.Name = value
	case "superType":
		c.SuperType = value
	case "simpleType":
		c.SimpleType, err = parseBool(value)
	}
	return err
}

func (c *ClassMetadata) AppendChild(name string) Element {
	switch name {
	case "field":
		c.Fields = append(c.Fields, FieldMetadata{})
		return &c.Fields[len(c.Fields)-1]
	case "setting":
		c.Settings = append(c.Settings, SettingMetadata{})
		return &c.Settings[len(c.Settings)-1]
	case "annotation":
		c.Annotations = append(c.Annotations, AnnotationMetadata{})
		return &c.Annotations[len(c.Annotations)-1]
	}
	return nil
}

type Metadata struct {
	Classes []ClassMetadata
}

func (m Metadata) SetAttribute(string, string) error { return nil }

func (m *Metadata) AppendChild(name string) Element {
	switch name {
	case "class":
		m.Classes = append(m.Classes, ClassMetadata{})
		return &m.Classes[len(m.Classes)-1]
	}
	return nil
}

type Region struct {
	Locale        string
	GMTOffset     string
	TicksToMillis string
}

func (m *Region) SetAttribute(key, value string) error {
	switch key {
	case "locale":
		m.Locale = value
	case "gmtOffset":
		// TODO int?
		m.GMTOffset = value
	case "ticksToMillis":
		// TODO int?
		m.TicksToMillis = value
	}
	return nil
}

func (m Region) AppendChild(string) Element { return nil }

type Root struct {
	Metadata Metadata
	Region   Region
}

func (r Root) SetAttribute(string, string) error { return nil }

func (r *Root) AppendChild(name string) Element {
	switch name {
	case "metadata":
		r.Metadata = Metadata{}
		return &r.Metadata
	case "region":
		r.Region = Region{}
		return &r.Region
	}
	return nil
}

type MetadataEvent struct {
	StartTime int64
	Duration  int64
	ID        int64
	Root      Root
}

func (m *MetadataEvent) Parse(r reader.Reader) (err error) {
	if kind, err := r.VarLong(); err != nil {
		return fmt.Errorf("unable to retrieve event type: %w", err)
	} else if kind != 0 {
		return fmt.Errorf("unexpected metadata event type: %d", kind)
	}

	if m.StartTime, err = r.VarLong(); err != nil {
		return fmt.Errorf("unable to parse metadata event's start time: %w", err)
	}
	if m.Duration, err = r.VarLong(); err != nil {
		return fmt.Errorf("unable to parse metadata event's duration: %w", err)
	}
	if m.ID, err = r.VarLong(); err != nil {
		return fmt.Errorf("unable to parse metadata event's ID: %w", err)
	}
	n, err := r.VarInt()
	if err != nil {
		return fmt.Errorf("unable to parse metadata event's number of strings: %w", err)
	}
	// TODO: assert n is small enough
	strings := make([]string, n)
	for i := 0; i < int(n); i++ {
		if strings[i], err = r.String(); err != nil {
			return fmt.Errorf("unable to parse metadata event's string: %w", err)
		}
	}

	name, err := parseName(r, strings)
	if err != nil {
		return err
	}
	if name != "root" {
		return fmt.Errorf("invalid root element name: %s", name)
	}

	m.Root = Root{}
	if err := parseElement(r, strings, &m.Root); err != nil {
		return fmt.Errorf("unable to parse metadata element tree: %w", err)
	}
	return nil
}

func parseElement(r reader.Reader, s []string, e Element) error {
	n, err := r.VarInt()
	if err != nil {
		return fmt.Errorf("unable to parse attribute count: %w", err)
	}
	// TODO: assert n is small enough
	for i := 0; i < int(n); i++ {
		k, err := parseName(r, s)
		if err != nil {
			return fmt.Errorf("unable to parse attribute key: %w", err)
		}
		v, err := parseName(r, s)
		if err != nil {
			return fmt.Errorf("unable to parse attribute value: %w", err)
		}
		if err := e.SetAttribute(k, v); err != nil {
			return fmt.Errorf("unable to set element attribute: %w", err)
		}
	}
	n, err = r.VarInt()
	if err != nil {
		return fmt.Errorf("unable to parse element count: %w", err)
	}
	// TODO: assert n is small enough
	for i := 0; i < int(n); i++ {
		name, err := parseName(r, s)
		if err != nil {
			return fmt.Errorf("unable to parse element name: %w", err)
		}
		child := e.AppendChild(name)
		if child == nil {
			return fmt.Errorf("unexpected child in metadata event: %s", name)
		}
		parseElement(r, s, child)
	}
	return nil
}

func parseName(r reader.Reader, s []string) (string, error) {
	n, err := r.VarInt()
	if err != nil {
		return "", fmt.Errorf("unable to parse string name index: %w", err)
	}
	if int(n) >= len(s) {
		return "", fmt.Errorf("invalid name index %d, only %d names available", n, len(s))
	}
	return s[int(n)], nil
}

func parseBool(s string) (bool, error) {
	if s == "true" {
		return true, nil
	}
	if s == "false" {
		return false, nil
	}
	return false, fmt.Errorf("unable to parse '%s' as boolean", s)
}
