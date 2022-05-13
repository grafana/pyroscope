package storage

import "context"

type IngestionProxy struct {
	putter Putter
}

func NewIngestionProxy(putter Putter) *IngestionProxy {
	return &IngestionProxy{putter: putter}
}

func (p *IngestionProxy) Put(ctx context.Context, in *PutInput) error {
	// Send data to anywhere else asynchronously. I'd use some buffer or queue,
	// ideally writes should happen in batches but our API does not support it,
	// so, we maybe implement this later.

	// Notes:
	//  - This is a good place to filter out exemplars (in.Key contains 'profile_id' label).
	//  - Sending format: I'd maybe try using 'tree':
	//    https://github.com/pyroscope-io/pyroscope/blob/f0589d9dccddbe4f34d83047d3b12c59277f7acc/pkg/parser/parser.go#L101.
	//    'in.Val' is a *tree.Tree with already resolved node names, therefore there is no need to use dictionary:
	//    'in.Val.SerializeNoDict()' should do the trick (I'd check if it produces a valid output and compare it to
	//    'SerializeTruncate' which we use more widely)

	// Write the data to the local storage as usual.
	return p.putter.Put(ctx, in)
}

func (p *IngestionProxy) Stop() {
	// Flush buffer/queue.
}
