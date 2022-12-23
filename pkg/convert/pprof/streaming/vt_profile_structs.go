package streaming

import (
	"fmt"
	"io"
)

// revive:disable-next-line:cognitive-complexity,cyclomatic necessary complexity
func (p *VTStreamingParser) UnmarshalVTStructs(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	p.period = 0
	p.nStrings = 0
	p.nFunctions = 0
	p.nLocations = 0
	p.nSampleTypes = 0
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
			return fmt.Errorf("proto: Profile: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Profile: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field SampleType", wireType)
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
			//if len(p.sampleTypesParsed) == cap(p.sampleTypesParsed) {
			//	p.sampleTypesParsed = append(p.sampleTypesParsed, valueType{})
			//} else {
			//	p.sampleTypesParsed = p.sampleTypesParsed[:len(p.sampleTypesParsed)+1]
			//	//if p.sampleTypesParsed[len(p.sampleTypesParsed)-1] == nil {
			//	//	p.sampleTypesParsed[len(p.sampleTypesParsed)-1] = &sampleTypesParsed{}
			//	//}
			//}
			p.nSampleTypes++
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Sample", wireType)
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
			//if len(p.Sample) == cap(p.Sample) {
			//	p.Sample = append(p.Sample, &Sample{})
			//} else {
			//	p.Sample = p.Sample[:len(p.Sample)+1]
			//	if p.Sample[len(p.Sample)-1] == nil {
			//		p.Sample[len(p.Sample)-1] = &Sample{}
			//	}
			//}
			//if err := p.Sample[len(p.Sample)-1].UnmarshalVT(dAtA[iNdEx:postIndex]); err != nil {
			//	return err
			//}
			iNdEx = postIndex
		case 3:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Mapping", wireType)
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
			//if len(p.Mapping) == cap(p.Mapping) {
			//	p.Mapping = append(p.Mapping, &Mapping{})
			//} else {
			//	p.Mapping = p.Mapping[:len(p.Mapping)+1]
			//	if p.Mapping[len(p.Mapping)-1] == nil {
			//		p.Mapping[len(p.Mapping)-1] = &Mapping{}
			//	}
			//}
			//if err := p.Mapping[len(p.Mapping)-1].UnmarshalVT(dAtA[iNdEx:postIndex]); err != nil {
			//	return err
			//}
			iNdEx = postIndex
		case 4:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Location", wireType)
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
			//if len(p.Location) == cap(p.Location) {
			//	p.Location = append(p.Location, &Location{})
			//} else {
			//	p.Location = p.Location[:len(p.Location)+1]
			//	if p.Location[len(p.Location)-1] == nil {
			//		p.Location[len(p.Location)-1] = &Location{}
			//	}
			//}
			//if err := p.Location[len(p.Location)-1].UnmarshalVT(dAtA[iNdEx:postIndex]); err != nil {
			//	return err
			//}
			p.nLocations++
			iNdEx = postIndex
		case 5:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Function", wireType)
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
			//if len(p.Function) == cap(p.Function) {
			//	p.Function = append(p.Function, &Function{})
			//} else {
			//	p.Function = p.Function[:len(p.Function)+1]
			//	if p.Function[len(p.Function)-1] == nil {
			//		p.Function[len(p.Function)-1] = &Function{}
			//	}
			//}
			//if err := p.Function[len(p.Function)-1].UnmarshalVT(dAtA[iNdEx:postIndex]); err != nil {
			//	return err
			//}
			p.nFunctions++
			iNdEx = postIndex
		case 6:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field StringTable", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflow
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLength
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLength
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			//s := dAtA[iNdEx:postIndex]
			//if bytes.Equal(s, profileIDLabel) {
			//	p.profileIDLabelIndex = int64(len(p.strings))
			//}
			//p.strings = append(p.strings, s)
			p.nStrings++
			iNdEx = postIndex
		case 7:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field DropFrames", wireType)
			}
			//p.DropFrames = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflow
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				//p.DropFrames |= int64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 8:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field KeepFrames", wireType)
			}
			//p.KeepFrames = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflow
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				//p.KeepFrames |= int64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 9:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field TimeNanos", wireType)
			}
			//p.TimeNanos = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflow
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				//p.TimeNanos |= int64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 10:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field DurationNanos", wireType)
			}
			//p.DurationNanos = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflow
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				//p.DurationNanos |= int64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 11:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field periodType", wireType)
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
			//if p.PeriodType == nil {
			//	p.PeriodType = &ValueType{}
			//}
			if err := p.periodType.UnmarshalVT(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 12:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field period", wireType)
			}
			p.period = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflow
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				p.period |= int64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 13:
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
				//if elementCount != 0 && len(p.Comment) == 0 && cap(p.Comment) < elementCount {
				//	p.Comment = make([]int64, 0, elementCount)
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
				}
			} else {
				return fmt.Errorf("proto: wrong wireType = %d for field Comment", wireType)
			}
		case 14:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field DefaultSampleType", wireType)
			}
			//p.DefaultSampleType = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflow
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				//p.DefaultSampleType |= int64(b&0x7F) << shift
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
			//p.unknownFields = append(p.unknownFields, dAtA[iNdEx:iNdEx+skippy]...)
			//iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
