import { uiReducer, initialUIState } from './views';

describe('View Reducer', () => {
  it('should save UI values', () => {
    const state = uiReducer(undefined, {});
    expect(state).toEqual(initialUIState);

    const state2 = uiReducer(
      { sidebar: false },
      { type: 'SET_UI_VALUE', payload: { path: 'test', value: 'test' } }
    );
    expect(state2).toEqual({ sidebar: false, test: 'test' });
  });
});
