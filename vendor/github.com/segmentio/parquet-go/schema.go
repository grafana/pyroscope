package parquet

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/segmentio/parquet-go/compress"
	"github.com/segmentio/parquet-go/deprecated"
	"github.com/segmentio/parquet-go/encoding"
)

// Schema represents a parquet schema created from a Go value.
//
// Schema implements the Node interface to represent the root node of a parquet
// schema.
type Schema struct {
	name        string
	root        Node
	deconstruct deconstructFunc
	reconstruct reconstructFunc
	readRows    readRowsFunc
	mapping     columnMapping
	columns     [][]string
}

// SchemaOf constructs a parquet schema from a Go value.
//
// The function can construct parquet schemas from struct or pointer-to-struct
// values only. A panic is raised if a Go value of a different type is passed
// to this function.
//
// When creating a parquet Schema from a Go value, the struct fields may contain
// a "parquet" tag to describe properties of the parquet node. The "parquet" tag
// follows the conventional format of Go struct tags: a comma-separated list of
// values describe the options, with the first one defining the name of the
// parquet column.
//
// The following options are also supported in the "parquet" struct tag:
//
//	optional  | make the parquet column optional
//	snappy    | sets the parquet column compression codec to snappy
//	gzip      | sets the parquet column compression codec to gzip
//	brotli    | sets the parquet column compression codec to brotli
//	lz4       | sets the parquet column compression codec to lz4
//	zstd      | sets the parquet column compression codec to zstd
//	plain     | enables the plain encoding (no-op default)
//	dict      | enables dictionary encoding on the parquet column
//	delta     | enables delta encoding on the parquet column
//	list      | for slice types, use the parquet LIST logical type
//	enum      | for string types, use the parquet ENUM logical type
//	uuid      | for string and [16]byte types, use the parquet UUID logical type
//	decimal   | for int32, int64 and [n]byte types, use the parquet DECIMAL logical type
//	date      | for int32 types use the DATE logical type
//	timestamp | for int64 types use the TIMESTAMP logical type with millisecond precision
//
// The date logical type is an int32 value of the number of days since the unix epoch
//
// The decimal tag must be followed by two integer parameters, the first integer
// representing the scale and the second the precision; for example:
//
//	type Item struct {
//		Cost int64 `parquet:"cost,decimal(0:3)"`
//	}
//
// Invalid combination of struct tags and Go types, or repeating options will
// cause the function to panic.
//
// The schema name is the Go type name of the value.
func SchemaOf(model interface{}) *Schema {
	return schemaOf(dereference(reflect.TypeOf(model)))
}

var cachedSchemas sync.Map // map[reflect.Type]*Schema

func schemaOf(model reflect.Type) *Schema {
	cached, _ := cachedSchemas.Load(model)
	schema, _ := cached.(*Schema)
	if schema != nil {
		return schema
	}
	if model.Kind() != reflect.Struct {
		panic("cannot construct parquet schema from value of type " + model.String())
	}
	schema = NewSchema(model.Name(), nodeOf(model))
	if actual, loaded := cachedSchemas.LoadOrStore(model, schema); loaded {
		schema = actual.(*Schema)
	}
	return schema
}

// NewSchema constructs a new Schema object with the given name and root node.
//
// The function panics if Node contains more leaf columns than supported by the
// package (see parquet.MaxColumnIndex).
func NewSchema(name string, root Node) *Schema {
	mapping, columns := columnMappingOf(root)
	return &Schema{
		name:        name,
		root:        root,
		deconstruct: makeDeconstructFunc(root),
		reconstruct: makeReconstructFunc(root),
		readRows:    makeReadRowsFunc(root),
		mapping:     mapping,
		columns:     columns,
	}
}

