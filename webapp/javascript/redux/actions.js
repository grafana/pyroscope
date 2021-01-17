import {
  SET_DATE_RANGE,
  SET_FROM,
  SET_UNTIL,
  SET_MAX_NODES,
  SET_LABELS,
  ADD_LABEL,
  REMOVE_LABEL,
  REFRESH,
  REQUEST_JSON,
  RECEIVE_JSON,
  REQUEST_NAMES,
  RECEIVE_NAMES,
} from "./actionTypes";

export const setDateRange = (from, until) => ({
  type: SET_DATE_RANGE,
  payload: { from, until },
});

export const setFrom = (from) => ({ type: SET_FROM, payload: { from } });

export const setUntil = (until) => ({ type: SET_UNTIL, payload: { until } });

export const setMaxNodes = (maxNodes) => ({
  type: SET_MAX_NODES,
  payload: { maxNodes },
});

export const setLabels = (labels) => ({
  type: SET_LABELS,
  payload: { labels },
});

export const addLabel = (name, value) => ({
  type: ADD_LABEL,
  payload: { name, value },
});

export const removeLabel = (name) => ({
  type: REMOVE_LABEL,
  payload: { name },
});

export const refresh = (url) => ({ type: REFRESH, payload: { url } });

export const requestJSON = (url) => ({ type: REQUEST_JSON, payload: { url } });

export const receiveJSON = (data) => ({ type: RECEIVE_JSON, payload: data });

export const requestNames = () => ({ type: REQUEST_NAMES, payload: {} });

export const receiveNames = (names) => ({
  type: RECEIVE_NAMES,
  payload: { names },
});

let currentJSONController;
export function fetchJSON(url) {
  return (dispatch) => {
    if (currentJSONController) {
      currentJSONController.abort();
    }
    currentJSONController = new AbortController();

    dispatch(requestJSON(url));
    return fetch(`${url}&format=json`, { signal: currentJSONController.signal })
      .then((response) => response.json())
      .then((data) => {
        dispatch(receiveJSON(data));
      })
      .finally();
  };
}

let currentNamesController;
export function fetchNames() {
  return (dispatch) => {
    if (currentNamesController) {
      currentNamesController.abort();
    }
    currentNamesController = new AbortController();

    dispatch(requestNames());
    return fetch("/label-values?label=__name__", {
      signal: currentNamesController.signal,
    })
      .then((response) => response.json())
      .then((data) => {
        dispatch(receiveNames(data));
      })
      .finally();
  };
}
