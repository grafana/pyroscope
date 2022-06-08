package parquet

import (
	"bytes"
	"io"

	"github.com/segmentio/parquet-go/deprecated"
	"github.com/segmentio/parquet-go/encoding"
	"github.com/segmentio/parquet-go/encoding/plain"
	"github.com/segmentio/parquet-go/internal/bits"
)

const (
	// Completely arbitrary, feel free to adjust if a different value would be
	// more representative of the map implementation in Go.
	mapSizeOverheadPerItem = 8
)

// The Dictionary interface represents type-specific implementations of parquet
// dictionaries.
//
// Programs can instantiate dictionaries by call the NewDictionary method of a
// Type object.
type Dictionary interface {
	// Returns the type that the dictionary was created from.
	Type() Type

	// Returns the number of value indexed in the dictionary.
	Len() int

	// Returns the dictionary value at the given index.
	Index(index int32) Value

	// Inserts values from the second slice to the dictionary and writes the
	// indexes at which each value was inserted to the first slice.
	//
	// The method panics if the length of the indexes slice is smaller than the
	// length of the values slice.
	Insert(indexes []int32, values []Value)

	// Given an array of dictionary indexes, lookup the values into the array
	// of values passed as second argument.
	//
	// The method panics if len(indexes) > len(values), or one of the indexes
	// is negative or greater than the highest index in the dictionary.
	Lookup(indexes []int32, values []Value)

	// Returns the min and max values found in the given indexes.
	Bounds(indexed []int32) (min, max Value)

	// Resets the dictionary to its initial state, removing all values.
	Reset()

	// Returns a BufferedPage representing the content of the dictionary.
	//
	// The returned page shares the underlying memory of the buffer, it remains
	// valid to use until the dictionary's Reset method is called.
	Page() BufferedPage
}

// The boolean dictionary always contains two values for true and false.
type booleanDictionary struct {
	booleanPage
	index map[bool]int32
}

func newBooleanDictionary(typ Type, columnIndex int16, numValues int32, values []byte) *booleanDictionary {
	return &booleanDictionary{
		booleanPage: booleanPage{
			typ:         typ,
			bits:        values[:bits.ByteCount(uint(numValues))],
			numValues:   numValues,
			columnIndex: ^columnIndex,
		},
	}
}

func (d *booleanDictionary) Type() Type { return newIndexedType(d.typ, d) }

func (d *booleanDictionary) Len() int { return int(d.numValues) }

func (d *booleanDictionary) Index(i int32) Value { return makeValueBoolean(d.valueAt(int(i))) }

func (d *booleanDictionary) Insert(indexes []int32, values []Value) {
	_ = indexes[:len(values)]

	if d.index == nil {
		d.index = make(map[bool]int32, cap(d.bits))
		for i := 0; i < int(d.numValues); i++ {
			d.index[d.valueAt(i)] = int32(i)
		}
	}

	for i, v := range values {
		value := v.Boolean()

		index, exists := d.index[value]
		if !exists {
			index = d.numValues
			d.bits = plain.AppendBoolean(d.bits, int(index), value)
			d.index[value] = index
			d.numValues++
		}

		indexes[i] = index
	}
}

func (d *booleanDictionary) Lookup(indexes []int32, values []Value) {
	for i, j := range indexes {
		values[i] = d.Index(j)
	}
}

func (d *booleanDictionary) Bounds(indexes []int32) (min, max Value) {
	if len(indexes) > 0 {
		hasFalse, hasTrue := false, false

		for _, i := range indexes {
			v := d.valueAt(int(i))
			if v {
				hasTrue = true
			} else {
				hasFalse = true
			}
			if hasTrue && hasFalse {
				break
			}
		}

		min = makeValueBoolean(!hasFalse)
		max = makeValueBoolean(hasTrue)
	}
	return min, max
}

func (d *booleanDictionary) Reset() {
	d.bits = d.bits[:0]
	d.offset = 0
	d.numValues = 0
	d.index = nil
}

func (d *booleanDictionary) Page() BufferedPage {
	return &d.booleanPage
}

type int32Dictionary struct {
	int32Page
	index map[int32]int32
}

