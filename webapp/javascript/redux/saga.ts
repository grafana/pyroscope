import { takeEvery, put, all, select } from 'redux-saga/effects';
import { selectAppNames } from './reducers/newRoot';

import { setQuery } from './actions';

export function* queryWatcherSaga(action) {
  // Setting up default query
  if (action.payload && !action.payload.query && action.payload.query !== '') {
    // Select all appNames from store
    const appNames = yield select(selectAppNames);

    // It would take pyroscope.server.cpu if exists and first one otherwise.
    const defaultName =
      appNames.find((x) => x === 'pyroscope.server.cpu') ||
      (window as any).initialState.appNames.find(
        (x) => x !== 'pyroscope.server.cpu'
      );

    // Set back default app
    yield put(setQuery(`${defaultName}{}`));
  }
}

export function* mainSaga() {
  yield all([yield takeEvery('SET_QUERY', queryWatcherSaga)]);
}
