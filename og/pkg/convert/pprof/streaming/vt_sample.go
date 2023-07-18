package streaming

import (
	"fmt"
	"github.com/pyroscope-io/pyroscope/pkg/util/arenahelper"

	"io"
)

func (p *VTStreamingParser) parseSampleVT(buffer []byte) error {
	p.tmpSample.reset(p.arena)
	err := p.tmpSample.UnmarshalSampleVT(buffer, p.arena)
	if err != nil {
		return err
	}

	for i := len(p.tmpSample.tmpStackLoc) - 1; i >= 0; i-- {
		err = p.addStackLocation(p.tmpSample.tmpStackLoc[i])
		if err != nil {
			return err
		}
	}

	p.createTrees()

	return nil
}



// revive:disable-next-line:cognitive-complexity,cyclomatic necessary complexity
func (s *sample) UnmarshalSampleVT(dAtA []byte, a arenahelper.ArenaWrapper) error {
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
			return fmt.Errorf("proto: Sample: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Sample: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType == 0 {
				var v uint64
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return ErrIntOverflow
					}
					if iNdEx >= l {
						return io.ErrUnexpectedEOF
					}
					b := dAtA[iNdEx]
					iNdEx++
					v |= uint64(b&0x7F) << shift
					if b < 0x80 {
						break
					}
				}
				//err := p.addStackLocation(v)
				//if err != nil {
				//	return err
				//}
				//m.LocationId = append(m.LocationId, v)
				if len(s.tmpStackLoc) < cap(s.tmpStackLoc) {
					s.tmpStackLoc = append(s.tmpStackLoc, v)
				} else {
					s.tmpStackLoc = arenahelper.AppendA(s.tmpStackLoc, v, a)
				}
			} else if wireType == 2 {
				var packedLen int
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return ErrIntOverflow
					}
					if iNdEx >= l {
						return io.ErrUnexpectedEOF
					}
					b := dAtA[iNdEx]
					iNdEx++
					packedLen |= int(b&0x7F) << shift
					if b < 0x80 {
						break
					}
				}
				if packedLen < 0 {
					return ErrInvalidLength
				}
				postIndex := iNdEx + packedLen
				if postIndex < 0 {
					return ErrInvalidLength
				}
				if postIndex > l {
					return io.ErrUnexpectedEOF
				}
				//var elementCount int
				//var count int
				//for _, integer := range dAtA[iNdEx:postIndex] {
				//	if integer < 128 {
				//		count++
				//	}
				//}
				//elementCount = count
				//if elementCount != 0 && len(m.LocationId) == 0 {
				//	m.LocationId = make([]uint64, 0, elementCount)
				//}
				for iNdEx < postIndex {
					var v uint64
					for shift := uint(0); ; shift += 7 {
						if shift >= 64 {
							return ErrIntOverflow
						}
						if iNdEx >= l {
							return io.ErrUnexpectedEOF
						}
						b := dAtA[iNdEx]
						iNdEx++
						v |= uint64(b&0x7F) << shift
						if b < 0x80 {
							break
						}
					}
					//m.LocationId = append(m.LocationId, v)
					//err := p.addStackLocation(v)
					//if err != nil {
					//	return err
					//}
					if len(s.tmpStackLoc) < cap(s.tmpStackLoc) {
						s.tmpStackLoc = append(s.tmpStackLoc, v)
					} else {
						s.tmpStackLoc = arenahelper.AppendA(s.tmpStackLoc, v, a)
					}
				}
			} else {
				return fmt.Errorf("proto: wrong wireType = %d for field LocationId", wireType)
			}
		case 2:
			if wireType == 0 {
				var v int64
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return ErrIntOverflow
					}
					if iNdEx >= l {
						return io.ErrUnexpectedEOF
					}
					b := dAtA[iNdEx]
					iNdEx++
					v |= int64(b&0x7F) << shift
					if b < 0x80 {
						break
					}
				}
				//m.Value = append(m.Value, v)
				if len(s.tmpValues) < cap(s.tmpValues) {
					s.tmpValues = append(s.tmpValues, v)
				} else {
					s.tmpValues = arenahelper.AppendA(s.tmpValues, v, a)
				}
			} else if wireType == 2 {
				var packedLen int
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return ErrIntOverflow
					}
					if iNdEx >= l {
						return io.ErrUnexpectedEOF
					}
					b := dAtA[iNdEx]
					iNdEx++
					packedLen |= int(b&0x7F) << shift
					if b < 0x80 {
						break
					}
				}
				if packedLen < 0 {
					return ErrInvalidLength
				}
				postIndex := iNdEx + packedLen
				if postIndex < 0 {
					return ErrInvalidLength
				}
				if postIndex > l {
					return io.ErrUnexpectedEOF
				}
				//var elementCount int
				//var count int
				//for _, integer := range dAtA[iNdEx:postIndex] {
				//	if integer < 128 {
				//		count++
				//	}
				//}
				//elementCount = count
				//if elementCount != 0 && len(m.Value) == 0 {
				//	m.Value = make([]int64, 0, elementCount)
				//}
				for iNdEx < postIndex {
					var v int64
					for shift := uint(0); ; shift += 7 {
						if shift >= 64 {
							return ErrIntOverflow
						}
						if iNdEx >= l {
							return io.ErrUnexpectedEOF
						}
						b := dAtA[iNdEx]
						iNdEx++
						v |= int64(b&0x7F) << shift
						if b < 0x80 {
							break
						}
					}
					//m.Value = append(m.Value, v)
					if len(s.tmpValues) < cap(s.tmpValues) {
						s.tmpValues = append(s.tmpValues, v)
					} else {
						s.tmpValues = arenahelper.AppendA(s.tmpValues, v, a)
					}
				}
			} else {
				return fmt.Errorf("proto: wrong wireType = %d for field Value", wireType)
			}
		case 3:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Label", wireType)
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
			//m.Label = append(m.Label, &Label{})
			//if err := m.Label[len(m.Label)-1].UnmarshalVT(dAtA[iNdEx:postIndex]); err != nil {
			//	return err
			//}
			tmpLabel, err := UnmarshalVTLabel(dAtA[iNdEx:postIndex])
			if err != nil {
				return err
			}
			v := tmpLabel & 0xffffffff
			if v != 0 {
				if len(s.tmpLabels) < cap(s.tmpLabels) {
					s.tmpLabels = append(s.tmpLabels, tmpLabel)
				} else {
					s.tmpLabels = arenahelper.AppendA(s.tmpLabels, tmpLabel, a)
				}
			}
			iNdEx = postIndex
		default:
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
			return ErrUnknownField
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}

var (
	ErrInvalidLength        = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflow          = fmt.Errorf("proto: integer overflow")
	ErrUnexpectedEndOfGroup = fmt.Errorf("proto: unexpected end of group")
	ErrUnknownField         = fmt.Errorf("proto: unknown field")
)
