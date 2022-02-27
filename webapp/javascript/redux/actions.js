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
  REQUEST_NAMES,
  RECEIVE_NAMES,
  SET_QUERY,
  SET_LEFT_DATE_RANGE,
  SET_RIGHT_DATE_RANGE,
  SET_LEFT_FROM,
  SET_LEFT_UNTIL,
  SET_RIGHT_FROM,
  SET_RIGHT_UNTIL,
  RECEIVE_COMPARISON_APP_DATA,
  REQUEST_COMPARISON_APP_DATA,
  CANCEL_COMPARISON_APP_DATA,
  REQUEST_PYROSCOPE_APP_DATA,
  RECEIVE_PYROSCOPE_APP_DATA,
  CANCEL_PYROSCOPE_APP_DATA,
  REQUEST_COMPARISON_DIFF_APP_DATA,
  RECEIVE_COMPARISON_DIFF_APP_DATA,
  CANCEL_COMPARISON_DIFF_APP_DATA,
  REQUEST_COMPARISON_TIMELINE,
  RECEIVE_COMPARISON_TIMELINE,
  SET_ADHOC_FILE,
  SET_ADHOC_LEFT_FILE,
  SET_ADHOC_RIGHT_FILE,
  REQUEST_ADHOC_PROFILES,
  RECEIVE_ADHOC_PROFILES,
  CANCEL_ADHOC_PROFILES,
  SET_ADHOC_PROFILE,
  REQUEST_ADHOC_PROFILE,
  RECEIVE_ADHOC_PROFILE,
  CANCEL_ADHOC_PROFILE,
  SET_ADHOC_LEFT_PROFILE,
  SET_ADHOC_RIGHT_PROFILE,
  REQUEST_ADHOC_LEFT_PROFILE,
  REQUEST_ADHOC_RIGHT_PROFILE,
  RECEIVE_ADHOC_LEFT_PROFILE,
  RECEIVE_ADHOC_RIGHT_PROFILE,
  CANCEL_ADHOC_LEFT_PROFILE,
  CANCEL_ADHOC_RIGHT_PROFILE,
  REQUEST_ADHOC_PROFILE_DIFF,
  RECEIVE_ADHOC_PROFILE_DIFF,
  CANCEL_ADHOC_PROFILE_DIFF,
} from './actionTypes';
import { isAbortError } from '../util/abort';
import { addNotification } from './reducers/notifications';

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
  type: REQUEST_COMPARISON_TIMELINE,
  payload: { url },
});

export const receiveTimeline = (data) => ({
  type: RECEIVE_COMPARISON_TIMELINE,
  payload: data,
});

export const requestPyroscopeAppData = (url) => ({
  type: REQUEST_PYROSCOPE_APP_DATA,
  payload: { url },
});

export const receivePyroscopeAppData = (data) => ({
  type: RECEIVE_PYROSCOPE_APP_DATA,
  payload: { data },
});
export const cancelPyroscopeAppData = () => ({
  type: CANCEL_PYROSCOPE_APP_DATA,
});

export const requestComparisonAppData = (url, viewSide) => ({
  type: REQUEST_COMPARISON_APP_DATA,
  payload: { url, viewSide },
});

export const receiveComparisonAppData = (data, viewSide) => ({
  type: RECEIVE_COMPARISON_APP_DATA,
  payload: { data, viewSide },
});

export const cancelComparisonappData = () => ({
  type: CANCEL_COMPARISON_APP_DATA,
});

export const requestComparisonDiffAppData = (url) => ({
  type: REQUEST_COMPARISON_DIFF_APP_DATA,
  payload: { url },
});

export const receiveComparisonDiffAppData = (data) => ({
  type: RECEIVE_COMPARISON_DIFF_APP_DATA,
  payload: { data },
});

