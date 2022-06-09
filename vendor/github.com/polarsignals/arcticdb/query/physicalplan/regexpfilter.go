package physicalplan

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/apache/arrow/go/v8/arrow"
	"github.com/apache/arrow/go/v8/arrow/array"
)

type RegExpFilter struct {
	left     *ArrayRef
	notMatch bool
	right    *regexp.Regexp
}

func (f *RegExpFilter) Eval(r arrow.Record) (*Bitmap, error) {
	leftData, exists, err := f.left.ArrowArray(r)
	if err != nil {
		return nil, err
	}

	// TODO: This needs a bunch of test cases to validate edge cases like non
	// existant columns or null values.
	if !exists {
		res := NewBitmap()
		if f.notMatch {
			for i := uint32(0); i < uint32(r.NumRows()); i++ {
				res.Add(i)
			}
			return res, nil
		}
		return res, nil
	}

	if f.notMatch {
		return ArrayScalarRegexNotMatch(leftData, f.right)
	}

	return ArrayScalarRegexMatch(leftData, f.right)
}

func (f *RegExpFilter) String() string {
	if f.notMatch {
		return fmt.Sprintf("%s !~ \"%s\"", f.left.String(), f.right.String())
	}
	return fmt.Sprintf("%s =~ \"%s\"", f.left.String(), f.right.String())
}

func ArrayScalarRegexMatch(left arrow.Array, right *regexp.Regexp) (*Bitmap, error) {
	switch left.(type) {
	case *array.Binary:
		return BinaryArrayScalarRegexMatch(left.(*array.Binary), right)
	case *array.String:
		return StringArrayScalarRegexMatch(left.(*array.String), right)
	}

	return nil, errors.New("unsupported type")
}

func ArrayScalarRegexNotMatch(left arrow.Array, right *regexp.Regexp) (*Bitmap, error) {
	switch left.(type) {
	case *array.Binary:
		return BinaryArrayScalarRegexNotMatch(left.(*array.Binary), right)
	case *array.String:
		return StringArrayScalarRegexNotMatch(left.(*array.String), right)
	}

	return nil, errors.New("unsupported type")
}

func BinaryArrayScalarRegexMatch(left *array.Binary, right *regexp.Regexp) (*Bitmap, error) {
	res := NewBitmap()
	for i := 0; i < left.Len(); i++ {
		if left.IsNull(i) {
			continue
		}
		if right.MatchString(string(left.Value(i))) {
			res.Add(uint32(i))
		}
	}

	return res, nil
}

func BinaryArrayScalarRegexNotMatch(left *array.Binary, right *regexp.Regexp) (*Bitmap, error) {
	res := NewBitmap()
	for i := 0; i < left.Len(); i++ {
		if left.IsNull(i) {
			continue
		}
		if !right.MatchString(string(left.Value(i))) {
			res.Add(uint32(i))
		}
	}

	return res, nil
}

func StringArrayScalarRegexMatch(left *array.String, right *regexp.Regexp) (*Bitmap, error) {
	res := NewBitmap()
	for i := 0; i < left.Len(); i++ {
		if left.IsNull(i) {
			continue
		}
		if right.MatchString(left.Value(i)) {
			res.Add(uint32(i))
		}
	}

	return res, nil
}

func StringArrayScalarRegexNotMatch(left *array.String, right *regexp.Regexp) (*Bitmap, error) {
	res := NewBitmap()
	for i := 0; i < left.Len(); i++ {
		if left.IsNull(i) {
			continue
		}
		if !right.MatchString(left.Value(i)) {
			res.Add(uint32(i))
		}
	}

	return res, nil
}
