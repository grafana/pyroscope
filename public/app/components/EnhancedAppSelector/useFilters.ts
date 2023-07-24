import { useMemo } from 'react';
import { Maybe } from 'true-myth';

import type { App } from '@phlare/models/app';
import type {
  SpyName,
  SpyNameFirstClassType,
} from '@pyroscope/models/src/spyName';
import useFiltersValues from './useFiltersValues';

const useFilters = (apps: App[]) => {
  const { filters, setFilters, spyNameValues, profileTypeValues } =
    useFiltersValues(apps);

  const handleFilterChange = (
    k: 'search' | 'spyNames' | 'profileTypes',
    v: SpyName | string
  ) => {
    setFilters((prevFilters) => {
      if (k === 'search') {
        return { ...prevFilters, [k]: Maybe.just(v) };
      }

      const prevFilterValue: Maybe<Array<SpyName | string>> = prevFilters[k];

      if (prevFilterValue.isJust && prevFilterValue.value.length > 0) {
        const { newValue, shouldAddValue } = prevFilterValue.value.reduce(
          (acc, prevV) => {
            if (v === prevV) {
              acc.shouldAddValue = false;
              return acc;
            }

            acc.newValue.push(prevV);
            return acc;
          },
          { newValue: [] as Array<SpyName | string>, shouldAddValue: true }
        );

        const existingValue = shouldAddValue ? [...newValue, v] : newValue;

        return {
          ...prevFilters,
          [k]:
            existingValue.length > 0
              ? Maybe.just(existingValue)
              : Maybe.nothing(),
        };
      }

      return { ...prevFilters, [k]: Maybe.just([v]) };
    });
  };

  const resetClickableFilters = () => {
    setFilters((v) => ({
      ...v,
      spyNames: Maybe.nothing(),
      profileTypes: Maybe.nothing(),
    }));
  };

  const filteredApps = useMemo(
    () =>
      apps.filter((n) => {
        const { search, spyNames, profileTypes } = filters;
        let matchFilters = true;

        if (search.isJust && matchFilters) {
          matchFilters = n.name
            .toLowerCase()
            .includes(search.value.trim().toLowerCase());
        }
        if (spyNames.isJust && matchFilters) {
          matchFilters =
            spyNames.value.indexOf(n.spyName as SpyNameFirstClassType) !== -1;
        }

        if (profileTypes.isJust && matchFilters) {
          for (let i = 0; i < profileTypes.value.length; i += 1) {
            matchFilters = !!n.name.includes(profileTypes.value[i]);

            if (matchFilters) {
              return matchFilters;
            }
          }
        }

        return matchFilters;
      }),
    [filters, apps]
  );

  return {
    filters,
    handleFilterChange,
    filteredAppNames: filteredApps.map((v) => v.name),
    spyNameValues,
    profileTypeValues,
    resetClickableFilters,
  };
};

export default useFilters;
