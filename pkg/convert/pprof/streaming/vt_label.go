package streaming

import (
	"fmt"
	"io"
)

// revive:disable-next-line:cognitive-complexity,cyclomatic necessary complexity
func UnmarshalVTLabel(dAtA []byte) (uint64, error) {
	k := int64(0)
	v := int64(0)

	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		//preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflow
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return 0, fmt.Errorf("proto: Label: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return 0, fmt.Errorf("proto: Label: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return 0, fmt.Errorf("proto: wrong wireType = %d for field Key", wireType)
			}
			k = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflow
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				k |= int64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 0 {
				return 0, fmt.Errorf("proto: wrong wireType = %d for field Str", wireType)
			}
			v = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflow
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= int64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 3:
			if wireType != 0 {
				return 0, fmt.Errorf("proto: wrong wireType = %d for field Num", wireType)
			}
			//m.Num = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflow
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				//m.Num |= int64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 4:
			if wireType != 0 {
				return 0, fmt.Errorf("proto: wrong wireType = %d for field NumUnit", wireType)
			}
			//m.NumUnit = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflow
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				//m.NumUnit |= int64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		default:
			return 0, ErrUnknownField
		}
	}

	if iNdEx > l {
		return 0, io.ErrUnexpectedEOF
	}
	return uint64(k<<32 | v), nil
}
