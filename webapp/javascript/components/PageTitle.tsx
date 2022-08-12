import React from 'react';
import { Helmet } from 'react-helmet';

import { Query } from '@webapp/models/query';
import { Maybe } from '@webapp/util/fp';

// usePageTitle generates a page title given a page name and the queries used
function usePageTitle({
  name,
  leftQuery,
  rightQuery,
}: {
  name: string;
  leftQuery?: string;
  rightQuery?: string;
}): string {
  const separator = ' | ';

  const leftOpt = Maybe.of(leftQuery);
  const rightOpt = Maybe.of(rightQuery);

  const formatQueries = leftOpt.match({
    Just: (left) => {
      return rightOpt.match({
        // Both are set
        Just: (right) => {
          if (left === right) {
            return Maybe.just(left);
          }

          return Maybe.just(`${left} and ${right}`);
        },
        // Just left is set
        Nothing: () => {
          return Maybe.just(left);
        },
      });
    },
    Nothing: () => {
      return rightOpt.match({
        // Just right is set
        Just: (right) => {
          return Maybe.just(right);
        },
        // None are set
        Nothing: () => {
          return Maybe.nothing<Query>();
        },
      });
    },
  });

  return formatQueries.map((q) => [name, q].join(separator)).unwrapOr(name);
}

export default function PageTitle(
  props: Parameters<typeof usePageTitle>[number]
) {
  const pageTitle = usePageTitle(props);

  return (
    <Helmet>
      <title>{pageTitle}</title>
    </Helmet>
  );
}
