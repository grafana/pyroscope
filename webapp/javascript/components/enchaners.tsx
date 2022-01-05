import { connect } from 'react-redux';
import { RootState } from '@pyroscope/redux/store';
import { selectUIState } from '@pyroscope/redux/reducers/views';

import { setUIValue } from '../redux/actions';

export interface ICollapsible {
  collapsed: boolean;
  setCollapsed: (boolean) => void;
}

export const withCollapsible = (path) =>
  connect(
    (state: RootState) => ({
      collapsed: selectUIState(state)(path),
    }),
    (dispatch) => ({
      setCollapsed: (value) => {
        dispatch(setUIValue(path, value));
      },
    })
  );
