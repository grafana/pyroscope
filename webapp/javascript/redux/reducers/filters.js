import {
  SET_DATE_RANGE,
  ADD_LABEL,
  REMOVE_LABEL,
  RECEIVE_DATA,
  REQUEST_DATA,
} from "../actionTypes";

import uniqBy from "lodash/fp/uniqBy";

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
    case ADD_LABEL:
      return {...state, labels: uniqBy("name", [action.payload].concat(state.labels)) }
    case REMOVE_LABEL:
      return {...state,
        labels: state.labels.filter((x) => x.name !== action.payload.name)
      }
    case REQUEST_DATA:
      return {...state,
        isDataLoading: true,
      }
    case RECEIVE_DATA:
      // console.log("RECEIVE_DATA", action)
      // let [samples, svg] = action.payload.data.split("\n", 2)
      let i = action.payload.data.indexOf("\n");
      return {...state,
        samples: JSON.parse(action.payload.data.substring(0, i)),
        svg: action.payload.data.substring(i+1),
        isDataLoading: false,
      }
    default:
    return state;
  }
}
