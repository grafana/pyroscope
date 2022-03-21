// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-nocheck
import {
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
  response: ShamefulAny;

  constructor(response: ShamefulAny, text: string) {
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
      .catch((e) => {
        handleError(dispatch, e);
        dispatch(cancelAdhocProfiles());
      })
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
      .catch((e) => {
        handleError(dispatch, e);
        dispatch(cancelAdhocProfile());
      })
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
      .catch((e) => {
        handleError(dispatch, e);
        dispatch(cancelAdhocLeftProfile());
      })
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
      .catch((e) => {
        handleError(dispatch, e);
        dispatch(cancelAdhocRightProfile());
      })
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
      .catch((e) => {
        handleError(dispatch, e);
        dispatch(cancelAdhocProfileDiff());
      })
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
