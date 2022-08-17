package parquet

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

// Group allows to write a custom ordered schema. As opposed to parquet.Group which orders fields alphabethical as it is based on a map.
type Group []parquet.Field

func (g Group) String() string {
	s := new(strings.Builder)
	if err := parquet.PrintSchema(s, "", g); err != nil {
		panic(err.Error())
	}
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

func NewGroupField(name string, node parquet.Node) parquet.Field {
	return &groupField{
		Node: node,
		name: name,
	}
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

func (groupType) NewDictionary(int, int, encoding.Values) parquet.Dictionary {
	panic("cannot create dictionary from parquet group")
}

func (t groupType) NewColumnBuffer(int, int) parquet.ColumnBuffer {
	panic("cannot create column buffer from parquet group")
}

func (t groupType) NewPage(int, int, encoding.Values) parquet.Page {
	panic("cannot create page from parquet group")
}

func (t groupType) NewValues(_ []byte, _ []uint32) encoding.Values {
	panic("cannot create values from parquet group")
}

func (groupType) Encode(_ []byte, _ encoding.Values, _ encoding.Encoding) ([]byte, error) {
	panic("cannot encode parquet group")
}

func (groupType) Decode(_ encoding.Values, _ []byte, _ encoding.Encoding) (encoding.Values, error) {
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
	if base.Kind() == reflect.Ptr {
		if base.IsNil() {
			base.Set(reflect.New(base.Type().Elem()))
		}
		return f.Value(base.Elem())
	}
	return base.FieldByName(f.name)
}
