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

/**
 * Calculates the oldest and most recent time ranges from `state`.
 *
 * @param state The redux state
 * @param includeLeftAndRight If true, include the left and right time ranges in the calculation
 * @returns The maximum time range possible
 */
function getTimeRange(
  state: ContinuousState,
  includeLeftAndRight = false
): { from: number; until: number } {
  if (includeLeftAndRight) {
    return biggestTimeRangeInUnix(state);
  }

  const [from, until] = [state.from, state.until]
    .map(formatAsOBject)
    .map((d) => d.getTime());
  return { from, until };
}

/**
 * The `query` field is the query by which to filter the tags. The
 * `includeLeftAndRight` field is true when the left and right time ranges
 * should be used to calculate the final time range to query. If it's false, it
 * will use only the primary time range.
 */
export type FetchTagsQuery = {
  query: Query;
  includeLeftAndRight: boolean;
};

/**
 * Fetch label names for a given time range. Use
 * `FetchTagsQuery.includeLeftAndRight` to customize the time range.
 */
export const fetchTags = createAsyncThunk<
  {
    appName: string;
    tags: string[];
    from: number;
    until: number;
  },
  FetchTagsQuery,
  { state: { continuous: ContinuousState } }
>(
  'continuous/fetchTags',
  async (fetchTagsQuery: FetchTagsQuery, thunkAPI) => {
    const { query, includeLeftAndRight } = fetchTagsQuery;

    const appName = assertIsValidAppName(query);
    const { from, until } = getTimeRange(
      thunkAPI.getState().continuous,
      includeLeftAndRight
    );

    const res = await tagsService.fetchTags(query, from, until);

    if (res.isOk) {
      return Promise.resolve({
        appName,
        tags: res.value,
        from,
        until,
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
    condition: (fetchTagsQuery, thunkAPI) => {
      const { query } = fetchTagsQuery;
      const appName = assertIsValidAppName(query);

      const state = thunkAPI.getState().continuous;
      const { from, until } = getTimeRange(state, true);

      const tagsState = state.tags[appName];

      // Haven't loaded yet
      if (!tagsState) {
        return true;
      }

      // Already loading that tag
      if (tagsState.type === 'loading') {
        return false;
      }

      // Any other state that's not loaded
      if (tagsState.type !== 'loaded') {
        return true;
      }

      const isInRange = (target: number) => {
        return target >= tagsState.from && target <= tagsState.until;
      };

      const isSmallerThanLoaded = isInRange(from) && isInRange(until);
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
