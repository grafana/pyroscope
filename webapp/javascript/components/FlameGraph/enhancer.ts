import { connect } from 'react-redux';
import { RootState } from '@pyroscope/redux/store';
import { selectUIState } from '@pyroscope/redux/reducers/views';

import { setUIValue } from '@pyroscope/redux/actions';

type PersistedValueSetters<Type> = {
  [Property in keyof Type as `set${Capitalize<string & Property>}`]: (
    v
  ) => void;
};

export type IPersistedValue<T> = PersistedValueSetters<T> & T;

const capitalize = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

type UIValue = string | object;

const withUpdateableUIValues = (
  valuesMap: { value: string; path: string; default: UIValue }[]
) =>
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
          [`set${capitalize(i.value)}`]: (value: UIValue) =>
            dispatch(setUIValue(i.path, value)),
        }),
        {}
      )
  );

export const withNamedUpdateableView = (name: string) =>
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
