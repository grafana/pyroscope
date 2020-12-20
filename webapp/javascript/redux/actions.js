import {
  SET_DATE_RANGE,
  SET_FROM,
  SET_UNTIL,
  SET_MAX_NODES,
  SET_LABELS,
  REFRESH,
  ADD_LABEL,
  REMOVE_LABEL,
  REQUEST_SVG,
  RECEIVE_SVG,
  REQUEST_JSON,
  RECEIVE_JSON,
  REQUEST_NAMES,
  RECEIVE_NAMES,
} from "./actionTypes";

export const setDateRange = (from, until) => {
  return Promise.resolve({ 
    type: SET_DATE_RANGE, 
    payload: { from, until } 
  })
};

export const setFrom = (from) => {
  return { type: SET_FROM, payload: { from } }
};
export const setUntil = (until) => {
  return { type: SET_UNTIL, payload: { until } }
};
export const setMaxNodes = (maxNodes) => {
  return { type: SET_MAX_NODES, payload: { maxNodes } }
};
export const refresh = () => {
  return { type: REFRESH, payload: { } }
};
export const setLabels = (labels) => {
  return { type: SET_LABELS, payload: { labels } }
};
export const addLabel = (name, value) => {
  return { type: ADD_LABEL, payload: { name, value } }
};
export const removeLabel = (name) => {
  return { type: REMOVE_LABEL, payload: { name } }
};
export const requestSVG = (url) => {
  return { type: REQUEST_SVG, payload: { url } }
};
const receiveSVG = (data) => {
  return { type: RECEIVE_SVG, payload: { data } }
};
export const requestJSON = (url) => {
  return { type: REQUEST_JSON, payload: { url } }
};

export const receiveJSON = (data) => {
  return { type: RECEIVE_JSON, payload: data }
};

export const requestNames = () => {
  return { type: REQUEST_NAMES, payload: {} }
};
export const receiveNames = (names) => {
  return { type: RECEIVE_NAMES, payload: { names } }
};

export const requestJSON2 = (url) => ({
  type: REQUEST_JSON,
  payload: url,
})

let currentSVGController = null;
export function fetchSVG(url) {
  return dispatch => {
    if (currentSVGController) {
      currentSVGController.abort();
    }
    currentSVGController = new AbortController();
    dispatch(requestSVG(url));
    return fetch(url, {signal: currentSVGController.signal})
      .then(response => response.text())
      .then(data => dispatch(receiveSVG(data)))
      .finally()
  }
}

// let currentJSONController = null;
// export function fetchJSON(url) {
//   return dispatch => {
//     if (currentJSONController) {
//       currentJSONController.abort();
//     }
//     currentJSONController = new AbortController();
//     dispatch(requestJSON(url));
//     return fetch(url, {signal: currentJSONController.signal})
//       .then(response => response.json())
//       .then(data => dispatch(receiveJSON(data)))
//       .finally()
//   }
// }

// let currentNamesController = null;
// export function fetchNames() {
//   return dispatch => {
//     if (currentNamesController) {
//       currentNamesController.abort();
//     }
//     currentNamesController = new AbortController();
//     dispatch(requestNames());
//     return fetch("/label-values?label=__name__", {signal: currentNamesController.signal})
//       .then(response => response.json())
//       .then(data => dispatch(receiveNames(data)))
//       .finally()
//   }
// }