func newInt32Dictionary(typ Type, columnIndex int16, numValues int32, values []byte) *int32Dictionary {
	return &int32Dictionary{
		int32Page: int32Page{
			typ:         typ,
			values:      bits.BytesToInt32(values)[:numValues],
			columnIndex: ^columnIndex,
		},
	}
}

func (d *int32Dictionary) Type() Type { return newIndexedType(d.typ, d) }

func (d *int32Dictionary) Len() int { return len(d.values) }

func (d *int32Dictionary) Index(i int32) Value { return makeValueInt32(d.values[i]) }

func (d *int32Dictionary) Insert(indexes []int32, values []Value) {
	_ = indexes[:len(values)]

	if d.index == nil {
		d.index = make(map[int32]int32, cap(d.values))
		for i, v := range d.values {
			d.index[v] = int32(i)
		}
	}

	for i, v := range values {
		value := v.Int32()

		index, exists := d.index[value]
		if !exists {
			index = int32(len(d.values))
			d.values = append(d.values, value)
			d.index[value] = index
		}

		indexes[i] = index
	}
}

func (d *int32Dictionary) Lookup(indexes []int32, values []Value) {
	for i, j := range indexes {
		values[i] = d.Index(j)
	}
}

func (d *int32Dictionary) Bounds(indexes []int32) (min, max Value) {
	if len(indexes) > 0 {
		minValue := d.values[indexes[0]]
		maxValue := minValue

		for _, i := range indexes[1:] {
			value := d.values[i]
			switch {
			case value < minValue:
				minValue = value
			case value > maxValue:
				maxValue = value
			}
		}

		min = makeValueInt32(minValue)
		max = makeValueInt32(maxValue)
	}
	return min, max
}

func (d *int32Dictionary) Reset() {
	d.values = d.values[:0]
	d.index = nil
}

func (d *int32Dictionary) Page() BufferedPage {
	return &d.int32Page
}

type int64Dictionary struct {
	int64Page
	index map[int64]int32
}

func newInt64Dictionary(typ Type, columnIndex int16, numValues int32, values []byte) *int64Dictionary {
	return &int64Dictionary{
		int64Page: int64Page{
			typ:         typ,
			values:      bits.BytesToInt64(values)[:numValues],
			columnIndex: ^columnIndex,
		},
	}
}

func (d *int64Dictionary) Type() Type { return newIndexedType(d.typ, d) }

func (d *int64Dictionary) Len() int { return len(d.values) }

func (d *int64Dictionary) Index(i int32) Value { return makeValueInt64(d.values[i]) }

func (d *int64Dictionary) Insert(indexes []int32, values []Value) {
	_ = indexes[:len(values)]

	if d.index == nil {
		d.index = make(map[int64]int32, cap(d.values))
		for i, v := range d.values {
			d.index[v] = int32(i)
		}
	}

	for i, v := range values {
		value := v.Int64()

		index, exists := d.index[value]
		if !exists {
			index = int32(len(d.values))
			d.values = append(d.values, value)
			d.index[value] = index
		}

		indexes[i] = index
	}
}

func (d *int64Dictionary) Lookup(indexes []int32, values []Value) {
	for i, j := range indexes {
		values[i] = d.Index(j)
	}
}

func (d *int64Dictionary) Bounds(indexes []int32) (min, max Value) {
	if len(indexes) > 0 {
		minValue := d.values[indexes[0]]
		maxValue := minValue

		for _, i := range indexes[1:] {
			value := d.values[i]
			switch {
			case value < minValue:
				minValue = value
			case value > maxValue:
				maxValue = value
			}
		}

		min = makeValueInt64(minValue)
		max = makeValueInt64(maxValue)
	}
	return min, max
}

func (d *int64Dictionary) Reset() {
	d.values = d.values[:0]
	d.index = nil
}

func (d *int64Dictionary) Page() BufferedPage {
	return &d.int64Page
}

type int96Dictionary struct {
	int96Page
	index map[deprecated.Int96]int32
}

