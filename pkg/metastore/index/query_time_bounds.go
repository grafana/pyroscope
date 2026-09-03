package index

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
)

const (
	blockMetaMinTimeField = 6
	blockMetaMaxTimeField = 7
)

// blockMetaTimeBounds reads min_time and max_time from an encoded BlockMeta
// without materializing the message, so that queries can discard blocks
// outside their time range before paying for a full decode.
func blockMetaTimeBounds(b []byte) (minTime, maxTime int64, err error) {
	for len(b) > 0 {
		field, wireType, n := protowire.ConsumeTag(b)
		if n < 0 {
			return 0, 0, protowire.ParseError(n)
		}
		b = b[n:]
		if wireType == protowire.EndGroupType {
			return 0, 0, fmt.Errorf("unexpected end group for BlockMeta field %d", field)
		}

		switch field {
		case blockMetaMinTimeField, blockMetaMaxTimeField:
			if wireType != protowire.VarintType {
				return 0, 0, fmt.Errorf("invalid wire type %d for BlockMeta field %d", wireType, field)
			}
			value, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return 0, 0, protowire.ParseError(n)
			}
			if field == blockMetaMinTimeField {
				minTime = int64(value)
			} else {
				maxTime = int64(value)
			}
			b = b[n:]
		default:
			// BlockMeta values are written by MarshalVT in ascending field order,
			// so min_time and max_time cannot appear after a larger field number.
			if field > blockMetaMaxTimeField {
				return minTime, maxTime, nil
			}
			n = protowire.ConsumeFieldValue(field, wireType, b)
			if n < 0 {
				return 0, 0, protowire.ParseError(n)
			}
			b = b[n:]
		}
	}
	return minTime, maxTime, nil
}
