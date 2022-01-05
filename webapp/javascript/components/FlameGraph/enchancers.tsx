import { connect } from 'react-redux';
import { RootState } from '@pyroscope/redux/store';
import { selectUIState } from '@pyroscope/redux/reducers/views';

import { setUIValue } from '@pyroscope/redux/actions';

type IPersistedValueSetters<Type> = {
  [Property in keyof Type as `set${Capitalize<string & Property>}`]: (
    v
  ) => void;
};

export type IPersistedValue<T> = IPersistedValueSetters<T> & T;

interface ICurrentUIView {
  view: string;
}

const capitalize = (s) => s.charAt(0).toUpperCase() + s.slice(1);

const withUpdateableUIValue = (valueName, savePath) =>
  connect(
    (state: RootState) => ({
      [valueName]: selectUIState(state)(savePath),
    }),
    (dispatch) => ({
      [`set${capitalize(valueName)}`]: (value) => {
        dispatch(setUIValue(savePath, value));
      },
    })
  );

export const withUpdateableView = withUpdateableUIValue(
  'view',
  'flamegraphView'
);
export const withNamedUpdateableView = (name) =>
  withUpdateableUIValue(`view`, `flamegraphView.${name}`);
