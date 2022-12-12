import { useMemo } from 'react';
import { Maybe } from 'true-myth';

import type { App } from '@webapp/models/app';
import useFiltersValues from './useFiltersValues';

const useFilters = (apps: App[]) => {
  const { filters, setFilters, spyNameValues, profileTypeValues } =
    useFiltersValues(apps);

  const handleFilterChange = (
    k: 'search' | 'spyName' | 'profileType',
    v: string
  ) => {
    setFilters((prevFilters) => {
      const prevFilterValue = prevFilters[k];

      if (prevFilterValue.isJust && prevFilterValue.value === v) {
        return { ...prevFilters, [k]: Maybe.nothing() };
      }

      return { ...prevFilters, [k]: Maybe.just(v) };
    });
  };

  const resetClickableFilters = () => {
    setFilters((v) => ({
      ...v,
      spyName: Maybe.nothing(),
      profileType: Maybe.nothing(),
    }));
  };

  const filteredApps = useMemo(
    () =>
      apps.filter((n) => {
        const { search, spyName, profileType } = filters;
        let matchFilters = true;

        if (search.isJust && matchFilters) {
          matchFilters = n.name
            .toLowerCase()
            .includes(search.value.trim().toLowerCase());
        }

        if (spyName.isJust && matchFilters) {
          matchFilters = n.spyName === spyName.value;
        }

        if (profileType.isJust && matchFilters) {
          matchFilters = n.name.includes(profileType.value);
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