export const cancelComparisonDiffAppData = () => ({
  type: CANCEL_COMPARISON_DIFF_APP_DATA,
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

export const setAdhocFile = (file, flamebearer) => ({
  type: SET_ADHOC_FILE,
  payload: { file, flamebearer },
});

export const setAdhocLeftFile = (file, flamebearer) => ({
  type: SET_ADHOC_LEFT_FILE,
  payload: { file, flamebearer },
});

export const setAdhocRightFile = (file, flamebearer) => ({
  type: SET_ADHOC_RIGHT_FILE,
  payload: { file, flamebearer },
});

export const requestAdhocProfiles = () => ({ type: REQUEST_ADHOC_PROFILES });

export const receiveAdhocProfiles = (profiles) => ({
  type: RECEIVE_ADHOC_PROFILES,
  payload: { profiles },
});

export const cancelAdhocProfiles = () => ({ type: CANCEL_ADHOC_PROFILES });

export const setAdhocProfile = (profile) => ({
  type: SET_ADHOC_PROFILE,
  payload: { profile },
});

export const requestAdhocProfile = () => ({ type: REQUEST_ADHOC_PROFILE });

export const receiveAdhocProfile = (flamebearer) => ({
  type: RECEIVE_ADHOC_PROFILE,
  payload: { flamebearer },
});

export const cancelAdhocProfile = () => ({ type: CANCEL_ADHOC_PROFILE });

export const setAdhocLeftProfile = (profile) => ({
  type: SET_ADHOC_LEFT_PROFILE,
  payload: { profile },
});

export const requestAdhocLeftProfile = () => ({
  type: REQUEST_ADHOC_LEFT_PROFILE,
});

export const receiveAdhocLeftProfile = (flamebearer) => ({
  type: RECEIVE_ADHOC_LEFT_PROFILE,
  payload: { flamebearer },
});

export const cancelAdhocLeftProfile = () => ({
  type: CANCEL_ADHOC_LEFT_PROFILE,
});

export const setAdhocRightProfile = (profile) => ({
  type: SET_ADHOC_RIGHT_PROFILE,
  payload: { profile },
});

export const requestAdhocRightProfile = () => ({
  type: REQUEST_ADHOC_RIGHT_PROFILE,
});

export const receiveAdhocRightProfile = (flamebearer) => ({
  type: RECEIVE_ADHOC_RIGHT_PROFILE,
  payload: { flamebearer },
});

export const cancelAdhocRightProfile = () => ({
  type: CANCEL_ADHOC_RIGHT_PROFILE,
});

export const requestAdhocProfileDiff = () => ({
  type: REQUEST_ADHOC_PROFILE_DIFF,
});

export const receiveAdhocProfileDiff = (flamebearer) => ({
  type: RECEIVE_ADHOC_PROFILE_DIFF,
  payload: { flamebearer },
});

export const cancelAdhocProfileDiff = () => ({
  type: CANCEL_ADHOC_PROFILE_DIFF,
});

// ResponseNotOkError refers to when request is not ok
// ie when status code is not in the 2xx range
class ResponseNotOkError extends Error {
  constructor(response, text) {
    super(`Bad Response with code ${response.status}: ${text}`);
    this.name = 'ResponseNotOkError';
    this.response = response;
  }
}

// dispatchNotificationByError dispatches a notification
// depending on the error passed
function handleError(dispatch, e) {
  if (e instanceof ResponseNotOkError) {
    dispatch(
      addNotification({
        title: 'Request Failed',
        message: e.message,
        type: 'danger',
      })
    );
  } else if (!isAbortError(e)) {
    // AbortErrors are fine

    // Generic case, so we use as message whatever error we got
    // It's not the best UX, but our users should be experienced enough
    // to be able to decipher what's going on based on the message
    dispatch(
      addNotification({
        title: 'Error',
        message: e.message,
        type: 'danger',
      })
    );
  }
}

// handleResponse retrieves the JSON data on success or raises an ResponseNotOKError otherwise
function handleResponse(dispatch, response) {
  if (response.ok) {
    return response.json();
  }
  return response.text().then((text) => {
    throw new ResponseNotOkError(response, text);
  });
}

/**
 * ATTENTION! There may be race conditions:
 * Since a new controller is created every time a 'fetch' action is called
 * A badly timed 'abort' action may cancel the brand new 'fetch' action!
 */
let currentTimelineController;
const currentComparisonTimelineController = {
  left: null,
  right: null,
};
let fetchTagController;
let fetchTagValuesController;

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
      .then((response) => handleResponse(dispatch, response))
      .then((data) => dispatch(receiveTimeline(data)))
      .catch((e) => handleError(dispatch, e))
      .finally();
  };
}