func dereference(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

func makeDeconstructFunc(node Node) (deconstruct deconstructFunc) {
	if schema, _ := node.(*Schema); schema != nil {
		return schema.deconstruct
	}
	if !node.Leaf() {
		_, deconstruct = deconstructFuncOf(0, node)
	}
	return deconstruct
}

func makeReconstructFunc(node Node) (reconstruct reconstructFunc) {
	if schema, _ := node.(*Schema); schema != nil {
		return schema.reconstruct
	}
	if !node.Leaf() {
		_, reconstruct = reconstructFuncOf(0, node)
	}
	return reconstruct
}

func makeReadRowsFunc(node Node) readRowsFunc {
	_, readRows := readRowsFuncOf(node, 0, 0)
	return readRows
}

// ConfigureRowGroup satisfies the RowGroupOption interface, allowing Schema
// instances to be passed to row group constructors to pre-declare the schema of
// the output parquet file.
func (s *Schema) ConfigureRowGroup(config *RowGroupConfig) { config.Schema = s }

// ConfigureReader satisfies the ReaderOption interface, allowing Schema
// instances to be passed to NewReader to pre-declare the schema of rows
// read from the reader.
func (s *Schema) ConfigureReader(config *ReaderConfig) { config.Schema = s }

// ConfigureWriter satisfies the WriterOption interface, allowing Schema
// instances to be passed to NewWriter to pre-declare the schema of the
// output parquet file.
func (s *Schema) ConfigureWriter(config *WriterConfig) { config.Schema = s }

// String returns a parquet schema representation of s.
func (s *Schema) String() string { return sprint(s.name, s.root) }

// Name returns the name of s.
func (s *Schema) Name() string { return s.name }

// Type returns the parquet type of s.
func (s *Schema) Type() Type { return s.root.Type() }

// Optional returns false since the root node of a parquet schema is always required.
func (s *Schema) Optional() bool { return s.root.Optional() }

// Repeated returns false since the root node of a parquet schema is always required.
func (s *Schema) Repeated() bool { return s.root.Repeated() }

// Required returns true since the root node of a parquet schema is always required.
func (s *Schema) Required() bool { return s.root.Required() }

// Leaf returns true if the root node of the parquet schema is a leaf column.
func (s *Schema) Leaf() bool { return s.root.Leaf() }

// Fields returns the list of fields on the root node of the parquet schema.
func (s *Schema) Fields() []Field { return s.root.Fields() }

// Encoding returns the encoding set on the root node of the parquet schema.
func (s *Schema) Encoding() encoding.Encoding { return s.root.Encoding() }

// Compression returns the compression codec set on the root node of the parquet
// schema.
func (s *Schema) Compression() compress.Codec { return s.root.Compression() }

// GoType returns the Go type that best represents the schema.
func (s *Schema) GoType() reflect.Type { return s.root.GoType() }

// Deconstruct deconstructs a Go value and appends it to a row.
//
// The method panics is the structure of the go value does not match the
// parquet schema.
func (s *Schema) Deconstruct(row Row, value interface{}) Row {
	v := reflect.ValueOf(value)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			v = reflect.Value{}
			break
		}
		v = v.Elem()
	}
	if s.deconstruct != nil {
		row = s.deconstruct(row, levels{}, v)
	}
	return row
}

// Reconstruct reconstructs a Go value from a row.
//
// The go value passed as first argument must be a non-nil pointer for the
// row to be decoded into.
//
// The method panics if the structure of the go value and parquet row do not
// match.
func (s *Schema) Reconstruct(value interface{}, row Row) error {
	v := reflect.ValueOf(value)
	if !v.IsValid() {
		panic("cannot reconstruct row into go value of type <nil>")
	}
	if v.Kind() != reflect.Ptr {
		panic("cannot reconstruct row into go value of non-pointer type " + v.Type().String())
	}
	if v.IsNil() {
		panic("cannot reconstruct row into nil pointer of type " + v.Type().String())
	}
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}
	var err error
	if s.reconstruct != nil {
		row, err = s.reconstruct(v, levels{}, row)
		if len(row) > 0 && err == nil {
			err = fmt.Errorf("%d values remain unused after reconstructing go value of type %s from parquet row", len(row), v.Type())
		}
	}
	return err
}