func newInt96Dictionary(typ Type, columnIndex int16, numValues int32, values []byte) *int96Dictionary {
	return &int96Dictionary{
		int96Page: int96Page{
			typ:         typ,
			values:      deprecated.BytesToInt96(values)[:numValues],
			columnIndex: ^columnIndex,
		},
	}
}

func (d *int96Dictionary) Type() Type { return newIndexedType(d.typ, d) }

func (d *int96Dictionary) Len() int { return len(d.values) }

func (d *int96Dictionary) Index(i int32) Value { return makeValueInt96(d.values[i]) }

func (d *int96Dictionary) Insert(indexes []int32, values []Value) {
	_ = indexes[:len(values)]

	if d.index == nil {
		d.index = make(map[deprecated.Int96]int32, cap(d.values))
		for i, v := range d.values {
			d.index[v] = int32(i)
		}
	}

	for i, v := range values {
		value := v.Int96()

		index, exists := d.index[value]
		if !exists {
			index = int32(len(d.values))
			d.values = append(d.values, value)
			d.index[value] = index
		}

		indexes[i] = index
	}
}

func (d *int96Dictionary) Lookup(indexes []int32, values []Value) {
	for i, j := range indexes {
		values[i] = d.Index(j)
	}
}

func (d *int96Dictionary) Bounds(indexes []int32) (min, max Value) {
	if len(indexes) > 0 {
		minValue := d.values[indexes[0]]
		maxValue := minValue

		for _, i := range indexes[1:] {
			value := d.values[i]
			switch {
			case value.Less(minValue):
				minValue = value
			case maxValue.Less(value):
				maxValue = value
			}
		}

		min = makeValueInt96(minValue)
		max = makeValueInt96(maxValue)
	}
	return min, max
}

func (d *int96Dictionary) Reset() {
	d.values = d.values[:0]
	d.index = nil
}

func (d *int96Dictionary) Page() BufferedPage {
	return &d.int96Page
}

type floatDictionary struct {
	floatPage
	index map[float32]int32
}

func newFloatDictionary(typ Type, columnIndex int16, numValues int32, values []byte) *floatDictionary {
	return &floatDictionary{
		floatPage: floatPage{
			typ:         typ,
			values:      bits.BytesToFloat32(values)[:numValues],
			columnIndex: ^columnIndex,
		},
	}
}

func (d *floatDictionary) Type() Type { return newIndexedType(d.typ, d) }

func (d *floatDictionary) Len() int { return len(d.values) }

func (d *floatDictionary) Index(i int32) Value { return makeValueFloat(d.values[i]) }

func (d *floatDictionary) Insert(indexes []int32, values []Value) {
	_ = indexes[:len(values)]

	if d.index == nil {
		d.index = make(map[float32]int32, cap(d.values))
		for i, v := range d.values {
			d.index[v] = int32(i)
		}
	}

	for i, v := range values {
		value := v.Float()

		index, exists := d.index[value]
		if !exists {
			index = int32(len(d.values))
			d.values = append(d.values, value)
			d.index[value] = index
		}

		indexes[i] = index
	}
}

func (d *floatDictionary) Lookup(indexes []int32, values []Value) {
	for i, j := range indexes {
		values[i] = d.Index(j)
	}
}

func (d *floatDictionary) Bounds(indexes []int32) (min, max Value) {
	if len(indexes) > 0 {
		minValue := d.values[indexes[0]]
		maxValue := minValue

		for _, i := range indexes[1:] {
			value := d.values[i]
			switch {
			case value < minValue:
				minValue = value
			case value > maxValue:
				maxValue = value
			}
		}

		min = makeValueFloat(minValue)
		max = makeValueFloat(maxValue)
	}
	return min, max
}

func (d *floatDictionary) Reset() {
	d.values = d.values[:0]
	d.index = nil
}

func (d *floatDictionary) Page() BufferedPage {
	return &d.floatPage
}

type doubleDictionary struct {
	doublePage
	index map[float64]int32
}

func newDoubleDictionary(typ Type, columnIndex int16, numValues int32, values []byte) *doubleDictionary {
	return &doubleDictionary{
		doublePage: doublePage{
			typ:         typ,
			values:      bits.BytesToFloat64(values)[:numValues],
			columnIndex: ^columnIndex,
		},
	}
}

