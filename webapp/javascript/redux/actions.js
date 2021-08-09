import {
  SET_DATE_RANGE,
  SET_FROM,
  SET_UNTIL,
  SET_MAX_NODES,
  REFRESH,
  REQUEST_TIMELINE,
  RECEIVE_TIMELINE,
  REQUEST_TAGS,
  RECEIVE_TAGS,
  REQUEST_TAG_VALUES,
  RECEIVE_TAG_VALUES,
  REQUEST_NAMES,
  RECEIVE_NAMES,
  SET_QUERY,
  SET_LEFT_DATE_RANGE,
  SET_RIGHT_DATE_RANGE,
  SET_LEFT_FROM,
  SET_LEFT_UNTIL,
  SET_RIGHT_FROM,
  SET_RIGHT_UNTIL,
} from "./actionTypes";

export const setDateRange = (from, until) => ({
  type: SET_DATE_RANGE,
  payload: { from, until },
});

export const setLeftDateRange = (from, until) => ({
  type: SET_LEFT_DATE_RANGE,
  payload: { from, until },
});

export const setRightDateRange = (from, until) => ({
  type: SET_RIGHT_DATE_RANGE,
  payload: { from, until },
});

export const setFrom = (from) => ({ type: SET_FROM, payload: { from } });

export const setLeftFrom = (from) => ({
  type: SET_LEFT_FROM,
  payload: { from },
});
export const setRightFrom = (from) => ({
  type: SET_RIGHT_FROM,
  payload: { from },
});

export const setUntil = (until) => ({ type: SET_UNTIL, payload: { until } });
export const setLeftUntil = (until) => ({
  type: SET_LEFT_UNTIL,
  payload: { until },
});
export const setRightUntil = (until) => ({
  type: SET_RIGHT_UNTIL,
  payload: { until },
});

export const setMaxNodes = (maxNodes) => ({
  type: SET_MAX_NODES,
  payload: { maxNodes },
});

export const refresh = (url) => ({ type: REFRESH, payload: { url } });

export const requestTimeline = (url) => ({
  type: REQUEST_TIMELINE,
  payload: { url },
});

export const receiveTimeline = (data) => ({
  type: RECEIVE_TIMELINE,
  payload: data,
});

export const requestTags = () => ({ type: REQUEST_TAGS });

export const receiveTags = (tags) => ({
  type: RECEIVE_TAGS,
  payload: { tags },
});

export const requestTagValues = (tag) => ({
  type: REQUEST_TAG_VALUES,
  payload: { tag },
});

export const receiveTagValues = (values, tag) => ({
  type: RECEIVE_TAG_VALUES,
  payload: { values, tag },
});

export const requestNames = () => ({ type: REQUEST_NAMES, payload: {} });

export const receiveNames = (names) => ({
  type: RECEIVE_NAMES,
  payload: { names },
});

export const setQuery = (query) => ({
  type: SET_QUERY,
  payload: { query },
});

let currentTimelineController;
export function fetchTimeline(url) {
  return (dispatch) => {
    if (currentTimelineController) {
      currentTimelineController.abort();
    }
    currentTimelineController = new AbortController();
    dispatch(requestTimeline(url));
    return fetch(`${url}&format=json`, {
      signal: currentTimelineController.signal,
    })
      .then((response) => response.json())
      .then((data) => {
        dispatch(receiveTimeline(data));
      })
      .finally();
  };
}

export function fetchTags(query) {
  return (dispatch) => {
    dispatch(requestTags());
    return fetch(`/labels?query=${encodeURIComponent(query)}`)
      .then((response) => response.json())
      .then((data) => {
        dispatch(receiveTags(data));
      })
      .finally();
  };
}

export function fetchTagValues(query, tag) {
  return (dispatch) => {
    dispatch(requestTagValues(tag));
    return fetch(
      `/label-values?label=${encodeURIComponent(
        tag
      )}&query=${encodeURIComponent(query)}`
    )
      .then((response) => response.json())
      .then((data) => {
        dispatch(receiveTagValues(data, tag));
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
