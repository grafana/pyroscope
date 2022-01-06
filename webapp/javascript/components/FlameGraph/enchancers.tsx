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

const withUpdateableUIValue = (valueName, savePath, defaultValue = undefined) =>
  connect(
    (state: RootState) => ({
      [valueName]: selectUIState(state)(savePath, defaultValue),
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
        (s, i) => ({
          ...s,
          [i.value]: selectUIState(state)(i.path, i.default),
        }),
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
    { value: `view`, path: `flamegraphView.${name}.view`, default: 'both' },
    {
      value: `sortBy`,
      path: `flamegraphView.${name}.sortBy`,
      default: 'total',
    },
    {
      value: `sortByDirection`,
      path: `flamegraphView.${name}.sortByDirection`,
      default: 'asc',
    },
    {
      value: 'fitMode',
      path: `flamegraphView.${name}.fitMode`,
      default: 'HEAD',
    },
  ]);