// Lookup returns the leaf column at the given path.
//
// The path is the sequence of column names identifying a leaf column (not
// including the root).
//
// If the path was not found in the mapping, or if it did not represent a
// leaf column of the parquet schema, the boolean will be false.
func (s *Schema) Lookup(path ...string) (LeafColumn, bool) {
	leaf := s.mapping.lookup(path)
	return LeafColumn{
		Node:               leaf.node,
		Path:               leaf.path,
		ColumnIndex:        int(leaf.columnIndex),
		MaxRepetitionLevel: int(leaf.maxRepetitionLevel),
		MaxDefinitionLevel: int(leaf.maxDefinitionLevel),
	}, leaf.node != nil
}

// Columns returns the list of column paths available in the schema.
//
// The method always returns the same slice value across calls to ColumnPaths,
// applications should treat it as immutable.
func (s *Schema) Columns() [][]string {
	return s.columns
}

func (s *Schema) forEachNode(do func(name string, node Node)) {
	forEachNodeOf(s.Name(), s, do)
}

type structNode struct {
	gotype reflect.Type
	fields []structField
}

func structNodeOf(t reflect.Type) *structNode {
	// Collect struct fields first so we can order them before generating the
	// column indexes.
	fields := structFieldsOf(t)

	s := &structNode{
		gotype: t,
		fields: make([]structField, len(fields)),
	}

	for i := range fields {
		s.fields[i] = makeStructField(fields[i])
	}

	return s
}

func structFieldsOf(t reflect.Type) []reflect.StructField {
	fields := appendStructFields(t, nil, nil, 0)

	for i := range fields {
		f := &fields[i]

		if tag := f.Tag.Get("parquet"); tag != "" {
			name, _ := split(tag)
			if name != "" {
				f.Name = name
			}
		}
	}

	return fields
}

func appendStructFields(t reflect.Type, fields []reflect.StructField, index []int, offset uintptr) []reflect.StructField {
	for i, n := 0, t.NumField(); i < n; i++ {
		fieldIndex := index[:len(index):len(index)]
		fieldIndex = append(fieldIndex, i)

		f := t.Field(i)
		f.Offset += offset

		if f.Anonymous {
			fields = appendStructFields(f.Type, fields, fieldIndex, f.Offset)
		} else if f.IsExported() {
			f.Index = fieldIndex
			fields = append(fields, f)
		}
	}
	return fields
}

func (s *structNode) Optional() bool { return false }

func (s *structNode) Repeated() bool { return false }

func (s *structNode) Required() bool { return true }

func (s *structNode) Leaf() bool { return false }

func (s *structNode) Encoding() encoding.Encoding { return nil }

func (s *structNode) Compression() compress.Codec { return nil }

func (s *structNode) GoType() reflect.Type { return s.gotype }

func (s *structNode) String() string { return sprint("", s) }

func (s *structNode) Type() Type { return groupType{} }

func (s *structNode) Fields() []Field {
	fields := make([]Field, len(s.fields))
	for i := range s.fields {
		fields[i] = &s.fields[i]
	}
	return fields
}

// fieldByIndex is like reflect.Value.FieldByIndex but returns the zero-value of
// reflect.Value if one of the fields was a nil pointer instead of panicking.
func fieldByIndex(v reflect.Value, index []int) reflect.Value {
	for _, i := range index {
		if v = v.Field(i); v.Kind() == reflect.Ptr {
			if v.IsNil() {
				v = reflect.Value{}
				break
			} else {
				v = v.Elem()
			}
		}
	}
	return v
}

type structField struct {
	Node
	name  string
	index []int
}

func (f *structField) Name() string { return f.name }

func (f *structField) Value(base reflect.Value) reflect.Value {
	switch base.Kind() {
	case reflect.Map:
		return base.MapIndex(reflect.ValueOf(&f.name).Elem())
	default:
		if len(f.index) == 1 {
			return base.Field(f.index[0])
		} else {
			return fieldByIndex(base, f.index)
		}
	}
	return reflect.Value{}
}

func structFieldString(f reflect.StructField) string {
	return f.Name + " " + f.Type.String() + " " + string(f.Tag)
}

