package typesv1

// HasTimeRange is true if the time range has been set for this request.
func (r *LabelNamesRequest) HasTimeRange() bool {
	return r.Start == 0 || r.End == 0
}
