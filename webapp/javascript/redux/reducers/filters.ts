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
  RECEIVE_PYROSCOPE_APP_DATA,
  REQUEST_PYROSCOPE_APP_DATA,
  CANCEL_PYROSCOPE_APP_DATA,
  REQUEST_COMPARISON_APP_DATA,
  REQUEST_COMPARISON_DIFF_APP_DATA,
  RECEIVE_COMPARISON_DIFF_APP_DATA,
  RECEIVE_COMPARISON_TIMELINE,
  REQUEST_COMPARISON_TIMELINE,
  CANCEL_COMPARISON_APP_DATA,
  CANCEL_COMPARISON_DIFF_APP_DATA,
  SET_ADHOC_FILE,
  SET_ADHOC_LEFT_FILE,
  SET_ADHOC_RIGHT_FILE,
  REQUEST_ADHOC_PROFILES,
  RECEIVE_ADHOC_PROFILES,
  CANCEL_ADHOC_PROFILES,
  SET_ADHOC_PROFILE,
  REQUEST_ADHOC_PROFILE,
  RECEIVE_ADHOC_PROFILE,
  CANCEL_ADHOC_PROFILE,
  SET_ADHOC_LEFT_PROFILE,
  REQUEST_ADHOC_LEFT_PROFILE,
  RECEIVE_ADHOC_LEFT_PROFILE,
  CANCEL_ADHOC_LEFT_PROFILE,
  SET_ADHOC_RIGHT_PROFILE,
  REQUEST_ADHOC_RIGHT_PROFILE,
  RECEIVE_ADHOC_RIGHT_PROFILE,
  CANCEL_ADHOC_RIGHT_PROFILE,
  REQUEST_ADHOC_PROFILE_DIFF,
  RECEIVE_ADHOC_PROFILE_DIFF,
  CANCEL_ADHOC_PROFILE_DIFF,
} from '../actionTypes';

import { deltaDiffWrapper } from '../../util/flamebearer';

const defaultName = (window as any).initialState.appNames.find(
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
  names: (window as any).initialState.appNames,
  timeline: null,
  single: {
    flamebearer: null,
  },
  comparison: {
    rawLeft: null,
    rawRight: null,
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
  // TODO(eh-am): add proper types
  adhocSingle: {
    raw: null as any,
    file: null as any,
    profile: null as any,
    flamebearer: null as any,
    isProfileLoading: false,
  },
  adhocShared: {
    left: {
      profile: null as any,
    },
    right: {
      profile: null as any,
    },
  },
  adhocComparison: {
    left: {
      file: null as any,
      raw: null as any,
      flamebearer: null as any,
      isProfileLoading: false,
    },
    right: {
      file: null as any,
      raw: null as any,
      flamebearer: null as any,
      isProfileLoading: false,
    },
  },
  adhocComparisonDiff: {
    flamebearer: null as any,
    raw: null as any,
    isProfileLoading: false,
  },
  serviceDiscovery: {
    data: [],
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

    case SET_ADHOC_FILE:
      ({
        payload: { file, flamebearer },
      } = action);
      return {
        ...state,
        adhocSingle: {
          ...state.adhocSingle,
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
        adhocShared: {
          ...state.adhocShared,
          left: {
            ...state.adhocShared.left,
            profile: null,
          },
        },
        adhocComparison: {
          ...state.adhocComparison,
          left: {
            ...state.adhocComparison.left,
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
        adhocShared: {
          ...state.adhocShared,
          right: {
            ...state.adhocShared.right,
            profile: null,
          },
        },
        adhocComparison: {
          ...state.adhocComparison,
          right: {
            ...state.adhocComparison.right,
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
    case CANCEL_ADHOC_PROFILES:
      return {
        ...state,
        areProfilesLoading: false,
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
          raw: JSON.parse(JSON.stringify(flamebearer)),
          ...state.adhocSingle,
          flamebearer: decodeFlamebearer(flamebearer),
          isProfileLoading: false,
        },
      };
    case CANCEL_ADHOC_PROFILE:
      return {
        ...state,
        adhocSingle: {
          ...state.adhocSingle,
          isProfileLoading: false,
        },
      };

    /******************************/
    /*      Adhoc Comparison      */
    /******************************/
    case SET_ADHOC_LEFT_PROFILE:
      ({
        payload: { profile },
      } = action);
      return {
        ...state,
        adhocShared: {
          ...state.adhocShared,
          left: {
            ...state.adhocShared.left,
            profile,
          },
        },
        adhocComparison: {
          ...state.adhocComparison,
          left: {
            ...state.adhocComparison.left,
            file: null,
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
            raw: JSON.parse(JSON.stringify(flamebearer)),
            ...state.adhocComparison.left,
            flamebearer: decodeFlamebearer(flamebearer),
            isProfileLoading: false,
          },
        },
      };
    case CANCEL_ADHOC_LEFT_PROFILE:
      return {
        ...state,
        adhocComparison: {
          ...state.adhocComparison,
          left: {
            ...state.adhocComparison.left,
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
        adhocShared: {
          ...state.adhocShared,
          right: {
            ...state.adhocShared.right,
            profile,
          },
        },
        adhocComparison: {
          ...state.adhocComparison,
          right: {
            ...state.adhocComparison.right,
            file: null,
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
            raw: JSON.parse(JSON.stringify(flamebearer)),
            ...state.adhocComparison.right,
            flamebearer: decodeFlamebearer(flamebearer),
            isProfileLoading: false,
          },
        },
      };
    case CANCEL_ADHOC_RIGHT_PROFILE:
      return {
        ...state,
        adhocComparison: {
          ...state.adhocComparison,
          right: {
            ...state.adhocComparison.right,
            isProfileLoading: false,
          },
        },
      };
    case REQUEST_ADHOC_PROFILE_DIFF:
      return {
        ...state,
        adhocComparisonDiff: {
          ...state.adhocComparisonDiff,
          isProfileLoading: true,
        },
      };
    case RECEIVE_ADHOC_PROFILE_DIFF:
      ({
        payload: { flamebearer },
      } = action);
      return {
        ...state,
        adhocComparisonDiff: {
          ...state.adhocComparisonDiff,
          raw: JSON.parse(JSON.stringify(flamebearer)),
          flamebearer: decodeFlamebearer(flamebearer),
          isProfileLoading: false,
        },
      };
    case CANCEL_ADHOC_PROFILE_DIFF:
      return {
        ...state,
        adhocComparisonDiff: {
          ...state.adhocComparisonDiff,
          isProfileLoading: false,
        },
      };
    default:
      return state;
  }
}
