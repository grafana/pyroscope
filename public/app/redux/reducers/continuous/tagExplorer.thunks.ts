import {
  RenderExploreOutput,
  renderSingle,
  RenderOutput,
  renderExplore,
} from '@pyroscope/services/render';
import { RequestAbortedError } from '@pyroscope/services/base';
import { appendLabelToQuery } from '@pyroscope/util/query';
import { addNotification } from '../notifications';
import { createAsyncThunk } from '../../async-thunk';
import { ContinuousState } from './state';

export const ALL_TAGS = 'All';

let tagExplorerViewAbortController: AbortController | undefined;
let tagExplorerViewProfileAbortController: AbortController | undefined;

export const fetchTagExplorerView = createAsyncThunk<
  RenderExploreOutput,
  null,
  { state: { continuous: ContinuousState } }
>('continuous/tagExplorerView', async (_, thunkAPI) => {
  if (tagExplorerViewAbortController) {
    tagExplorerViewAbortController.abort();
  }

  tagExplorerViewAbortController = new AbortController();
  thunkAPI.signal = tagExplorerViewAbortController.signal;

  const state = thunkAPI.getState();
  const res = await renderExplore(
    {
      query: state.continuous.query,
      from: state.continuous.from,
      until: state.continuous.until,
      groupBy: state.continuous.tagExplorerView.groupByTag,
      grouByTagValue: state.continuous.tagExplorerView.groupByTagValue,
      refreshToken: state.continuous.refreshToken,
    },
    tagExplorerViewAbortController
  );

  if (res.isOk) {
    return Promise.resolve(res.value);
  }

  if (res.isErr && res.error instanceof RequestAbortedError) {
    return Promise.reject(res.error);
  }

  thunkAPI.dispatch(
    addNotification({
      type: 'danger',
      title: 'Failed to load explore view data',
      message: res.error.message,
    })
  );

  return Promise.reject(res.error);
});

export const fetchTagExplorerViewProfile = createAsyncThunk<
  RenderOutput,
  null,
  { state: { continuous: ContinuousState } }
>('continuous/fetchTagExplorerViewProfile', async (_, thunkAPI) => {
  if (tagExplorerViewProfileAbortController) {
    tagExplorerViewProfileAbortController.abort();
  }

  tagExplorerViewProfileAbortController = new AbortController();
  thunkAPI.signal = tagExplorerViewProfileAbortController.signal;

  const state = thunkAPI.getState();
  const { groupByTag, groupByTagValue } = state.continuous.tagExplorerView;
  // if "All" option is selected we dont need to modify query to fetch profile
  const queryProps =
    ALL_TAGS === groupByTagValue
      ? { groupBy: groupByTag, query: state.continuous.query }
      : {
          query: appendLabelToQuery(
            state.continuous.query,
            state.continuous.tagExplorerView.groupByTag,
            state.continuous.tagExplorerView.groupByTagValue
          ),
        };
  const res = await renderSingle(
    {
      ...state.continuous,
      ...queryProps,
    },
    tagExplorerViewProfileAbortController
  );

  if (res.isOk) {
    return Promise.resolve(res.value);
  }

  if (res.isErr && res.error instanceof RequestAbortedError) {
    return Promise.reject(res.error);
  }

  thunkAPI.dispatch(
    addNotification({
      type: 'danger',
      title: 'Failed to load explore view profile',
      message: res.error.message,
    })
  );

  return Promise.reject(res.error);
});