func (d *doubleDictionary) Type() Type { return newIndexedType(d.typ, d) }

func (d *doubleDictionary) Len() int { return len(d.values) }

func (d *doubleDictionary) Index(i int32) Value { return makeValueDouble(d.values[i]) }

func (d *doubleDictionary) Insert(indexes []int32, values []Value) {
	_ = indexes[:len(values)]

	if d.index == nil {
		d.index = make(map[float64]int32, cap(d.values))
		for i, v := range d.values {
			d.index[v] = int32(i)
		}
	}

	for i, v := range values {
		value := v.Double()

		index, exists := d.index[value]
		if !exists {
			index = int32(len(d.values))
			d.values = append(d.values, value)
			d.index[value] = index
		}

		indexes[i] = index
	}
}

func (d *doubleDictionary) Lookup(indexes []int32, values []Value) {
	for i, j := range indexes {
		values[i] = d.Index(j)
	}
}

func (d *doubleDictionary) Bounds(indexes []int32) (min, max Value) {
	if len(indexes) > 0 {
		minValue := d.values[indexes[0]]
		maxValue := minValue

		for _, i := range indexes[1:] {
			value := d.values[i]
			switch {
			case value < minValue:
				minValue = value
			case value > maxValue:
				maxValue = value
			}
		}

		min = makeValueDouble(minValue)
		max = makeValueDouble(maxValue)
	}
	return min, max
}

func (d *doubleDictionary) Reset() {
	d.values = d.values[:0]
	d.index = nil
}

func (d *doubleDictionary) Page() BufferedPage {
	return &d.doublePage
}

type byteArrayDictionary struct {
	byteArrayPage
	offsets []uint32
	index   map[string]int32
}

func newByteArrayDictionary(typ Type, columnIndex int16, numValues int32, values []byte) *byteArrayDictionary {
	d := &byteArrayDictionary{
		offsets: make([]uint32, 0, numValues),
		byteArrayPage: byteArrayPage{
			typ:         typ,
			values:      values,
			numValues:   numValues,
			columnIndex: ^columnIndex,
		},
	}

	for i := 0; i < len(values); {
		n := plain.ByteArrayLength(values[i:])
		d.offsets = append(d.offsets, uint32(i))
		i += plain.ByteArrayLengthSize
		i += n
	}

	return d
}

func (d *byteArrayDictionary) Type() Type { return newIndexedType(d.typ, d) }

func (d *byteArrayDictionary) Len() int { return len(d.offsets) }

func (d *byteArrayDictionary) Index(i int32) Value {
	return makeValueBytes(ByteArray, d.valueAt(d.offsets[i]))
}

func (d *byteArrayDictionary) Insert(indexes []int32, values []Value) {
	_ = indexes[:len(values)]

	if d.index == nil {
		d.index = make(map[string]int32, cap(d.offsets))
		for index, offset := range d.offsets {
			d.index[bits.BytesToString(d.valueAt(offset))] = int32(index)
		}
	}

	for i, v := range values {
		value := v.ByteArray()

		index, exists := d.index[string(value)]
		if !exists {
			index = int32(len(d.offsets))
			value = d.append(value)
			stringValue := bits.BytesToString(value)
			d.index[stringValue] = index
		}

		indexes[i] = index
	}
}

func (d *byteArrayDictionary) append(value []byte) []byte {
	offset := len(d.values)
	d.values = plain.AppendByteArray(d.values, value)
	d.offsets = append(d.offsets, uint32(offset))
	d.numValues++
	return d.values[offset+plain.ByteArrayLengthSize : len(d.values) : len(d.values)]
}

func (d *byteArrayDictionary) Lookup(indexes []int32, values []Value) {
	for i, j := range indexes {
		values[i] = d.Index(j)
	}
}

func (d *byteArrayDictionary) Bounds(indexes []int32) (min, max Value) {
	if len(indexes) > 0 {
		minValue := d.valueAt(d.offsets[indexes[0]])
		maxValue := minValue

		for _, i := range indexes[1:] {
			value := d.valueAt(d.offsets[i])
			switch {
			case bytes.Compare(value, minValue) < 0:
				minValue = value
			case bytes.Compare(value, maxValue) > 0:
				maxValue = value
			}
		}

		min = makeValueBytes(ByteArray, minValue)
		max = makeValueBytes(ByteArray, maxValue)
	}
	return min, max
}

