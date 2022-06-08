package parquet

import (
	"reflect"
	"sort"
	"unicode"
	"unicode/utf8"

	"github.com/segmentio/parquet-go/compress"
	"github.com/segmentio/parquet-go/deprecated"
	"github.com/segmentio/parquet-go/encoding"
	"github.com/segmentio/parquet-go/format"
)

// Node values represent nodes of a parquet schema.
//
// Nodes carry the type of values, as well as properties like whether the values
// are optional or repeat. Nodes with one or more children represent parquet
// groups and therefore do not have a logical type.
//
// Nodes are immutable values and therefore safe to use concurrently from
// multiple goroutines.
type Node interface {
	// Returns a human-readable representation of the parquet node.
	String() string

	// For leaf nodes, returns the type of values of the parquet column.
	//
	// Calling this method on non-leaf nodes will panic.
	Type() Type

	// Returns whether the parquet column is optional.
	Optional() bool

	// Returns whether the parquet column is repeated.
	Repeated() bool

	// Returns whether the parquet column is required.
	Required() bool

	// Returns true if this a leaf node.
	Leaf() bool

	// Returns a mapping of the node's fields.
	//
	// As an optimization, the same slices may be returned by multiple calls to
	// this method, programs must treat the returned values as immutable.
	//
	// This method returns an empty mapping when called on leaf nodes.
	Fields() []Field

	// Returns the encoding used by the node.
	//
	// The method may return nil to indicate that no specific encoding was
	// configured on the node, in which case a default encoding might be used.
	Encoding() encoding.Encoding

	// Returns compression codec used by the node.
	//
	// The method may return nil to indicate that no specific compression codec
	// was configured on the node, in which case a default compression might be
	// used.
	Compression() compress.Codec

	// Returns the Go type that best represents the parquet node.
	//
	// For leaf nodes, this will be one of bool, int32, int64, deprecated.Int96,
	// float32, float64, string, []byte, or [N]byte.
	//
	// For groups, the method returns a struct type.
	//
	// If the method is called on a repeated node, the method returns a slice of
	// the underlying type.
	//
	// For optional nodes, the method returns a pointer of the underlying type.
	//
	// For nodes that were constructed from Go values (e.g. using SchemaOf), the
	// method returns the original Go type.
	GoType() reflect.Type
}

// Field instances represent fields of a parquet node, which associate a node to
// their name in their parent node.
type Field interface {
	Node

	// Returns the name of this field in its parent node.
	Name() string

	// Given a reference to the Go value matching the structure of the parent
	// node, returns the Go value of the field.
	Value(base reflect.Value) reflect.Value
}

// Encoded wraps the node passed as argument to use the given encoding.
//
// The function panics if it is called on a non-leaf node, or if the
// encoding does not support the node type.
func Encoded(node Node, encoding encoding.Encoding) Node {
	if !node.Leaf() {
		panic("cannot add encoding to a non-leaf node")
	}
	if encoding != nil {
		kind := node.Type().Kind()
		if !canEncode(encoding, kind) {
			panic("cannot apply " + encoding.Encoding().String() + " to node of type " + kind.String())
		}
	}
	return &encodedNode{
		Node:     node,
		encoding: encoding,
	}
}

type encodedNode struct {
	Node
	encoding encoding.Encoding
}

func (n *encodedNode) Encoding() encoding.Encoding {
	return n.encoding
}

// Compressed wraps the node passed as argument to use the given compression
// codec.
//
// If the codec is nil, the node's compression is left unchanged.
//
// The function panics if it is called on a non-leaf node.
func Compressed(node Node, codec compress.Codec) Node {
	if !node.Leaf() {
		panic("cannot add compression codec to a non-leaf node")
	}
	return &compressedNode{
		Node:  node,
		codec: codec,
	}
}

type compressedNode struct {
	Node
	codec compress.Codec
}

func (n *compressedNode) Compression() compress.Codec {
	return n.codec
}

// Optional wraps the given node to make it optional.
func Optional(node Node) Node { return &optionalNode{node} }

type optionalNode struct{ Node }

func (opt *optionalNode) Optional() bool       { return true }
func (opt *optionalNode) Repeated() bool       { return false }
func (opt *optionalNode) Required() bool       { return false }
func (opt *optionalNode) GoType() reflect.Type { return reflect.PtrTo(opt.Node.GoType()) }

// Repeated wraps the given node to make it repeated.
func Repeated(node Node) Node { return &repeatedNode{node} }

