import { RenderOutput, renderSingle } from '@pyroscope/services/render';
import { RequestAbortedError } from '@pyroscope/services/base';
import { addNotification } from '../notifications';
import { createAsyncThunk } from '../../async-thunk';
import { ContinuousState } from './state';

let sideTimelinesAbortController: AbortController | undefined;

export const fetchSideTimelines = createAsyncThunk<
  { left: RenderOutput; right: RenderOutput },
  null,
  { state: { continuous: ContinuousState } }
>('continuous/fetchSideTimelines', async (_, thunkAPI) => {
  if (sideTimelinesAbortController) {
    sideTimelinesAbortController.abort();
  }

  sideTimelinesAbortController = new AbortController();
  thunkAPI.signal = sideTimelinesAbortController.signal;

  const state = thunkAPI.getState();

  const res = await Promise.all([
    renderSingle(
      {
        query: state.continuous.leftQuery || '',
        from: state.continuous.from,
        until: state.continuous.until,
        maxNodes: state.continuous.maxNodes,
        refreshToken: state.continuous.refreshToken,
      },
      sideTimelinesAbortController
    ),
    renderSingle(
      {
        query: state.continuous.rightQuery || '',
        from: state.continuous.from,
        until: state.continuous.until,
        maxNodes: state.continuous.maxNodes,
        refreshToken: state.continuous.refreshToken,
      },
      sideTimelinesAbortController
    ),
  ]);

  if (
    (res?.[0]?.isErr && res?.[0]?.error instanceof RequestAbortedError) ||
    (res?.[1]?.isErr && res?.[1]?.error instanceof RequestAbortedError) ||
    (!res && thunkAPI.signal.aborted)
  ) {
    return Promise.reject();
  }

  if (res?.[0].isOk && res?.[1].isOk) {
    return Promise.resolve({
      left: res[0].value,
      right: res[1].value,
    });
  }

  thunkAPI.dispatch(
    addNotification({
      type: 'danger',
      title: `Failed to load the timelines`,
      message: '',
      additionalInfo: [
        res?.[0].error.message,
        res?.[1].error.message,
      ] as string[],
    })
  );

  return Promise.reject(res && res.filter((a) => a?.isErr).map((a) => a.error));
});
