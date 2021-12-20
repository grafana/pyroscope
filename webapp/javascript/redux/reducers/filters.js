import {
  SET_DATE_RANGE,
  SET_FROM,
  SET_UNTIL,
  SET_MAX_NODES,
  REFRESH,
  REQUEST_TAGS,
  RECEIVE_TAGS,
  REQUEST_TAG_VALUES,
  RECEIVE_TAG_VALUES,
  RECEIVE_NAMES,
  REQUEST_NAMES,
  SET_LEFT_DATE_RANGE,
  SET_RIGHT_DATE_RANGE,
  SET_LEFT_FROM,
  SET_RIGHT_FROM,
  SET_LEFT_UNTIL,
  SET_RIGHT_UNTIL,
  SET_QUERY,
  RECEIVE_COMPARISON_APP_DATA,
  RECEIVE_PYRESCOPE_APP_DATA,
  REQUEST_PYRESCOPE_APP_DATA,
  REQUEST_COMPARISON_APP_DATA,
  REQUEST_COMPARISON_DIFF_APP_DATA,
  RECEIVE_COMPARISON_DIFF_APP_DATA,
  RECEIVE_COMPARISON_TIMELINE,
  REQUEST_COMPARISON_TIMELINE,
  SET_ADHOC_FILE,
  SET_ADHOC_LEFT_FILE,
  SET_ADHOC_RIGHT_FILE,
  REQUEST_ADHOC_PROFILES,
  RECEIVE_ADHOC_PROFILES,
  SET_ADHOC_PROFILE,
  REQUEST_ADHOC_PROFILE,
  RECEIVE_ADHOC_PROFILE,
  SET_ADHOC_LEFT_PROFILE,
  REQUEST_ADHOC_LEFT_PROFILE,
  RECEIVE_ADHOC_LEFT_PROFILE,
  SET_ADHOC_RIGHT_PROFILE,
  REQUEST_ADHOC_RIGHT_PROFILE,
  RECEIVE_ADHOC_RIGHT_PROFILE,
} from '../actionTypes';

import { deltaDiffWrapper } from '../../util/flamebearer';

const defaultName = window.initialState.appNames.find(
  (x) => x !== 'pyroscope.server.cpu'
);

const initialState = {
  from: 'now-1h',
  leftFrom: 'now-1h',
  rightFrom: 'now-30m',
  until: 'now',
  leftUntil: 'now-30m',
  rightUntil: 'now',
  query: `${defaultName || 'pyroscope.server.cpu'}{}`,
  names: window.initialState.appNames,
  timeline: null,
  single: {
    flamebearer: null,
  },
  comparison: {
    left: {
      flamebearer: null,
    },
    right: {
      flamebearer: null,
    },
  },
  diff: {
    flamebearer: null,
  },
  adhocSingle: {
    file: null,
    profile: null,
    flamebearer: null,
    isProfileLoading: false,
  },
  adhocComparison: {
    left: {
      file: null,
      profile: null,
      flamebearer: null,
      isProfileLoading: false,
    },
    right: {
      file: null,
      profile: null,
      flamebearer: null,
      isProfileLoading: false,
    },
  },
  isJSONLoading: false,
  maxNodes: 1024,
  tags: [],
  profiles: null,
  areProfilesLoading: false,
};

function decodeTimelineData(timelineData) {
  if (!timelineData) {
    return [];
  }
  let time = timelineData.startTime;
  return timelineData.samples.map((x) => {
    const res = [time * 1000, x];
    time += timelineData.durationDelta;
    return res;
  });
}

function decodeFlamebearer({
  flamebearer,
  metadata,
  leftTicks,
  rightTicks,
  version,
}) {
  const fb = {
    ...flamebearer,
    format: metadata.format,
    spyName: metadata.spyName,
    sampleRate: metadata.sampleRate,
    units: metadata.units,
  };
  if (fb.format === 'double') {
    fb.leftTicks = leftTicks;
    fb.rightTicks = rightTicks;
  }
  fb.version = version || 0;
  fb.levels = deltaDiffWrapper(fb.format, fb.levels);
  return fb;
}