export function abortTimelineRequest() {
  return () => {
    if (currentTimelineController) {
      currentTimelineController.abort();
    }
  };
}

export function fetchComparisonAppData(url, viewSide) {
  return (dispatch) => {
    const getTimelineController = () => {
      switch (viewSide) {
        case 'left':
          return currentComparisonTimelineController.left;
        case 'right':
          return currentComparisonTimelineController.right;
        default:
          throw new Error(`Invalid viewSide: '${viewSide}'`);
      }
    };
    let timelineController = getTimelineController();
    if (timelineController) {
      timelineController.abort();
    }

    switch (viewSide) {
      case 'left':
        currentComparisonTimelineController.left = new AbortController();
        break;
      case 'right':
        currentComparisonTimelineController.right = new AbortController();
        break;
      default:
        throw new Error(`Invalid viewSide: '${viewSide}'`);
    }
    dispatch(requestComparisonAppData(url, viewSide));
    timelineController = getTimelineController();
    return fetch(`${url}&format=json`, {
      signal: timelineController.signal,
    })
      .then((response) => handleResponse(dispatch, response))
      .then((data) => dispatch(receiveComparisonAppData(data, viewSide)))
      .catch((e) => handleError(dispatch, e))
      .then(() => dispatch(cancelComparisonappData()))
      .finally();
  };
}

export function fetchPyroscopeAppData(url) {
  return (dispatch) => {
    if (currentTimelineController) {
      currentTimelineController.abort();
    }
    currentTimelineController = new AbortController();
    dispatch(requestPyroscopeAppData(url));
    return fetch(`${url}&format=json`, {
      signal: currentTimelineController.signal,
    })
      .then((response) => handleResponse(dispatch, response))
      .then((data) => dispatch(receivePyroscopeAppData(data)))
      .catch((e) => handleError(dispatch, e))
      .then(() => dispatch(cancelPyroscopeAppData()))
      .finally();
  };
}

export function fetchComparisonDiffAppData(url) {
  return (dispatch) => {
    if (currentTimelineController) {
      currentTimelineController.abort();
    }
    currentTimelineController = new AbortController();
    dispatch(requestComparisonDiffAppData(url));
    return fetch(`${url}&format=json`, {
      signal: currentTimelineController.signal,
    })
      .then((response) => handleResponse(dispatch, response))
      .then((data) => dispatch(receiveComparisonDiffAppData(data)))
      .catch((e) => handleError(dispatch, e))
      .catch((e) => dispatchNotificationByError(dispatch, e))
      .then(() => dispatch(cancelComparisonDiffAppData()))
      .finally();
  };
}

export function fetchTags(query) {
  return (dispatch) => {
    if (fetchTagController) {
      fetchTagController.abort();
    }
    fetchTagController = new AbortController();

    dispatch(requestTags());
    return fetch(`./labels?query=${encodeURIComponent(query)}`)
      .then((response) => handleResponse(dispatch, response))
      .then((data) => dispatch(receiveTags(data)))
      .catch((e) => handleError(dispatch, e))
      .finally();
  };
}

export function abortFetchTags() {
  return () => {
    if (fetchTagController) {
      fetchTagController.abort();
    }
  };
}

export function fetchTagValues(query, tag) {
  return (dispatch) => {
    if (fetchTagValuesController) {
      fetchTagValuesController.abort();
    }
    fetchTagValuesController = new AbortController();

    dispatch(requestTagValues(tag));
    return fetch(
      `./label-values?label=${encodeURIComponent(
        tag
      )}&query=${encodeURIComponent(query)}`
    )
      .then((response) => handleResponse(dispatch, response))
      .then((data) => dispatch(receiveTagValues(data, tag)))
      .catch((e) => handleError(dispatch, e))
      .finally();
  };
}
export function abortFetchTagValues() {
  return () => {
    if (fetchTagValuesController) {
      fetchTagValuesController.abort();
    }
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
    return fetch('./label-values?label=__name__', {
      signal: currentNamesController.signal,
    })
      .then((response) => handleResponse(dispatch, response))
      .then((data) => dispatch(receiveNames(data)))
      .catch((e) => handleError(dispatch, e))
      .finally();
  };
}

