package typesv1

// IsLegacy is true if this is a legacy label names request.
//
// Some clients may not be sending start/end timestamps. If start or end are 0,
// then mark this a legacy request. Legacy requests should only query ingesters.
func (r *LabelNamesRequest) IsLegacy() bool {
	return r.Start == 0 || r.End == 0
}
