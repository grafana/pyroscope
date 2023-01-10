package streaming

import (
	"fmt"
	"io"
	"math"
)

// revive:disable-next-line:cognitive-complexity,cyclomatic necessary complexity
func (m *function) UnmarshalVT(dAtA []byte) error {
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
			return fmt.Errorf("proto: Function: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Function: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Id", wireType)
			}
			m.id = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflow
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.id |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Name", wireType)
			}
			name := int64(0)
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflow
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				name |= int64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if name < 0 || name > math.MaxInt32 {
				return fmt.Errorf("wromg mamae %d", name)
			}
			m.name = int32(name)
		case 3:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field SystemName", wireType)
			}
			//m.SystemName = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflow
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				//m.SystemName |= int64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 4:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Filename", wireType)
			}
			filename := int64(0)
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflow
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				filename |= int64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if filename < 0 || filename > math.MaxInt32 {
				return fmt.Errorf("wromg filename %d", filename)
			}
			m.filename = int32(filename)
		case 5:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field StartLine", wireType)
			}
			//m.StartLine = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflow
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				//m.StartLine |= int64(b&0x7F) << shift
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