type repeatedNode struct{ Node }

func (rep *repeatedNode) Optional() bool       { return false }
func (rep *repeatedNode) Repeated() bool       { return true }
func (rep *repeatedNode) Required() bool       { return false }
func (rep *repeatedNode) GoType() reflect.Type { return reflect.SliceOf(rep.Node.GoType()) }

// Required wraps the given node to make it required.
func Required(node Node) Node { return &requiredNode{node} }

type requiredNode struct{ Node }

func (req *requiredNode) Optional() bool       { return false }
func (req *requiredNode) Repeated() bool       { return false }
func (req *requiredNode) Required() bool       { return true }
func (req *requiredNode) GoType() reflect.Type { return req.Node.GoType() }

type node struct{}

// Leaf returns a leaf node of the given type.
func Leaf(typ Type) Node {
	return &leafNode{typ: typ}
}

type leafNode struct{ typ Type }

func (n *leafNode) String() string { return sprint("", n) }

func (n *leafNode) Type() Type { return n.typ }

func (n *leafNode) Optional() bool { return false }

func (n *leafNode) Repeated() bool { return false }

func (n *leafNode) Required() bool { return true }

func (n *leafNode) Leaf() bool { return true }

func (n *leafNode) Fields() []Field { return nil }

func (n *leafNode) Encoding() encoding.Encoding { return nil }

func (n *leafNode) Compression() compress.Codec { return nil }

func (n *leafNode) GoType() reflect.Type { return goTypeOfLeaf(n) }

var repetitionTypes = [...]format.FieldRepetitionType{
	0: format.Required,
	1: format.Optional,
	2: format.Repeated,
}

func fieldRepetitionTypePtrOf(node Node) *format.FieldRepetitionType {
	switch {
	case node.Required():
		return &repetitionTypes[format.Required]
	case node.Optional():
		return &repetitionTypes[format.Optional]
	case node.Repeated():
		return &repetitionTypes[format.Repeated]
	default:
		return nil
	}
}

func fieldRepetitionTypeOf(node Node) format.FieldRepetitionType {
	switch {
	case node.Optional():
		return format.Optional
	case node.Repeated():
		return format.Repeated
	default:
		return format.Required
	}
}

type Group map[string]Node

func (g Group) String() string { return sprint("", g) }

func (g Group) Type() Type { return groupType{} }

func (g Group) Optional() bool { return false }

func (g Group) Repeated() bool { return false }

func (g Group) Required() bool { return true }

func (g Group) Leaf() bool { return false }

func (g Group) Fields() []Field {
	groupFields := make([]groupField, 0, len(g))
	for name, node := range g {
		groupFields = append(groupFields, groupField{
			Node: node,
			name: name,
		})
	}
	sort.Slice(groupFields, func(i, j int) bool {
		return groupFields[i].name < groupFields[j].name
	})
	fields := make([]Field, len(groupFields))
	for i := range groupFields {
		fields[i] = &groupFields[i]
	}
	return fields
}

func (g Group) Encoding() encoding.Encoding { return nil }

func (g Group) Compression() compress.Codec { return nil }

func (g Group) GoType() reflect.Type { return goTypeOfGroup(g) }

type groupField struct {
	Node
	name string
}

func (f *groupField) Name() string { return f.name }

func (f *groupField) Value(base reflect.Value) reflect.Value {
	return base.MapIndex(reflect.ValueOf(&f.name).Elem())
}

func goTypeOf(node Node) reflect.Type {
	switch {
	case node.Optional():
		return goTypeOfOptional(node)
	case node.Repeated():
		return goTypeOfRepeated(node)
	default:
		return goTypeOfRequired(node)
	}
}

func goTypeOfOptional(node Node) reflect.Type {
	return reflect.PtrTo(goTypeOfRequired(node))
}

func goTypeOfRepeated(node Node) reflect.Type {
	return reflect.SliceOf(goTypeOfRequired(node))
}

func goTypeOfRequired(node Node) reflect.Type {
	if node.Leaf() {
		return goTypeOfLeaf(node)
	} else {
		return goTypeOfGroup(node)
	}
}

