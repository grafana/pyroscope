import {
  SET_DATE_RANGE,
  REFRESH,
  ADD_LABEL,
  REMOVE_LABEL,
  REQUEST_SVG,
  RECEIVE_SVG,
  REQUEST_NAMES,
  RECEIVE_NAMES,
} from "./actionTypes";

export const setDateRange = (from, until) => {
  return { type: SET_DATE_RANGE, payload: { from, until } }
};
export const refresh = () => {
  return { type: REFRESH, payload: { } }
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

export const requestNames = () => {
  return { type: REQUEST_NAMES, payload: {} }
};
const receiveNames = (names) => {
  return { type: RECEIVE_NAMES, payload: { names } }
};


let currentSVGController = null;
export function fetchSVG(url) {
  return dispatch => {
    if (currentSVGController) {
      currentSVGController.abort();
    }
    currentSVGController = new AbortController();
    dispatch(requestSVG(url))
    return fetch(url, {signal: currentSVGController.signal})
      .then(response => response.text())
      .then(data => dispatch(receiveSVG(data)))
      .finally()
  }
}

let currentNamesController = null;
export function fetchNames() {
  return dispatch => {
    if (currentNamesController) {
      currentNamesController.abort();
    }
    currentNamesController = new AbortController();
    dispatch(requestNames())
    return fetch("/label-values?label=__name__", {signal: currentNamesController.signal})
      .then(response => response.json())
      .then(data => dispatch(receiveNames(data)))
      .finally()
  }
}
