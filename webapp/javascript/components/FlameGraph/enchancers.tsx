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

const withUpdateableUIValues = (valuesMap) =>
  connect(
    (state: RootState) =>
      valuesMap.reduce(
        (s, i) => ({ ...s, [i.value]: selectUIState(state)(i.path) }),
        {}
      ),
    (dispatch) =>
      valuesMap.reduce(
        (s, i) => ({
          ...s,
          [`set${capitalize(i.value)}`]: (value) =>
            dispatch(setUIValue(i.path, value)),
        }),
        {}
      )
  );

export const withNamedUpdateableView = (name) =>
  withUpdateableUIValues([
    { value: `view`, path: `flamegraphView.${name}.view` },
    { value: `sortBy`, path: `flamegraphView.${name}.sortBy` },
    {
      value: `sortByDirection`,
      path: `flamegraphView.${name}.sortByDirection`,
    },
    { value: 'fitMode', path: `flamegraphView.${name}.fitMode` },
  ]);