func (d *byteArrayDictionary) Reset() {
	d.offsets = d.offsets[:0]
	d.values = d.values[:0]
	d.numValues = 0
	d.index = nil
}

func (d *byteArrayDictionary) Page() BufferedPage {
	return &d.byteArrayPage
}

type fixedLenByteArrayDictionary struct {
	fixedLenByteArrayPage
	index map[string]int32
}

func newFixedLenByteArrayDictionary(typ Type, columnIndex int16, numValues int32, data []byte) *fixedLenByteArrayDictionary {
	size := typ.Length()
	return &fixedLenByteArrayDictionary{
		fixedLenByteArrayPage: fixedLenByteArrayPage{
			typ:         typ,
			size:        size,
			data:        data,
			columnIndex: ^columnIndex,
		},
	}
}

func (d *fixedLenByteArrayDictionary) Type() Type { return newIndexedType(d.typ, d) }

func (d *fixedLenByteArrayDictionary) Len() int { return len(d.data) / d.size }

func (d *fixedLenByteArrayDictionary) Index(i int32) Value {
	return makeValueBytes(FixedLenByteArray, d.value(i))
}

func (d *fixedLenByteArrayDictionary) value(i int32) []byte {
	return d.data[int(i)*d.size : int(i+1)*d.size]
}

func (d *fixedLenByteArrayDictionary) Insert(indexes []int32, values []Value) {
	_ = indexes[:len(values)]

	if d.index == nil {
		d.index = make(map[string]int32, cap(d.data)/d.size)
		for i, j := 0, int32(0); i < len(d.data); i += d.size {
			d.index[bits.BytesToString(d.data[i:i+d.size])] = j
			j++
		}
	}

	for i, v := range values {
		value := v.ByteArray()

		index, exists := d.index[string(value)]
		if !exists {
			index = int32(d.Len())
			start := len(d.data)
			d.data = append(d.data, value...)
			d.index[bits.BytesToString(d.data[start:])] = index
		}

		indexes[i] = index
	}
}

func (d *fixedLenByteArrayDictionary) Lookup(indexes []int32, values []Value) {
	for i, j := range indexes {
		values[i] = d.Index(j)
	}
}

func (d *fixedLenByteArrayDictionary) Bounds(indexes []int32) (min, max Value) {
	if len(indexes) > 0 {
		minValue := d.value(indexes[0])
		maxValue := minValue

		for _, i := range indexes[1:] {
			value := d.value(i)
			switch {
			case bytes.Compare(value, minValue) < 0:
				minValue = value
			case bytes.Compare(value, maxValue) > 0:
				maxValue = value
			}
		}

		min = makeValueBytes(FixedLenByteArray, minValue)
		max = makeValueBytes(FixedLenByteArray, maxValue)
	}
	return min, max
}

func (d *fixedLenByteArrayDictionary) Reset() {
	d.data = d.data[:0]
	d.index = nil
}

func (d *fixedLenByteArrayDictionary) Page() BufferedPage {
	return &d.fixedLenByteArrayPage
}

type uint32Dictionary struct {
	uint32Page
	index map[uint32]int32
}

func newUint32Dictionary(typ Type, columnIndex int16, numValues int32, data []byte) *uint32Dictionary {
	return &uint32Dictionary{
		uint32Page: uint32Page{
			typ:         typ,
			values:      bits.BytesToUint32(data)[:numValues],
			columnIndex: ^columnIndex,
		},
	}
}

func (d *uint32Dictionary) Type() Type { return newIndexedType(d.typ, d) }

func (d *uint32Dictionary) Len() int { return len(d.values) }

func (d *uint32Dictionary) Index(i int32) Value { return makeValueUint32(d.values[i]) }

