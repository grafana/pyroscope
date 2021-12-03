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
  SET_FILE,
  SET_LEFT_FILE,
  SET_RIGHT_FILE,
} from '../actionTypes';

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
    case RECEIVE_PYRESCOPE_APP_DATA:
      data = action.payload.data;
      timeline = data.timeline;
      flamebearer = data.flamebearer;

      return {
        ...state,
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
      flamebearer = action.payload.data.flamebearer;

      let left;
      let right;
      switch (viewSide) {
        case 'left':
          left = { flamebearer };
          right = state.comparison.right;
          break;

        case 'right': {
          left = state.comparison.left;
          right = { flamebearer };
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
      const { leftTicks, rightTicks } = data;

      return {
        ...state,
        timeline: decodeTimelineData(timeline),
        diff: {
          flamebearer: {
            leftTicks,
            rightTicks,
            ...data.flamebearer,
          },
        },
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
          flamebearer: action.payload.flamebearer,
        },
      };
    case SET_LEFT_FILE:
      return {
        ...state,
        adhocComparison: {
          ...state.adhocComparison,
          left: {
            file: action.payload.file,
            flamebearer: action.payload.flamebearer,
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
            flamebearer: action.payload.flamebearer,
          },
        },
      };
    default:
      return state;
  }
}