func throwInvalidFieldTag(f reflect.StructField, tag string) {
	panic("struct has invalid '" + tag + "' parquet tag: " + structFieldString(f))
}

func throwUnknownFieldTag(f reflect.StructField, tag string) {
	panic("struct has unrecognized '" + tag + "' parquet tag: " + structFieldString(f))
}

func throwInvalidStructField(msg string, field reflect.StructField) {
	panic(msg + ": " + structFieldString(field))
}

func makeStructField(f reflect.StructField) structField {
	var (
		field      = structField{name: f.Name, index: f.Index}
		optional   bool
		list       bool
		encoded    encoding.Encoding
		compressed compress.Codec
	)

	setNode := func(node Node) {
		if field.Node != nil {
			throwInvalidStructField("struct field has multiple logical parquet types declared", f)
		}
		field.Node = node
	}

	setOptional := func() {
		if optional {
			throwInvalidStructField("struct field has multiple declaration of the optional tag", f)
		}
		optional = true
	}

	setList := func() {
		if list {
			throwInvalidStructField("struct field has multiple declaration of the list tag", f)
		}
		list = true
	}

	setEncoding := func(e encoding.Encoding) {
		if encoded != nil {
			throwInvalidStructField("struct field has encoding declared multiple times", f)
		}
		encoded = e
	}

	setCompression := func(c compress.Codec) {
		if compressed != nil {
			throwInvalidStructField("struct field has compression codecs declared multiple times", f)
		}
		compressed = c
	}

	forEachStructTagOption(f.Tag, func(option, args string) {
		switch option {
		case "optional":
			setOptional()

		case "snappy":
			setCompression(&Snappy)

		case "gzip":
			setCompression(&Gzip)

		case "brotli":
			setCompression(&Brotli)

		case "lz4":
			setCompression(&Lz4Raw)

		case "zstd":
			setCompression(&Zstd)

		case "uncompressed":
			setCompression(&Uncompressed)

		case "plain":
			setEncoding(&Plain)

		case "dict":
			setEncoding(&RLEDictionary)

		case "delta":
			switch f.Type.Kind() {
			case reflect.Int, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint32, reflect.Uint64:
				setEncoding(&DeltaBinaryPacked)
			case reflect.String:
				setEncoding(&DeltaByteArray)
			case reflect.Slice:
				if f.Type.Elem().Kind() == reflect.Uint8 { // []byte?
					setEncoding(&DeltaByteArray)
				} else {
					throwInvalidFieldTag(f, option)
				}
			case reflect.Array:
				if f.Type.Elem().Kind() == reflect.Uint8 { // [N]byte?
					setEncoding(&DeltaByteArray)
				} else {
					throwInvalidFieldTag(f, option)
				}
			default:
				throwInvalidFieldTag(f, option)
			}

		case "list":
			switch f.Type.Kind() {
			case reflect.Slice:
				element := nodeOf(f.Type.Elem())
				setNode(element)
				setList()
			default:
				throwInvalidFieldTag(f, option)
			}

		case "enum":
			switch f.Type.Kind() {
			case reflect.String:
				setNode(Enum())
			default:
				throwInvalidFieldTag(f, option)
			}

		case "uuid":
			switch f.Type.Kind() {
			case reflect.Array:
				if f.Type.Elem().Kind() != reflect.Uint8 || f.Type.Len() != 16 {
					throwInvalidFieldTag(f, option)
				}
			default:
				throwInvalidFieldTag(f, option)
			}

		case "decimal":
			scale, precision, err := parseDecimalArgs(args)
			if err != nil {
				throwInvalidFieldTag(f, option+args)
			}
			var baseType Type
			switch f.Type.Kind() {
			case reflect.Int32:
				baseType = Int32Type
			case reflect.Int64:
				baseType = Int64Type
			case reflect.Array:
				baseType = FixedLenByteArrayType(calcDecimalFixedLenByteArraySize(precision))
			default:
				throwInvalidFieldTag(f, option)
			}

			setNode(Decimal(scale, precision, baseType))
		case "date":
			switch f.Type.Kind() {
			case reflect.Int32:
				setNode(Date())
			default:
				throwInvalidFieldTag(f, option)
			}
		case "timestamp":
			switch f.Type.Kind() {
			case reflect.Int64:
				setNode(Timestamp(Millisecond))
			default:
				throwInvalidFieldTag(f, option)
			}
		default:
			throwUnknownFieldTag(f, option)
		}
	})

	if field.Node == nil {
		field.Node = nodeOf(f.Type)
	}

	if compressed != nil {
		field.Node = Compressed(field.Node, compressed)
	}

	if encoded != nil {
		field.Node = Encoded(field.Node, encoded)
	}

	if list {
		field.Node = List(field.Node)
	}

	if optional {
		field.Node = Optional(field.Node)
	}

	return field
}

