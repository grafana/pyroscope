package history

import "context"

type NoopManager struct{}

func (*NoopManager) Add(_ context.Context, _ *Entry) (QueryID, error) {
	return "", nil
}
func (*NoopManager) Get(_ context.Context, _ QueryID) (*Entry, error) {
	return nil, nil
}
func (*NoopManager) List(_ context.Context, _ string) ([]*Entry, string, error) {
	return []*Entry{}, "", nil
}