func (d *uint32Dictionary) Insert(indexes []int32, values []Value) {
	_ = indexes[:len(values)]

	if d.index == nil {
		d.index = make(map[uint32]int32, cap(d.values))
		for i, v := range d.values {
			d.index[v] = int32(i)
		}
	}

	for i, v := range values {
		value := v.Uint32()

		index, exists := d.index[value]
		if !exists {
			index = int32(len(d.values))
			d.values = append(d.values, value)
			d.index[value] = index
		}

		indexes[i] = index
	}
}

func (d *uint32Dictionary) Lookup(indexes []int32, values []Value) {
	for i, j := range indexes {
		values[i] = d.Index(j)
	}
}

func (d *uint32Dictionary) Bounds(indexes []int32) (min, max Value) {
	if len(indexes) > 0 {
		minValue := d.values[indexes[0]]
		maxValue := minValue

		for _, i := range indexes[1:] {
			value := d.values[i]
			switch {
			case value < minValue:
				minValue = value
			case value > maxValue:
				maxValue = value
			}
		}

		min = makeValueUint32(minValue)
		max = makeValueUint32(maxValue)
	}
	return min, max
}

func (d *uint32Dictionary) Reset() {
	d.values = d.values[:0]
	d.index = nil
}

func (d *uint32Dictionary) Page() BufferedPage {
	return &d.uint32Page
}

type uint64Dictionary struct {
	uint64Page
	index map[uint64]int32
}

func newUint64Dictionary(typ Type, columnIndex int16, numValues int32, data []byte) *uint64Dictionary {
	return &uint64Dictionary{
		uint64Page: uint64Page{
			typ:         typ,
			values:      bits.BytesToUint64(data),
			columnIndex: ^columnIndex,
		},
	}
}

func (d *uint64Dictionary) Type() Type { return newIndexedType(d.typ, d) }

func (d *uint64Dictionary) Len() int { return len(d.values) }

func (d *uint64Dictionary) Index(i int32) Value { return makeValueUint64(d.values[i]) }

func (d *uint64Dictionary) Insert(indexes []int32, values []Value) {
	_ = indexes[:len(values)]

	if d.index == nil {
		d.index = make(map[uint64]int32, cap(d.values))
		for i, v := range d.values {
			d.index[v] = int32(i)
		}
	}

	for i, v := range values {
		value := v.Uint64()

		index, exists := d.index[value]
		if !exists {
			index = int32(len(d.values))
			d.values = append(d.values, value)
			d.index[value] = index
		}

		indexes[i] = index
	}
}

func (d *uint64Dictionary) Lookup(indexes []int32, values []Value) {
	for i, j := range indexes {
		values[i] = d.Index(j)
	}
}

func (d *uint64Dictionary) Bounds(indexes []int32) (min, max Value) {
	if len(indexes) > 0 {
		minValue := d.values[indexes[0]]
		maxValue := minValue

		for _, i := range indexes[1:] {
			value := d.values[i]
			switch {
			case value < minValue:
				minValue = value
			case value > maxValue:
				maxValue = value
			}
		}

		min = makeValueUint64(minValue)
		max = makeValueUint64(maxValue)
	}
	return min, max
}

func (d *uint64Dictionary) Reset() {
	d.values = d.values[:0]
	d.index = nil
}

func (d *uint64Dictionary) Page() BufferedPage {
	return &d.uint64Page
}

// indexedType is a wrapper around a Type value which overrides object
// constructors to use indexed versions referencing values in the dictionary
// instead of storing plain values.
type indexedType struct {
	Type
	dict Dictionary
}

func newIndexedType(typ Type, dict Dictionary) *indexedType {
	return &indexedType{Type: typ, dict: dict}
}

func (t *indexedType) NewColumnBuffer(columnIndex, bufferSize int) ColumnBuffer {
	return newIndexedColumnBuffer(t, makeColumnIndex(columnIndex), bufferSize)
}

func (t *indexedType) NewPage(columnIndex, numValues int, data []byte) Page {
	return newIndexedPage(t, makeColumnIndex(columnIndex), makeNumValues(numValues), data)
}

// indexedPage is an implementation of the BufferedPage interface which stores
// indexes instead of plain value. The indexes reference the values in a
// dictionary that the page was created for.
type indexedPage struct {
	typ         *indexedType
	values      []int32
	columnIndex int16
}