func goTypeOfLeaf(node Node) reflect.Type {
	t := node.Type()
	if convertibleType, ok := t.(interface{ GoType() reflect.Type }); ok {
		return convertibleType.GoType()
	}
	switch t.Kind() {
	case Boolean:
		return reflect.TypeOf(false)
	case Int32:
		return reflect.TypeOf(int32(0))
	case Int64:
		return reflect.TypeOf(int64(0))
	case Int96:
		return reflect.TypeOf(deprecated.Int96{})
	case Float:
		return reflect.TypeOf(float32(0))
	case Double:
		return reflect.TypeOf(float64(0))
	case ByteArray:
		return reflect.TypeOf(([]byte)(nil))
	case FixedLenByteArray:
		return reflect.ArrayOf(t.Length(), reflect.TypeOf(byte(0)))
	default:
		panic("BUG: parquet type returned an unsupported kind")
	}
}

func goTypeOfGroup(node Node) reflect.Type {
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

func exportedStructFieldName(name string) string {
	firstRune, size := utf8.DecodeRuneInString(name)
	return string([]rune{unicode.ToUpper(firstRune)}) + name[size:]
}

func isList(node Node) bool {
	logicalType := node.Type().LogicalType()
	return logicalType != nil && logicalType.List != nil
}

func isMap(node Node) bool {
	logicalType := node.Type().LogicalType()
	return logicalType != nil && logicalType.Map != nil
}

func numLeafColumnsOf(node Node) int16 {
	return makeColumnIndex(numLeafColumns(node, 0))
}

func numLeafColumns(node Node, columnIndex int) int {
	if node.Leaf() {
		return columnIndex + 1
	}
	for _, field := range node.Fields() {
		columnIndex = numLeafColumns(field, columnIndex)
	}
	return columnIndex
}

func listElementOf(node Node) Node {
	if !node.Leaf() {
		if list := childByName(node, "list"); list != nil {
			if elem := childByName(list, "element"); elem != nil {
				return elem
			}
		}
	}
	panic("node with logical type LIST is not composed of a repeated .list.element")
}

func mapKeyValueOf(node Node) Node {
	if !node.Leaf() && (node.Required() || node.Optional()) {
		if keyValue := childByName(node, "key_value"); keyValue != nil && !keyValue.Leaf() && keyValue.Repeated() {
			k := childByName(keyValue, "key")
			v := childByName(keyValue, "value")
			if k != nil && v != nil && k.Required() {
				return keyValue
			}
		}
	}
	panic("node with logical type MAP is not composed of a repeated .key_value group with key and value fields")
}

func encodingOf(node Node) encoding.Encoding {
	encoding := node.Encoding()
	// The parquet-format documentation states that the
	// DELTA_LENGTH_BYTE_ARRAY is always preferred to PLAIN when
	// encoding BYTE_ARRAY values. We apply it as a default if
	// none were explicitly specified, which gives the application
	// the opportunity to override this behavior if needed.
	//
	// https://github.com/apache/parquet-format/blob/master/Encodings.md#delta-length-byte-array-delta_length_byte_array--6
	if node.Type().Kind() == ByteArray && encoding == nil {
		encoding = &DeltaLengthByteArray
	}
	if encoding == nil {
		encoding = &Plain
	}
	return encoding
}

func forEachNodeOf(name string, node Node, do func(string, Node)) {
	do(name, node)

	for _, f := range node.Fields() {
		forEachNodeOf(f.Name(), f, do)
	}
}

func childByName(node Node, name string) Node {
	for _, f := range node.Fields() {
		if f.Name() == name {
			return f
		}
	}
	return nil
}

func nodesAreEqual(node1, node2 Node) bool {
	if node1.Leaf() {
		return node2.Leaf() && leafNodesAreEqual(node1, node2)
	} else {
		return !node2.Leaf() && groupNodesAreEqual(node1, node2)
	}
}

func typesAreEqual(node1, node2 Node) bool {
	return node1.Type().Kind() == node2.Type().Kind()
}

func repetitionsAreEqual(node1, node2 Node) bool {
	return node1.Optional() == node2.Optional() && node1.Repeated() == node2.Repeated()
}

func leafNodesAreEqual(node1, node2 Node) bool {
	return typesAreEqual(node1, node2) && repetitionsAreEqual(node1, node2)
}

func groupNodesAreEqual(node1, node2 Node) bool {
	fields1 := node1.Fields()
	fields2 := node2.Fields()

	if len(fields1) != len(fields2) {
		return false
	}

	for i := range fields1 {
		f1 := fields1[i]
		f2 := fields2[i]

		if f1.Name() != f2.Name() {
			return false
		}

		if !nodesAreEqual(f1, f2) {
			return false
		}
	}

	return true
}
