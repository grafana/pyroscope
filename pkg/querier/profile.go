package querier

import (
	"container/heap"

	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	"github.com/grafana/fire/pkg/model"
	"github.com/grafana/fire/pkg/util"
)

type profilesResponsesHeap []responseFromIngesters[*ingestv1.SelectProfilesResponse]

// newProfilesResponseHeap returns a heap that sort responses by their timestamps and labels.
func newProfilesResponseHeap(profiles profilesResponsesHeap) heap.Interface {
	res := make(profilesResponsesHeap, 0, len(profiles))
	for _, p := range profiles {
		if len(p.response.Profiles) > 0 {
			res = append(res, p)
		}
	}
	return &res
}

// Implement sort.Interface
func (h profilesResponsesHeap) Len() int      { return len(h) }
func (h profilesResponsesHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h profilesResponsesHeap) Less(i, j int) bool {
	return model.CompareProfile(h[i].response.Profiles[0], h[j].response.Profiles[0]) < 0
}

// Implement heap.Interface
func (h *profilesResponsesHeap) Push(x interface{}) {
	*h = append(*h, x.(responseFromIngesters[*ingestv1.SelectProfilesResponse]))
}

func (h *profilesResponsesHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// dedupeProfiles dedupes profiles responses by timestamp and labels.
// It expects profiles from each response to be sorted by timestamp and labels already.
// Returns the profile deduped per ingester address.
// The function tries to spread profile per ingester evenly.
// todo: This function can be optimized by peeking instead of popping the heap.
func dedupeProfiles(responses []responseFromIngesters[*ingestv1.SelectProfilesResponse]) map[string][]*ingestv1.Profile {
	type tuple struct {
		ingesterAddr string
		profile      *ingestv1.Profile
		responseFromIngesters[*ingestv1.SelectProfilesResponse]
	}
	var (
		responsesHeap       = newProfilesResponseHeap(responses)
		deduped             []*ingestv1.Profile
		profilesPerIngester = make(map[string][]*ingestv1.Profile, len(responses))
		tuples              = make([]tuple, 0, len(responses))
	)

	heap.Init(responsesHeap)

	for responsesHeap.Len() > 0 || len(tuples) > 0 {
		if responsesHeap.Len() > 0 {
			current := heap.Pop(responsesHeap).(responseFromIngesters[*ingestv1.SelectProfilesResponse])
			if len(current.response.Profiles) == 0 {
				continue
			}
			// add the top profile to the tuple list if the current profile is equal the previous one.
			if len(tuples) == 0 || model.CompareProfile(current.response.Profiles[0], tuples[len(tuples)-1].profile) == 0 {
				tuples = append(tuples, tuple{
					ingesterAddr:          current.addr,
					profile:               current.response.Profiles[0],
					responseFromIngesters: current,
				})
				current.response.Profiles = current.response.Profiles[1:]
				continue
			}
			// the current profile is different.
			heap.Push(responsesHeap, current)
		}
		// if the heap is empty and we don't have tuples we're done.
		if len(tuples) == 0 {
			continue
		}
		// no duplicate found just a single profile.
		if len(tuples) == 1 {
			profilesPerIngester[tuples[0].addr] = append(profilesPerIngester[tuples[0].addr], tuples[0].profile)
			deduped = append(deduped, tuples[0].profile)
			if len(tuples[0].response.Profiles) > 0 {
				heap.Push(responsesHeap, tuples[0].responseFromIngesters)
			}
			tuples = tuples[:0]
			continue
		}
		// we have a duplicate let's select a winner based on the ingester with the less profiles
		// this way we evenly distribute the profiles symbols API calls across the ingesters
		min := tuples[0]
		for _, t := range tuples {
			if len(profilesPerIngester[t.addr]) < len(profilesPerIngester[min.addr]) {
				min = t
			}
		}
		profilesPerIngester[min.addr] = append(profilesPerIngester[min.addr], min.profile)
		deduped = append(deduped, min.profile)
		if len(min.response.Profiles) > 0 {
			heap.Push(responsesHeap, min.responseFromIngesters)
		}
		for _, t := range tuples {
			if t.addr != min.addr && len(t.response.Profiles) > 0 {
				heap.Push(responsesHeap, t.responseFromIngesters)
				continue
			}
		}
		tuples = tuples[:0]

	}
	return profilesPerIngester
}

type stack struct {
	locations []string
	value     int64
}

// Merge stacktraces and prepare for fetching symbols.
// Returns ingesters addresses, a map of stracktraces per ingester address and per ID.
func mergeStacktraces(profilesPerIngester map[string][]*ingestv1.Profile) (ingester []string, stacktracesPerIngester map[string][][]byte, stracktracesByID map[string]*stack) {
	stacktracesPerIngester = make(map[string][][]byte, len(profilesPerIngester))
	stracktracesByID = make(map[string]*stack)
	ingester = make([]string, 0, len(profilesPerIngester))

	for ing, profiles := range profilesPerIngester {
		ingester = append(ingester, ing)
		for _, profile := range profiles {
			for _, stacktrace := range profile.Stacktraces {
				id := util.UnsafeGetString(stacktrace.ID)
				var stacktraceSample *stack
				var ok bool
				stacktraceSample, ok = stracktracesByID[id]
				if !ok {
					stacktraceSample = &stack{}
					stacktracesPerIngester[ing] = append(stacktracesPerIngester[ing], stacktrace.ID)
					stracktracesByID[id] = stacktraceSample
				}
				stacktraceSample.value += stacktrace.Value

			}
		}
	}
	return ingester, stacktracesPerIngester, stracktracesByID
}
