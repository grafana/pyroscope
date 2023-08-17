import { renderSingle, RenderOutput } from '@pyroscope/services/render';
import { RequestAbortedError } from '@pyroscope/services/base';
import { addNotification } from '../notifications';
import { createAsyncThunk } from '../../async-thunk';
import { ContinuousState } from './state';

let comparisonSideAbortControllerLeft: AbortController | undefined;
let comparisonSideAbortControllerRight: AbortController | undefined;

export const fetchComparisonSide = createAsyncThunk<
  { side: 'left' | 'right'; data: Pick<RenderOutput, 'profile'> },
  { side: 'left' | 'right'; query: string },
  { state: { continuous: ContinuousState } }
>('continuous/fetchComparisonSide', async ({ side, query }, thunkAPI) => {
  const state = thunkAPI.getState();

  const res = await (() => {
    switch (side) {
      case 'left': {
        if (comparisonSideAbortControllerLeft) {
          comparisonSideAbortControllerLeft.abort();
        }

        comparisonSideAbortControllerLeft = new AbortController();
        thunkAPI.signal = comparisonSideAbortControllerLeft.signal;

        return renderSingle(
          {
            ...state.continuous,
            query,

            from: state.continuous.leftFrom,
            until: state.continuous.leftUntil,
          },
          comparisonSideAbortControllerLeft
        );
      }
      case 'right': {
        if (comparisonSideAbortControllerRight) {
          comparisonSideAbortControllerRight.abort();
        }

        comparisonSideAbortControllerRight = new AbortController();
        thunkAPI.signal = comparisonSideAbortControllerRight.signal;

        return renderSingle(
          {
            ...state.continuous,
            query,

            from: state.continuous.rightFrom,
            until: state.continuous.rightUntil,
          },
          comparisonSideAbortControllerRight
        );
      }
      default: {
        throw new Error('invalid side');
      }
    }
  })();

  if (res?.isErr && res?.error instanceof RequestAbortedError) {
    return thunkAPI.rejectWithValue({ rejectedWithValue: 'reloading' });
  }

  if (res.isOk) {
    return Promise.resolve({
      side,
      data: {
        profile: res.value.profile,
      },
    });
  }

  thunkAPI.dispatch(
    addNotification({
      type: 'danger',
      title: `Failed to load the ${side} side comparison`,
      message: res.error.message,
    })
  );

  return Promise.reject(res.error);
});