func newIndexedPage(typ *indexedType, columnIndex int16, numValues int32, values []byte) *indexedPage {
	// RLE encoded values that contain dictionary indexes in data pages are
	// sometimes truncated when they contain only zeros. We account for this
	// special case here and extend the values buffer if it is shorter than
	// needed to hold `numValues`.
	size := 4 * int(numValues)

	if len(values) < size {
		if cap(values) < size {
			tmp := make([]byte, size)
			copy(tmp, values)
			values = tmp
		} else {
			clear := values[len(values) : len(values)+size]
			for i := range clear {
				clear[i] = 0
			}
		}
	}

	return &indexedPage{
		typ:         typ,
		values:      bits.BytesToInt32(values[:size]),
		columnIndex: ^columnIndex,
	}
}

func (page *indexedPage) Type() Type { return indexedPageType{page.typ} }

func (page *indexedPage) Column() int { return int(^page.columnIndex) }

func (page *indexedPage) Dictionary() Dictionary { return page.typ.dict }

func (page *indexedPage) NumRows() int64 { return int64(len(page.values)) }

func (page *indexedPage) NumValues() int64 { return int64(len(page.values)) }

func (page *indexedPage) NumNulls() int64 { return 0 }

func (page *indexedPage) Size() int64 { return 4 * int64(len(page.values)) }

func (page *indexedPage) RepetitionLevels() []byte { return nil }

func (page *indexedPage) DefinitionLevels() []byte { return nil }

func (page *indexedPage) Data() []byte { return bits.Int32ToBytes(page.values) }

func (page *indexedPage) Values() ValueReader { return &indexedPageValues{page: page} }

func (page *indexedPage) Buffer() BufferedPage { return page }

func (page *indexedPage) Bounds() (min, max Value, ok bool) {
	if ok = len(page.values) > 0; ok {
		min, max = page.typ.dict.Bounds(page.values)
		min.columnIndex = page.columnIndex
		max.columnIndex = page.columnIndex
	}
	return min, max, ok
}

func (page *indexedPage) Clone() BufferedPage {
	return &indexedPage{
		typ:         page.typ,
		values:      append([]int32{}, page.values...),
		columnIndex: page.columnIndex,
	}
}

func (page *indexedPage) Slice(i, j int64) BufferedPage {
	return &indexedPage{
		typ:         page.typ,
		values:      page.values[i:j],
		columnIndex: page.columnIndex,
	}
}

// indexedPageType is an adapter for the indexedType returned when accessing
// the type of an indexedPage value. It overrides the Encode/Decode methods to
// account for the fact that an indexed page is holding indexes of values into
// its dictionary instead of plain values.
type indexedPageType struct{ *indexedType }

func (t indexedPageType) Encode(dst, src []byte, enc encoding.Encoding) ([]byte, error) {
	return enc.EncodeInt32(dst, src)
}

func (t indexedPageType) Decode(dst, src []byte, enc encoding.Encoding) ([]byte, error) {
	return enc.DecodeInt32(dst, src)
}

type indexedPageValues struct {
	page   *indexedPage
	offset int
}

func (r *indexedPageValues) ReadValues(values []Value) (n int, err error) {
	dict := r.page.typ.dict
	pageValues := r.page.values
	columnIndex := r.page.columnIndex

	for n < len(values) && r.offset < len(pageValues) {
		v := dict.Index(pageValues[r.offset])
		v.columnIndex = columnIndex
		values[n] = v
		r.offset++
		n++
	}

	if r.offset == len(pageValues) {
		err = io.EOF
	}

	return n, err
}

// indexedColumnBuffer is an implementation of the ColumnBuffer interface which
// builds a page of indexes into a parent dictionary when values are written.
type indexedColumnBuffer struct{ indexedPage }

func newIndexedColumnBuffer(typ *indexedType, columnIndex int16, bufferSize int) *indexedColumnBuffer {
	return &indexedColumnBuffer{
		indexedPage: indexedPage{
			typ:         typ,
			values:      make([]int32, 0, bufferSize/4),
			columnIndex: ^columnIndex,
		},
	}
}