let adhocProfilesController;
export function fetchAdhocProfiles() {
  return (dispatch) => {
    if (adhocProfilesController) {
      adhocProfilesController.abort();
    }

    adhocProfilesController = new AbortController();
    dispatch(requestAdhocProfiles());
    return fetch('./api/adhoc/v1/profiles', {
      signal: adhocProfilesController.signal,
    })
      .then((response) => handleResponse(dispatch, response))
      .then((data) => dispatch(receiveAdhocProfiles(data)))
      .catch((e) => handleError(dispatch, e))
      .then(() => dispatch(cancelAdhocProfiles()))
      .finally();
  };
}
export function abortFetchAdhocProfiles() {
  return () => {
    if (adhocProfilesController) {
      adhocProfilesController.abort();
    }
  };
}

let adhocProfileController;
export function fetchAdhocProfile(profile) {
  return (dispatch) => {
    if (adhocProfileController) {
      adhocProfileController.abort();
    }

    adhocProfileController = new AbortController();
    dispatch(requestAdhocProfile());
    return fetch(`./api/adhoc/v1/profile/${profile}`, {
      signal: adhocProfileController.signal,
    })
      .then((response) => handleResponse(dispatch, response))
      .then((data) => dispatch(receiveAdhocProfile(data)))
      .catch((e) => handleError(dispatch, e))
      .then(() => dispatch(cancelAdhocProfile()))
      .finally();
  };
}
export function abortFetchAdhocProfile() {
  return () => {
    if (adhocProfileController) {
      adhocProfileController.abort();
    }
  };
}

let adhocLeftProfileController;
export function fetchAdhocLeftProfile(profile) {
  return (dispatch) => {
    if (adhocLeftProfileController) {
      adhocLeftProfileController.abort();
    }

    adhocLeftProfileController = new AbortController();
    dispatch(requestAdhocLeftProfile());
    return fetch(`./api/adhoc/v1/profile/${profile}`, {
      signal: adhocLeftProfileController.signal,
    })
      .then((response) => handleResponse(dispatch, response))
      .then((data) => dispatch(receiveAdhocLeftProfile(data)))
      .catch((e) => handleError(dispatch, e))
      .then(() => dispatch(cancelAdhocLeftProfile()))
      .finally();
  };
}
export function abortFetchAdhocLeftProfile() {
  return () => {
    if (adhocLeftProfileController) {
      adhocLeftProfileController.abort();
    }
  };
}

let adhocRightProfileController;
export function fetchAdhocRightProfile(profile) {
  return (dispatch) => {
    if (adhocRightProfileController) {
      adhocRightProfileController.abort();
    }

    adhocRightProfileController = new AbortController();
    dispatch(requestAdhocRightProfile());
    return fetch(`./api/adhoc/v1/profile/${profile}`, {
      signal: adhocRightProfileController.signal,
    })
      .then((response) => handleResponse(dispatch, response))
      .then((data) => dispatch(receiveAdhocRightProfile(data)))
      .catch((e) => handleError(dispatch, e))
      .then(() => dispatch(cancelAdhocRightProfile()))
      .finally();
  };
}
export function abortFetchAdhocRightProfile() {
  return () => {
    if (adhocRightProfileController) {
      adhocRightProfileController.abort();
    }
  };
}

let adhocProfileDiffController;
export function fetchAdhocProfileDiff(left, right) {
  return (dispatch) => {
    if (adhocProfileDiffController) {
      adhocProfileDiffController.abort();
    }

    adhocProfileDiffController = new AbortController();
    dispatch(requestAdhocProfileDiff());
    return fetch(`./api/adhoc/v1/diff/${left}/${right}`, {
      signal: adhocProfileDiffController.signal,
    })
      .then((response) => handleResponse(dispatch, response))
      .then((data) => dispatch(receiveAdhocProfileDiff(data)))
      .catch((e) => handleError(dispatch, e))
      .then(() => dispatch(cancelAdhocProfileDiff()))
      .finally();
  };
}
export function abortFetchAdhocProfileDiff() {
  return () => {
    if (adhocProfileDiffController) {
      adhocProfileDiffController.abort();
    }
  };
}
