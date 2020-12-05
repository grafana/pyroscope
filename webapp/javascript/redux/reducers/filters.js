import {
  SET_DATE_RANGE,
  REFRESH,
  ADD_LABEL,
  REMOVE_LABEL,
  RECEIVE_SVG,
  REQUEST_SVG,
  RECEIVE_NAMES,
  REQUEST_NAMES,
} from "../actionTypes";

import uniqBy from "lodash/fp/uniqBy";
import { random } from "core-js/fn/number";

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
    case REFRESH:
      return {
        ...state,
        refreshToken: Math.random(),
      }
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
      // console.log("RECEIVE_DATA", action)
      // let [samples, svg] = action.payload.data.split("\n", 2)
      let i = action.payload.data.indexOf("\n");
      return {...state,
        samples: JSON.parse(action.payload.data.substring(0, i)),
        svg: action.payload.data.substring(i+1),
        isSVGLoading: false,
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
