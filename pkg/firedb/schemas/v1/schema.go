package v1

import (
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/segmentio/parquet-go"
	"github.com/segmentio/parquet-go/compress"
	"github.com/segmentio/parquet-go/deprecated"
	"github.com/segmentio/parquet-go/encoding"
	"github.com/segmentio/parquet-go/format"
)

type Group []*groupField

func (g Group) String() string {
	s := new(strings.Builder)
	parquet.PrintSchema(s, "", g)
	return s.String()
}

func (g Group) Type() parquet.Type { return &groupType{} }

func (g Group) Optional() bool { return false }

func (g Group) Repeated() bool { return false }

func (g Group) Required() bool { return true }

func (g Group) Leaf() bool { return false }

func (g Group) Fields() []parquet.Field {
	fields := make([]parquet.Field, len(g))
	for pos := range g {
		fields[pos] = g[pos]
	}
	return fields
}

func (g Group) Encoding() encoding.Encoding { return nil }

func (g Group) Compression() compress.Codec { return nil }

func (g Group) GoType() reflect.Type { return goTypeOfGroup(g) }

func exportedStructFieldName(name string) string {
	firstRune, size := utf8.DecodeRuneInString(name)
	return string([]rune{unicode.ToUpper(firstRune)}) + name[size:]
}

func goTypeOfGroup(node parquet.Node) reflect.Type {
	fields := node.Fields()
	structFields := make([]reflect.StructField, len(fields))
	for i, field := range fields {
		structFields[i].Name = exportedStructFieldName(field.Name())
		structFields[i].Type = field.GoType()
		// TODO: can we reconstruct a struct tag that would be valid if a value
		// of this type were passed to SchemaOf?
	}
	return reflect.StructOf(structFields)
}

type groupField struct {
	parquet.Node
	name string
}

type groupType struct{}

func (groupType) String() string { return "group" }

func (groupType) Kind() parquet.Kind {
	panic("cannot call Kind on parquet group")
}

func (groupType) Compare(parquet.Value, parquet.Value) int {
	panic("cannot compare values on parquet group")
}

func (groupType) NewColumnIndexer(int) parquet.ColumnIndexer {
	panic("cannot create column indexer from parquet group")
}

func (groupType) NewDictionary(int, int, []byte) parquet.Dictionary {
	panic("cannot create dictionary from parquet group")
}

func (t groupType) NewColumnBuffer(int, int) parquet.ColumnBuffer {
	panic("cannot create column buffer from parquet group")
}

func (t groupType) NewPage(int, int, []byte) parquet.Page {
	panic("cannot create page from parquet group")
}

func (groupType) Encode(_, _ []byte, _ encoding.Encoding) ([]byte, error) {
	panic("cannot encode parquet group")
}

func (groupType) Decode(_, _ []byte, _ encoding.Encoding) ([]byte, error) {
	panic("cannot decode parquet group")
}

func (groupType) Length() int { return 0 }

func (groupType) EstimateSize(int) int64 { return 0 }

func (groupType) ColumnOrder() *format.ColumnOrder { return nil }

func (groupType) PhysicalType() *format.Type { return nil }

func (groupType) LogicalType() *format.LogicalType { return nil }

func (groupType) ConvertedType() *deprecated.ConvertedType { return nil }

func (f *groupField) Name() string { return f.name }

func (f *groupField) Value(base reflect.Value) reflect.Value {
	return base.MapIndex(reflect.ValueOf(&f.name).Elem())
}

func ProfilesSchema() *parquet.Schema {
	stringRef := parquet.Encoded(parquet.Int(64), &parquet.DeltaBinaryPacked)
	sampleType := parquet.Group{
		"Type": stringRef,
		"Unit": stringRef,
	}

	externalLabels := parquet.Repeated(Group{
		{name: "Name", Node: stringRef},
		{name: "Value", Node: stringRef},
	})

	pprofLabels := parquet.Repeated(Group{
		{name: "Key", Node: stringRef},
		{name: "Str", Node: parquet.Optional(stringRef)},
		{name: "Num", Node: parquet.Optional(parquet.Int(64))},
		{name: "NumUnit", Node: parquet.Optional(stringRef)},
	})

	s := parquet.NewSchema("Profile", Group{
		{name: "ID", Node: parquet.UUID()},
		{name: "ExternalLabels", Node: externalLabels},
		{name: "Types", Node: parquet.Repeated(sampleType)},
		{name: "Samples", Node: parquet.Repeated(Group{
			{name: "LocationIds", Node: parquet.Repeated(parquet.Uint(64))},
			{name: "Values", Node: parquet.Repeated(parquet.Encoded(parquet.Int(64), &parquet.DeltaBinaryPacked))},
			{name: "Labels", Node: pprofLabels},
		})},
		{name: "DropFrames", Node: stringRef},
		{name: "KeepFrames", Node: stringRef},
		{name: "TimeNanos", Node: parquet.Timestamp(parquet.Nanosecond)},
		{name: "DurationNanos", Node: parquet.Int(64)},
		{name: "PeriodType", Node: parquet.Optional(sampleType)},
		{name: "Period", Node: parquet.Int(64)},
		{name: "Comments", Node: parquet.Repeated(stringRef)},
		{name: "DefaultSampleType", Node: parquet.Int(64)},
	})
	return s
}
