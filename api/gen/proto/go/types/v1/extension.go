package typesv1

// IsLegacy is true if this is a legacy label names request.
func (r *LabelNamesRequest) IsLegacy() bool {
	return r.Start == 0 || r.End == 0
}
