package streaming

import (
	"fmt"
	"github.com/pyroscope-io/pyroscope/pkg/util/arenahelper"
	"io"
)

// location references into lines slice as packed uint64(from << 32 | to)
type locationFunctions struct {
	lines []line
}

func (l *locationFunctions) reset(a *arenahelper.ArenaWrapper, nLocations int) {
	if l.lines == nil {
		l.lines = arenahelper.MakeSlice[line](a, 0, nLocations*2)
	}
	l.lines = l.lines[:0]
}

// revive:disable-next-line:cognitive-complexity,cyclomatic necessary complexity
func (m *location) UnmarshalVT(dAtA []byte, functions *locationFunctions, a *arenahelper.ArenaWrapper) error {
	var tmpLine line
	m.id = 0
	m.linesRef = 0
	refFrom := len(functions.lines)
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
			return fmt.Errorf("proto: Location: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Location: illegal tag %d (wire type %d)", fieldNum, wire)
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
				return fmt.Errorf("proto: wrong wireType = %d for field MappingId", wireType)
			}
			//m.MappingId = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflow
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				//m.MappingId |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 3:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Address", wireType)
			}
			//m.Address = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflow
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				//m.Address |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 4:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Line", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflow
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLength
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLength
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			//m.Line = append(m.Line, &Line{})
			if err := tmpLine.UnmarshalVT(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			if len(functions.lines) < cap(functions.lines) {
				functions.lines = append(functions.lines, tmpLine)
			} else {
				functions.lines = arenahelper.AppendA(functions.lines, tmpLine, a)
			}
			iNdEx = postIndex
		case 5:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field IsFolded", wireType)
			}
			var v int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflow
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			//m.IsFolded = bool(v != 0)
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
	refTo := len(functions.lines)
	ref := uint64(refFrom<<32 | refTo)
	m.linesRef = ref
	return nil
}
