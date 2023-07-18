package merge

import (
	"sync"
)

type Merger interface {
	Merge(src Merger)
}

func MergeTriesConcurrently(concurrency int, tries ...Merger) Merger {
	// mutex := sync.Mutex{}
	if len(tries) == 0 {
		return nil
	}
	pool := tries
	jobs := make(chan []Merger)
	done := make(chan Merger, len(tries)-1)
	wg := sync.WaitGroup{}
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			for j := range jobs {
				j[0].Merge(j[1])
				done <- j[0]
			}
			wg.Done()
		}()
	}

	merges := 0
	inProgress := 0
	for merges < len(tries)-1 {
		// first, queue all possible jobs
		for len(pool) >= 2 && inProgress < concurrency {
			inProgress++
			j := pool[:2]
			jobs <- j
			pool = pool[2:]
		}
		// then block until there's a job done notification
		t := <-done
		pool = append([]Merger{t}, pool...)
		merges++
		inProgress--
	}
	close(jobs)
	wg.Wait()
	return pool[0]
}

func MergeTriesSerially(_ int, tries ...Merger) Merger {
	// rand.Shuffle(len(tries), func(i, j int) {
	// 	tries[i], tries[j] = tries[j], tries[i]
	// })
	// mutex := sync.Mutex{}
	if len(tries) == 0 {
		return nil
	}
	resultTrie := tries[0]
	for i := 1; i < len(tries); i++ {
		resultTrie.Merge(tries[i])
	}
	return resultTrie
}