export default function (state = initialState, action) {
  const { type } = action;
  let data;
  let file;
  let flamebearer;
  let from;
  let maxNodes;
  let names;
  let profile;
  let profiles;
  let query;
  let tag;
  let tags;
  let timeline;
  let until;
  let values;
  let viewSide;

  switch (type) {
    case SET_DATE_RANGE:
      ({
        payload: { from, until },
      } = action);
      return {
        ...state,
        from,
        until,
      };
    case SET_FROM:
      ({
        payload: { from },
      } = action);
      return {
        ...state,
        from,
      };
    case SET_LEFT_FROM:
      ({
        payload: { from },
      } = action);
      return {
        ...state,
        leftFrom: from,
      };
    case SET_RIGHT_FROM:
      ({
        payload: { from },
      } = action);
      return {
        ...state,
        rightFrom: from,
      };
    case SET_UNTIL:
      ({
        payload: { until },
      } = action);
      return {
        ...state,
        until,
      };
    case SET_LEFT_UNTIL:
      ({
        payload: { until },
      } = action);
      return {
        ...state,
        leftUntil: until,
      };
    case SET_RIGHT_UNTIL:
      ({
        payload: { until },
      } = action);
      return {
        ...state,
        rightUntil: until,
      };
    case SET_LEFT_DATE_RANGE:
      ({
        payload: { from, until },
      } = action);
      return {
        ...state,
        leftFrom: from,
        leftUntil: until,
      };
    case SET_RIGHT_DATE_RANGE:
      ({
        payload: { from, until },
      } = action);
      return {
        ...state,
        rightFrom: from,
        rightUntil: until,
      };
    case SET_MAX_NODES:
      ({
        payload: { maxNodes },
      } = action);
      return {
        ...state,
        maxNodes,
      };
    case REFRESH:
      return {
        ...state,
        refreshToken: Math.random(),
      };

    case REQUEST_PYRESCOPE_APP_DATA:
      return {
        ...state,
        isJSONLoading: true,
      };
    case RECEIVE_PYRESCOPE_APP_DATA:
      ({
        payload: { data },
      } = action);
      ({ timeline } = data);
      // since we gonna mutate that data, keep a reference to the old one
      const raw = JSON.parse(JSON.stringify(data));
      return {
        ...state,
        raw,
        timeline: decodeTimelineData(timeline),
        single: { flamebearer: decodeFlamebearer({ ...data }) },
        isJSONLoading: false,
      };

    case REQUEST_COMPARISON_APP_DATA:
      return {
        ...state,
        isJSONLoading: true,
      };
    case RECEIVE_COMPARISON_APP_DATA:
      ({
        payload: { data, viewSide },
      } = action);
      let left;
      let right;
      let rawLeft;
      let rawRight;
      switch (viewSide) {
        case 'left':
          // since we gonna mutate that data, keep a reference to the old one
          rawLeft = JSON.parse(JSON.stringify(data));

          left = { flamebearer: decodeFlamebearer(data) };
          right = state.comparison.right;
          rawRight = state.comparison.rawRight;
          break;

        case 'right': {
          // since we gonna mutate that data, keep a reference to the old one
          rawRight = JSON.parse(JSON.stringify(data));

          left = state.comparison.left;
          right = { flamebearer: decodeFlamebearer(data) };
          rawLeft = state.comparison.rawLeft;
          break;
        }
        default:
          throw new Error(`Invalid viewSide: '${viewSide}'`);
      }

      return {
        ...state,
        comparison: {
          left,
          right,
          rawLeft,
          rawRight,
        },
        isJSONLoading: false,
      };
    case REQUEST_COMPARISON_TIMELINE:
      return {
        ...state,
        isJSONLoading: true,
      };
    case RECEIVE_COMPARISON_TIMELINE:
      ({
        payload: { timeline },
      } = action);
      return {
        ...state,
        timeline: decodeTimelineData(timeline),
        isJSONLoading: false,
      };

    case REQUEST_COMPARISON_DIFF_APP_DATA:
      return {
        ...state,
        isJSONLoading: true,
      };
    case RECEIVE_COMPARISON_DIFF_APP_DATA:
      ({
        payload: { data },
      } = action);
      ({ timeline } = data);
      return {
        ...state,
        timeline: decodeTimelineData(timeline),
        diff: { flamebearer: decodeFlamebearer(data) },
        isJSONLoading: false,
      };

    case REQUEST_TAGS:
      return {
        ...state,
        areTagsLoading: true,
      };
    case RECEIVE_TAGS: {
      ({
        payload: { tags },
      } = action);
      return {
        ...state,
        areTagsLoading: false,
        tags: tags.reduce((acc, tag) => {
          if (tag !== '__name__') {
            acc[tag] = [];
          }
          return acc;
        }, {}),
      };
    }
    case REQUEST_TAG_VALUES:
      ({
        payload: { tag },
      } = action);
      return {
        ...state,
        tagValuesLoading: tag,
      };
    case RECEIVE_TAG_VALUES:
      ({
        payload: { tag, values },
      } = action);
      return {
        ...state,
        tagValuesLoading: '',
        tags: {
          ...state.tags,
          [tag]: values,
        },
      };
    case REQUEST_NAMES:
      return {
        ...state,
        areNamesLoading: true,
      };
    case RECEIVE_NAMES:
      ({
        payload: { names },
      } = action);
      return {
        ...state,
        names,
        areNamesLoading: false,
      };
    case SET_QUERY:
      ({
        payload: { query },
      } = action);
      return {
        ...state,
        query,
      };
    case SET_ADHOC_FILE:
      ({
        payload: { file, flamebearer },
      } = action);
      return {
        ...state,
        adhocSingle: {
          profile: null,
          file,
          flamebearer: flamebearer ? decodeFlamebearer(flamebearer) : null,
        },
      };
    case SET_ADHOC_LEFT_FILE:
      ({
        payload: { file, flamebearer },
      } = action);
      return {
        ...state,
        adhocComparison: {
          ...state.adhocComparison,
          left: {
            ...state.adhocComparison.left,
            profile: null,
            file,
            flamebearer: flamebearer ? decodeFlamebearer(flamebearer) : null,
          },
        },
      };
    case SET_ADHOC_RIGHT_FILE:
      ({
        payload: { file, flamebearer },
      } = action);
      return {
        ...state,
        adhocComparison: {
          ...state.adhocComparison,
          right: {
            ...state.adhocComparison.right,
            profile: null,
            file,
            flamebearer: flamebearer ? decodeFlamebearer(flamebearer) : null,
          },
        },
      };
    case REQUEST_ADHOC_PROFILES:
      return {
        ...state,
        areProfilesLoading: true,
      };
    case RECEIVE_ADHOC_PROFILES:
      ({
        payload: { profiles },
      } = action);
      return {
        ...state,
        areProfilesLoading: false,
        profiles,
      };
    case SET_ADHOC_PROFILE:
      ({
        payload: { profile },
      } = action);
      return {
        ...state,
        adhocSingle: {
          ...state.adhocSingle,
          file: null,
          profile,
        },
      };
    case REQUEST_ADHOC_PROFILE:
      return {
        ...state,
        adhocSingle: {
          ...state.adhocSingle,
          isProfileLoading: true,
        },
      };
    case RECEIVE_ADHOC_PROFILE:
      ({
        payload: { flamebearer },
      } = action);
      return {
        ...state,
        adhocSingle: {
          ...state.adhocSingle,
          flamebearer: decodeFlamebearer(flamebearer),
          isProfileLoading: false,
        },
      };
    case SET_ADHOC_LEFT_PROFILE:
      ({
        payload: { profile },
      } = action);
      return {
        ...state,
        adhocComparison: {
          ...state.adhocComparison,
          left: {
            ...state.adhocComparison.left,
            file: null,
            profile,
          },
        },
      };
    case REQUEST_ADHOC_LEFT_PROFILE:
      return {
        ...state,
        adhocComparison: {
          ...state.adhocComparison,
          left: {
            ...state.adhocComparison.left,
            isProfileLoading: true,
          },
        },
      };
    case RECEIVE_ADHOC_LEFT_PROFILE:
      ({
        payload: { flamebearer },
      } = action);
      return {
        ...state,
        adhocComparison: {
          ...state.adhocComparison,
          left: {
            ...state.adhocComparison.left,
            flamebearer: decodeFlamebearer(flamebearer),
            isProfileLoading: false,
          },
        },
      };
    case SET_ADHOC_RIGHT_PROFILE:
      ({
        payload: { profile },
      } = action);
      return {
        ...state,
        adhocComparison: {
          ...state.adhocComparison,
          right: {
            ...state.adhocComparison.right,
            file: null,
            profile,
          },
        },
      };
    case REQUEST_ADHOC_RIGHT_PROFILE:
      return {
        ...state,
        adhocComparison: {
          ...state.adhocComparison,
          right: {
            ...state.adhocComparison.right,
            isProfileLoading: true,
          },
        },
      };
    case RECEIVE_ADHOC_RIGHT_PROFILE:
      ({
        payload: { flamebearer },
      } = action);
      return {
        ...state,
        adhocComparison: {
          ...state.adhocComparison,
          right: {
            ...state.adhocComparison.right,
            flamebearer: decodeFlamebearer(flamebearer),
            isProfileLoading: false,
          },
        },
      };
    default:
      return state;
  }
}
