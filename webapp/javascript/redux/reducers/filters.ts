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
  CANCEL_PYRESCOPE_APP_DATA,
  REQUEST_COMPARISON_APP_DATA,
  REQUEST_COMPARISON_DIFF_APP_DATA,
  RECEIVE_COMPARISON_DIFF_APP_DATA,
  RECEIVE_COMPARISON_TIMELINE,
  REQUEST_COMPARISON_TIMELINE,
  SET_FILE,
  SET_LEFT_FILE,
  SET_RIGHT_FILE,
  CANCEL_COMPARISON_APP_DATA,
  CANCEL_COMPARISON_DIFF_APP_DATA,
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
    flamebearer: null,
  },
  adhocComparison: {
    left: {
      file: null,
      flamebearer: null,
    },
    right: {
      file: null,
      flamebearer: null,
    },
  },
  isJSONLoading: false,
  maxNodes: 1024,
  tags: [],
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
  let flamebearer;
  let timeline;
  let data;
  let viewSide;
  switch (action.type) {
    case SET_DATE_RANGE:
      return {
        ...state,
        from: action.payload.from,
        until: action.payload.until,
      };
    case SET_FROM:
      return {
        ...state,
        from: action.payload.from,
      };
    case SET_LEFT_FROM:
      return {
        ...state,
        leftFrom: action.payload.from,
      };
    case SET_RIGHT_FROM:
      return {
        ...state,
        rightFrom: action.payload.from,
      };
    case SET_UNTIL:
      return {
        ...state,
        until: action.payload.until,
      };
    case SET_LEFT_UNTIL:
      return {
        ...state,
        leftUntil: action.payload.until,
      };
    case SET_RIGHT_UNTIL:
      return {
        ...state,
        rightUntil: action.payload.until,
      };
    case SET_LEFT_DATE_RANGE:
      return {
        ...state,
        leftFrom: action.payload.from,
        leftUntil: action.payload.until,
      };
    case SET_RIGHT_DATE_RANGE:
      return {
        ...state,
        rightFrom: action.payload.from,
        rightUntil: action.payload.until,
      };
    case SET_MAX_NODES:
      return {
        ...state,
        maxNodes: action.payload.maxNodes,
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

    case CANCEL_PYRESCOPE_APP_DATA:
    case CANCEL_COMPARISON_APP_DATA:
    case CANCEL_COMPARISON_DIFF_APP_DATA:
      return {
        ...state,
        isJSONLoading: false,
      };

    case RECEIVE_PYRESCOPE_APP_DATA:
      data = action.payload.data;
      // since we gonna mutate that data, keep a reference to the old one
      const raw = JSON.parse(JSON.stringify(data));
      timeline = data.timeline;
      flamebearer = decodeFlamebearer({ ...data });
      return {
        ...state,
        raw,
        timeline: decodeTimelineData(timeline),
        single: { flamebearer },
        isJSONLoading: false,
      };

    case REQUEST_COMPARISON_APP_DATA:
      return {
        ...state,
        isJSONLoading: true,
      };
    case RECEIVE_COMPARISON_APP_DATA:
      viewSide = action.payload.viewSide;
      flamebearer = action.payload.data;

      let left;
      let right;
      let rawLeft;
      let rawRight;
      switch (viewSide) {
        case 'left':
          // since we gonna mutate that data, keep a reference to the old one
          rawLeft = JSON.parse(JSON.stringify(flamebearer));

          left = { flamebearer: decodeFlamebearer(flamebearer) };
          right = state.comparison.right;
          rawRight = state.comparison.rawRight;
          break;

        case 'right': {
          // since we gonna mutate that data, keep a reference to the old one
          rawRight = JSON.parse(JSON.stringify(flamebearer));

          left = state.comparison.left;
          right = { flamebearer: decodeFlamebearer(flamebearer) };
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
      return {
        ...state,
        timeline: decodeTimelineData(action.payload.timeline),
        isJSONLoading: false,
      };

    case REQUEST_COMPARISON_DIFF_APP_DATA:
      return {
        ...state,
        isJSONLoading: true,
      };
    case RECEIVE_COMPARISON_DIFF_APP_DATA:
      data = action.payload.data;
      timeline = data.timeline;
      flamebearer = decodeFlamebearer(data);

      return {
        ...state,
        timeline: decodeTimelineData(timeline),
        diff: { flamebearer },
        isJSONLoading: false,
      };

    case REQUEST_TAGS:
      return {
        ...state,
        areTagsLoading: true,
      };
    case RECEIVE_TAGS: {
      return {
        ...state,
        areTagsLoading: false,
        tags: action.payload.tags.reduce((acc, tag) => {
          if (tag !== '__name__') {
            acc[tag] = [];
          }
          return acc;
        }, {}),
      };
    }
    case REQUEST_TAG_VALUES:
      return {
        ...state,
        tagValuesLoading: action.payload.tag,
      };
    case RECEIVE_TAG_VALUES:
      return {
        ...state,
        tagValuesLoading: '',
        tags: {
          ...state.tags,
          [action.payload.tag]: action.payload.values,
        },
      };
    case REQUEST_NAMES:
      return {
        ...state,
        areNamesLoading: true,
      };
    case RECEIVE_NAMES:
      return {
        ...state,
        names: action.payload.names,
        areNamesLoading: false,
      };
    case SET_QUERY:
      return {
        ...state,
        query: action.payload.query,
      };
    case SET_FILE:
      return {
        ...state,
        adhocSingle: {
          file: action.payload.file,
          flamebearer: decodeFlamebearer(action.payload.flamebearer),
        },
      };
    case SET_LEFT_FILE:
      return {
        ...state,
        adhocComparison: {
          ...state.adhocComparison,
          left: {
            file: action.payload.file,
            flamebearer: decodeFlamebearer(action.payload.flamebearer),
          },
        },
      };
    case SET_RIGHT_FILE:
      return {
        ...state,
        adhocComparison: {
          ...state.adhocComparison,
          right: {
            file: action.payload.file,
            flamebearer: decodeFlamebearer(action.payload.flamebearer),
          },
        },
      };
    default:
      return state;
  }
}
