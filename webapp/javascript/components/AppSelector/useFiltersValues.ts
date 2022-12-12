import { useState } from 'react';
import { Maybe } from 'true-myth';

import type { App } from '@webapp/models/app';
import type { SpyNameFirstClassType } from '@pyroscope/models/src/spyName';

type FiltersType = {
  search: Maybe<string>;
  spyNames: Maybe<(SpyNameFirstClassType | 'unknown')[]>;
  profileTypes: Maybe<string[]>;
};

const useFiltersValues = (apps: App[]) => {
  const [filters, setFilters] = useState<FiltersType>({
    search: Maybe.nothing(),
    spyNames: Maybe.nothing(),
    profileTypes: Maybe.nothing(),
  });

  const { spyNameValues, profileTypeValues } = apps.reduce(
    (acc, v) => {
      // use as SpyNameFirstClassType because for now we support only first class types
      const appSpyName = v.spyName as SpyNameFirstClassType;
      if (acc.spyNameValues.indexOf(appSpyName) === -1) {
        acc.spyNameValues.push(appSpyName);
      }

      const propfileType = v.name.split('.').pop() as string;
      if (acc.profileTypeValues.indexOf(propfileType) === -1) {
        acc.profileTypeValues.push(propfileType);
      }

      return acc;
    },
    {
      spyNameValues: [] as SpyNameFirstClassType[],
      profileTypeValues: [] as string[],
    }
  );

  return {
    filters,
    setFilters,
    spyNameValues,
    profileTypeValues,
  };
};

export default useFiltersValues;
