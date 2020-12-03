import {
  SET_DATE_RANGE,
  ADD_LABEL,
  REMOVE_LABEL,
  REQUEST_DATA,
  RECEIVE_DATA,
} from "./actionTypes";

export const setDateRange = (from, until) => {
  return { type: SET_DATE_RANGE, payload: { from, until } }
};
export const addLabel = (name, value) => {
  return { type: ADD_LABEL, payload: { name, value } }
};
export const removeLabel = (name) => {
  return { type: REMOVE_LABEL, payload: { name } }
};
export const requestData = (url) => {
  return { type: REQUEST_DATA, payload: { url } }
};
const receiveData = (data) => {
  return { type: RECEIVE_DATA, payload: { data } }
};


let currentController = null;
export function fetchData(url) {
  return dispatch => {
    if (currentController) {
      currentController.abort();
    }
    currentController = new AbortController();
    dispatch(requestData(url))
    return fetch(url, {signal: currentController.signal})
      .then(response => response.text())
      .then(data => dispatch(receiveData(data)))
      .finally()
  }
}
