import { Query, queryToAppName } from '@pyroscope/models/query';
import * as tagsService from '@pyroscope/services/tags';
import { createBiggestInterval } from '@pyroscope/util/timerange';
import { formatAsOBject } from '@pyroscope/util/formatDate';
import { ContinuousState } from './state';
import { addNotification } from '../notifications';
import { createAsyncThunk } from '../../async-thunk';

function biggestTimeRangeInUnix(state: ContinuousState) {
  const getTime = (d: Date) => d.getTime();

  return createBiggestInterval({
    from: [state.from, state.leftFrom, state.rightFrom]
      .map(formatAsOBject)
      .map(getTime),
    until: [state.until, state.leftUntil, state.leftUntil]
      .map(formatAsOBject)
      .map(getTime),
  });
}

function assertIsValidAppName(query: Query) {
  const appName = queryToAppName(query);
  if (appName.isNothing) {
    throw Error(`Query '${query}' is not a valid app`);
  }

  return appName.value;
}

export const fetchTags = createAsyncThunk<
  { appName: string; tags: string[]; from: number; until: number },
  Query,
  { state: { continuous: ContinuousState } }
>(
  'continuous/fetchTags',
  async (query: Query, thunkAPI) => {
    const appName = assertIsValidAppName(query);

    const state = thunkAPI.getState().continuous;
    const timerange = biggestTimeRangeInUnix(state);
    const res = await tagsService.fetchTags(
      query,
      timerange.from,
      timerange.until
    );

    if (res.isOk) {
      return Promise.resolve({
        appName,
        tags: res.value,
        from: timerange.from,
        until: timerange.until,
      });
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to load tags',
        message: res.error.message,
      })
    );

    return Promise.reject(res.error);
  },
  {
    // If we already loaded the tags for that application
    // And we are trying to load tags for a smaller range
    // Skip it, since we most likely already have that data
    condition: (query, thunkAPI) => {
      const appName = assertIsValidAppName(query);
      const state = thunkAPI.getState().continuous;
      const timerange = biggestTimeRangeInUnix(state);

      const s = state.tags[appName];

      // Haven't loaded yet
      if (!s) {
        return true;
      }

      // Already loading that tag
      if (s.type === 'loading') {
        return false;
      }

      // Any other state that's not loaded
      if (s.type !== 'loaded') {
        return true;
      }

      const isInRange = (target: number) => {
        return target >= s.from && target <= s.until;
      };

      const isSmallerThanLoaded =
        isInRange(timerange.from) && isInRange(timerange.until);

      return !isSmallerThanLoaded;
    },
  }
);
export const fetchTagValues = createAsyncThunk<
  {
    appName: string;
    label: string;
    values: string[];
  },
  {
    query: Query;
    label: string;
  },
  { state: { continuous: ContinuousState } }
>(
  'continuous/fetchTagsValues',
  async (payload: { query: Query; label: string }, thunkAPI) => {
    const appName = assertIsValidAppName(payload.query);

    const state = thunkAPI.getState().continuous.tags[appName];
    if (!state || state.type !== 'loaded') {
      return Promise.reject(
        new Error(
          `Trying to load label-values for an unloaded label. This is likely due to a race condition.`
        )
      );
    }

    const res = await tagsService.fetchLabelValues(
      payload.label,
      payload.query,
      state.from,
      state.until
    );

    if (res.isOk) {
      return Promise.resolve({
        appName,
        label: payload.label,
        values: res.value,
      });
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to load tag values',
        message: res.error.message,
      })
    );

    return Promise.reject(res.error);
  },
  {
    condition: ({ query, label }, thunkAPI) => {
      const appName = assertIsValidAppName(query);

      // Are we trying to load values from a tag that wasn't loaded?
      // If so it's most likely due to a race condition
      const tagState = thunkAPI.getState().continuous.tags[appName];
      if (!tagState || tagState.type !== 'loaded') {
        return false;
      }

      const tagValueState = tagState.tags[label];
      // Have not being loaded yet
      if (!tagValueState) {
        return true;
      }

      // Loading or already loaded
      if (tagValueState.type === 'loading' || tagValueState.type === 'loaded') {
        return false;
      }

      return true;
    },
  }
);
