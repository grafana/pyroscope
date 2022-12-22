package streaming

import (
	"fmt"

	"io"
)

func (p *MoleculeParser) parseSampleVT(buffer []byte) error {
	p.tmpSample.resetSample()
	err := p.tmpSample.UnmarshalSampleVT(buffer, &p.tmpLabel)
	if err != nil {
		return err
	}

	for i := range p.tmpSample.tmpStackLoc {
		err = p.addStackLocation(p.tmpSample.tmpStackLoc[i])
		if err != nil {
			return err
		}
	}
	reverseStack(p.tmpSample.tmpStack)

	p.createTrees(p.newCache)

	return nil
}
func (s *sample) UnmarshalSampleVT(dAtA []byte, tmpLabel *label) error {
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
				s.tmpStackLoc = append(s.tmpStackLoc, v)
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
					s.tmpStackLoc = append(s.tmpStackLoc, v)
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
				s.tmpValues = append(s.tmpValues, v)
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
					s.tmpValues = append(s.tmpValues, v)
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

			if err := tmpLabel.UnmarshalVT(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			if tmpLabel.v != 0 {
				s.tmpLabels = append(s.tmpLabels, *tmpLabel)
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
