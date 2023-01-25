package streaming

func (p *VTStreamingParser) parseSampleWB(buffer []byte) error {
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

	for _, vi := range p.indexes {
		v := uint64(p.tmpSample.tmpValues[vi])
		if v == 0 {
			continue
		}
		s := p.tmpSample.tmpStack
		_ = s
		wb := p.wbCache.getWriteBatch(p, vi)
		if wb == nil {
			continue
		}
		sb := wb.wb.StackBuilder()
		sb.Reset()
		for _, frame := range p.tmpSample.tmpStack {
			sb.Push(frame)
		}
		stackID := sb.Build()

		if j := findLabelIndex(p.tmpSample.tmpLabels, p.profileIDLabelIndex); j >= 0 {
			baseLineLabels := CutLabel(p.arena, p.tmpSample.tmpLabels, j)
			wb.getAppender(p, baseLineLabels).Append(stackID, v)
			if p.skipExemplars {
				continue
			}
		}
		wb.getAppender(p, p.tmpSample.tmpLabels).Append(stackID, v)
	}
	return nil
}
