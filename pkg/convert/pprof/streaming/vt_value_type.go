package streaming

import (
	"fmt"
	"io"
)

func (m *valueType) UnmarshalVT(dAtA []byte) error {
	m.unit = 0
	m.Type = 0
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		//preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflow
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
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
			return fmt.Errorf("proto: ValueType: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: ValueType: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Type", wireType)
			}
			m.Type = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflow
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Type |= int64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Unit", wireType)
			}
			m.unit = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflow
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.unit |= int64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		default:
			return ErrUnknownField
			//iNdEx = preIndex
			//skippy, err := skip(dAtA[iNdEx:])
			//if err != nil {
			//	return err
			//}
			//if (skippy < 0) || (iNdEx+skippy) < 0 {
			//	return ErrInvalidLength
			//}
			//if (iNdEx + skippy) > l {
			//	return io.ErrUnexpectedEOF
			//}
			//m.unknownFields = append(m.unknownFields, dAtA[iNdEx:iNdEx+skippy]...)
			//iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