func forEachStructTagOption(st reflect.StructTag, do func(option, args string)) {
	if tag := st.Get("parquet"); tag != "" {
		_, tag = split(tag) // skip the field name
		for tag != "" {
			option := ""
			option, tag = split(tag)
			option, args := splitOptionArgs(option)
			do(option, args)
		}
	}
}

func nodeOf(t reflect.Type) Node {
	switch t {
	case reflect.TypeOf(deprecated.Int96{}):
		return Leaf(Int96Type)
	case reflect.TypeOf(uuid.UUID{}):
		return UUID()
	}

	var n Node
	switch t.Kind() {
	case reflect.Bool:
		n = Leaf(BooleanType)

	case reflect.Int, reflect.Int64:
		n = Int(64)

	case reflect.Int8, reflect.Int16, reflect.Int32:
		n = Int(t.Bits())

	case reflect.Uint, reflect.Uintptr, reflect.Uint64:
		n = Uint(64)

	case reflect.Uint8, reflect.Uint16, reflect.Uint32:
		n = Uint(t.Bits())

	case reflect.Float32:
		n = Leaf(FloatType)

	case reflect.Float64:
		n = Leaf(DoubleType)

	case reflect.String:
		n = String()

	case reflect.Ptr:
		n = Optional(nodeOf(t.Elem()))

	case reflect.Slice:
		if elem := t.Elem(); elem.Kind() == reflect.Uint8 { // []byte?
			n = Leaf(ByteArrayType)
		} else {
			n = Repeated(nodeOf(elem))
		}

	case reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			n = Leaf(FixedLenByteArrayType(t.Len()))
		}

	case reflect.Map:
		n = Map(nodeOf(t.Key()), nodeOf(t.Elem()))

	case reflect.Struct:
		return structNodeOf(t)
	}

	if n == nil {
		panic("cannot create parquet node from go value of type " + t.String())
	}

	return &goNode{Node: n, gotype: t}
}

func split(s string) (head, tail string) {
	if i := strings.IndexByte(s, ','); i < 0 {
		head = s
	} else {
		head, tail = s[:i], s[i+1:]
	}
	return
}

func splitOptionArgs(s string) (option, args string) {
	if i := strings.IndexByte(s, '('); i >= 0 {
		return s[:i], s[i:]
	} else {
		return s, "()"
	}
}

func parseDecimalArgs(args string) (scale, precision int, err error) {
	if !strings.HasPrefix(args, "(") || !strings.HasSuffix(args, ")") {
		return 0, 0, fmt.Errorf("malformed decimal args: %s", args)
	}
	args = strings.TrimPrefix(args, "(")
	args = strings.TrimSuffix(args, ")")
	parts := strings.Split(args, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("malformed decimal args: (%s)", args)
	}
	s, err := strconv.ParseInt(parts[0], 10, 32)
	if err != nil {
		return 0, 0, err
	}
	p, err := strconv.ParseInt(parts[1], 10, 32)
	if err != nil {
		return 0, 0, err
	}
	return int(s), int(p), nil
}

type goNode struct {
	Node
	gotype reflect.Type
}

func (n *goNode) GoType() reflect.Type { return n.gotype }

var (
	_ RowGroupOption = (*Schema)(nil)
	_ ReaderOption   = (*Schema)(nil)
	_ WriterOption   = (*Schema)(nil)
)
