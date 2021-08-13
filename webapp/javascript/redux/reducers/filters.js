import uniqBy from "lodash/fp/uniqBy";
import {
  SET_DATE_RANGE,
  SET_FROM,
  SET_UNTIL,
  SET_MAX_NODES,
  REFRESH,
  RECEIVE_TIMELINE,
  REQUEST_TIMELINE,
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
} from "../actionTypes";

const defaultName = window.initialState.appNames.find(
  (x) => x !== "pyroscope.server.cpu"
);

const initialState = {
  from: "now-1h",
  leftFrom: "now-1h",
  rightFrom: "now-30m",
  until: "now",
  leftUntil: "now-30m",
  rightUntil: "now",
  query: `${defaultName || "pyroscope.server.cpu"}{}`,
  names: window.initialState.appNames,
  timeline: null,
  isJSONLoading: false,
  maxNodes: 1024,
  tags: [],
};

window.uniqBy = uniqBy;

function decodeTimelineData(timelineData) {
  if (!timelineData) {
    return [];
  }
  const res = [];
  let time = timelineData.startTime;
  return timelineData.samples.map((x) => {
    const res = [time * 1000, x];
    time += timelineData.durationDelta;
    return res;
  });
}

export default function (state = initialState, action) {
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
    case REQUEST_TIMELINE:
      return {
        ...state,
        isJSONLoading: true,
      };
    case RECEIVE_TIMELINE:
      return {
        ...state,
        timeline: decodeTimelineData(action.payload.timeline),
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
          if (tag !== "__name__") {
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
        tagValuesLoading: "",
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
    default:
      return state;
  }
}