func (col *indexedColumnBuffer) Clone() ColumnBuffer {
	return &indexedColumnBuffer{
		indexedPage: indexedPage{
			typ:         col.typ,
			values:      append([]int32{}, col.values...),
			columnIndex: col.columnIndex,
		},
	}
}

func (col *indexedColumnBuffer) ColumnIndex() ColumnIndex { return indexedColumnIndex{col} }

func (col *indexedColumnBuffer) OffsetIndex() OffsetIndex { return indexedOffsetIndex{col} }

func (col *indexedColumnBuffer) BloomFilter() BloomFilter { return nil }

func (col *indexedColumnBuffer) Dictionary() Dictionary { return col.typ.dict }

func (col *indexedColumnBuffer) Pages() Pages { return onePage(col.Page()) }

func (col *indexedColumnBuffer) Page() BufferedPage { return &col.indexedPage }

func (col *indexedColumnBuffer) Reset() { col.values = col.values[:0] }

func (col *indexedColumnBuffer) Cap() int { return cap(col.values) }

func (col *indexedColumnBuffer) Len() int { return len(col.values) }

func (col *indexedColumnBuffer) Less(i, j int) bool {
	u := col.typ.dict.Index(col.values[i])
	v := col.typ.dict.Index(col.values[j])
	return col.typ.Compare(u, v) < 0
}

func (col *indexedColumnBuffer) Swap(i, j int) {
	col.values[i], col.values[j] = col.values[j], col.values[i]
}

func (col *indexedColumnBuffer) WriteValues(values []Value) (int, error) {
	i := len(col.values)
	j := len(col.values) + len(values)

	if j <= cap(col.values) {
		col.values = col.values[:j]
	} else {
		colValues := make([]int32, j, 2*j)
		copy(colValues, col.values)
		col.values = colValues
	}

	col.typ.dict.Insert(col.values[i:], values)
	return len(values), nil
}

func (col *indexedColumnBuffer) ReadValuesAt(values []Value, offset int64) (n int, err error) {
	i := int(offset)
	switch {
	case i < 0:
		return 0, errRowIndexOutOfBounds(offset, int64(len(col.values)))
	case i >= len(col.values):
		return 0, io.EOF
	default:
		for n < len(values) && i < len(col.values) {
			values[n] = col.typ.dict.Index(col.values[i])
			values[n].columnIndex = col.columnIndex
			n++
			i++
		}
		if n < len(values) {
			err = io.EOF
		}
		return n, err
	}
}

func (col *indexedColumnBuffer) ReadRowAt(row Row, index int64) (Row, error) {
	switch {
	case index < 0:
		return row, errRowIndexOutOfBounds(index, int64(len(col.values)))
	case index >= int64(len(col.values)):
		return row, io.EOF
	default:
		v := col.typ.dict.Index(col.values[index])
		v.columnIndex = col.columnIndex
		return append(row, v), nil
	}
}

type indexedColumnIndex struct{ col *indexedColumnBuffer }

func (index indexedColumnIndex) NumPages() int       { return 1 }
func (index indexedColumnIndex) NullCount(int) int64 { return 0 }
func (index indexedColumnIndex) NullPage(int) bool   { return false }
func (index indexedColumnIndex) MinValue(int) Value {
	min, _, _ := index.col.Bounds()
	return min
}
func (index indexedColumnIndex) MaxValue(int) Value {
	_, max, _ := index.col.Bounds()
	return max
}
func (index indexedColumnIndex) IsAscending() bool {
	min, max, _ := index.col.Bounds()
	return index.col.typ.Compare(min, max) <= 0
}
func (index indexedColumnIndex) IsDescending() bool {
	min, max, _ := index.col.Bounds()
	return index.col.typ.Compare(min, max) > 0
}

type indexedOffsetIndex struct{ col *indexedColumnBuffer }

func (index indexedOffsetIndex) NumPages() int                { return 1 }
func (index indexedOffsetIndex) Offset(int) int64             { return 0 }
func (index indexedOffsetIndex) CompressedPageSize(int) int64 { return index.col.Size() }
func (index indexedOffsetIndex) FirstRowIndex(int) int64      { return 0 }
