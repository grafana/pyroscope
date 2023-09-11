import { renderDiff, RenderDiffResponse } from '@pyroscope/services/render';
import { RequestAbortedError } from '@pyroscope/services/base';
import { addNotification } from '../notifications';
import { createAsyncThunk } from '../../async-thunk';
import { ContinuousState } from './state';

let diffViewAbortController: AbortController | undefined;

export const fetchDiffView = createAsyncThunk<
  { profile: RenderDiffResponse },
  {
    leftQuery: string;
    leftFrom: string;
    leftUntil: string;
    rightQuery: string;
    rightFrom: string;
    rightUntil: string;
  },
  { state: { continuous: ContinuousState } }
>('continuous/diffView', async (params, thunkAPI) => {
  if (diffViewAbortController) {
    diffViewAbortController.abort();
  }

  diffViewAbortController = new AbortController();
  thunkAPI.signal = diffViewAbortController.signal;

  const state = thunkAPI.getState();
  const res = await renderDiff(
    {
      ...params,
      maxNodes: state.continuous.maxNodes,
    },
    diffViewAbortController
  );

  if (res.isOk) {
    return Promise.resolve({ profile: res.value });
  }

  if (res.isErr && res.error instanceof RequestAbortedError) {
    return thunkAPI.rejectWithValue({ rejectedWithValue: 'reloading' });
  }

  thunkAPI.dispatch(
    addNotification({
      type: 'danger',
      title: 'Failed to load diff view',
      message: res.error.message,
    })
  );

  return Promise.reject(res.error);
});
