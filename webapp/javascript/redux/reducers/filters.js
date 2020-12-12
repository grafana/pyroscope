import {
  SET_DATE_RANGE,
  SET_FROM,
  SET_UNTIL,
  SET_MAX_NODES,
  SET_LABELS,
  REFRESH,
  ADD_LABEL,
  REMOVE_LABEL,
  RECEIVE_SVG,
  REQUEST_SVG,
  RECEIVE_JSON,
  REQUEST_JSON,
  RECEIVE_NAMES,
  REQUEST_NAMES,
} from "../actionTypes";

import uniqBy from "lodash/fp/uniqBy";
import {deltaDiff} from "../../flamebearer";

const initialState = {
  from: "now-1h",
  until: "now",
  labels: []
};
window.uniqBy = uniqBy;

export default function(state = initialState, action) {
  switch(action.type){
    case SET_DATE_RANGE:
      return {
        ...state,
        from: action.payload.from,
        until: action.payload.until
      }
    case SET_FROM:
      return {
        ...state,
        from: action.payload.from,
      }
    case SET_UNTIL:
      return {
        ...state,
        until: action.payload.until
      }
    case SET_MAX_NODES:
      return {
        ...state,
        maxNodes: action.payload.maxNodes
      }
    case REFRESH:
      return {
        ...state,
        refreshToken: Math.random(),
      }
    case SET_LABELS:
      return {...state, labels: action.payload.labels }
    case ADD_LABEL:
      return {...state, labels: uniqBy("name", [action.payload].concat(state.labels)) }
    case REMOVE_LABEL:
      return {...state,
        labels: state.labels.filter((x) => x.name !== action.payload.name)
      }
    case REQUEST_SVG:
      return {...state,
        isSVGLoading: true,
      }
    case RECEIVE_SVG:
      let i = action.payload.data.indexOf("\n");
      return {...state,
        timeline: decodeTimelineData(JSON.parse(action.payload.data.substring(0, i))),
        svg: action.payload.data.substring(i+1),
        isSVGLoading: false,
      }
    case REQUEST_JSON:
      return {...state,
        isJSONLoading: true,
      }
    case RECEIVE_JSON:
      deltaDiff(action.payload.flamebearer.levels);
      return {...state,
        timeline: decodeTimelineData(action.payload.timeline),
        flamebearer: action.payload.flamebearer,
        isJSONLoading: false,
      }
    case REQUEST_NAMES:
      return {...state,
        areNamesLoading: true,
      }
    case RECEIVE_NAMES:
      let labels = state.labels;
      let firstName = action.payload.names[0] || "none";
      if (labels.filter((x) => x.name == "__name__").length == 0){
        labels = labels.concat([{name: "__name__", value: firstName}])
      }
      return {...state,
        names: action.payload.names,
        areNamesLoading: false,
        labels: labels,
      }
    default:
    return state;
  }
}

function decodeTimelineData(timelineData){
  if(!timelineData) {
    return [];
  }
  const res = [];
  let time = timelineData.startTime;
  return timelineData.samples.map((x) => {
    const res = [time*1000, x];
    time += timelineData.durationDelta;
    return res;
  });
}
