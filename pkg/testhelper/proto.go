package testhelper

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
)

// CloneProto clones a protobuf message.
func CloneProto[T proto.Message](t *testing.T, in T) T {
	t.Helper()
	return proto.Clone(in).(T)
}

// Equal compares two protobuf messages ignoring extra generated proto fields.
func EqualProto(t *testing.T, expected, actual interface{}) {
	t.Helper()
	if diff := cmp.Diff(expected, actual, ignoreProtoFields()); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func ignoreProtoFields() cmp.Option {
	return cmp.FilterPath(func(p cmp.Path) bool {
		switch p[len(p)-1].String() {
		case ".state", ".sizeCache", ".unknownFields":
			return true
		}
		return false
	}, cmp.Ignore())
}
