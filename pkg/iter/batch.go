package iter

import "context"

// ReadBatch reads profiles from the iterator in batches and call fn.
// If fn returns an error, the iteration is stopped and the error is returned.
// The array passed in fn is reused between calls, so it should be copied if needed.
func ReadBatch[T any](ctx context.Context, iterator Iterator[T], batchSize int, fn func(context.Context, []T) error) error {
	defer iterator.Close()
	batch := make([]T, 0, batchSize)
	for {
		// build a batch of profiles
		batch = batch[:0]
		for iterator.Next() {
			profile := iterator.At()
			batch = append(batch, profile)
			if len(batch) >= batchSize {
				break
			}
		}
		if iterator.Err() != nil {
			return iterator.Err()
		}
		if len(batch) == 0 {
			return nil
		}
		if err := fn(ctx, batch); err != nil {
			return err
		}
	}
}
