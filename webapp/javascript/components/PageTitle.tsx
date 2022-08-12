import React from 'react';
import { Helmet } from 'react-helmet';
import { useLocation } from 'react-router-dom';

import { Query } from '@webapp/models/query';
import { Maybe } from '@webapp/util/fp';
import { PAGES } from '@webapp/pages/constants';

type PagesUnion = `${PAGES}`;

function pageToName(c: PagesUnion): Maybe<string> {
  // eslint-disable-next-line default-case
  switch (c) {
    case PAGES.CONTINOUS_SINGLE_VIEW: {
      return Maybe.of('Single View');
    }
    case PAGES.COMPARISON_VIEW: {
      return Maybe.of('Comparison View');
    }
    case PAGES.COMPARISON_DIFF_VIEW: {
      return Maybe.of('Diff View');
    }
    case PAGES.SETTINGS: {
      return Maybe.of('Settings');
    }
    case PAGES.LOGIN: {
      return Maybe.of('Login');
    }
    case PAGES.SIGNUP: {
      return Maybe.of('Sign Up');
    }
    case PAGES.SERVICE_DISCOVERY: {
      return Maybe.of('Service Discovery');
    }
    case PAGES.ADHOC_SINGLE: {
      return Maybe.of('Adhoc Single');
    }
    case PAGES.ADHOC_COMPARISON: {
      return Maybe.of('Adhoc Comparison');
    }
    case PAGES.ADHOC_COMPARISON_DIFF: {
      return Maybe.of('Adhoc Diff');
    }
    case PAGES.FORBIDDEN: {
      return Maybe.of('Forbidden');
    }
    case PAGES.TAG_EXPLORER: {
      return Maybe.of('Tag Explorer');
    }
  }

  return Maybe.nothing();
}

// usePageTitle generates a page title given a page name and the queries used
function buildPageTitle({
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

export default function PageTitle() {
  //  props: Parameters<typeof usePageTitle>[number]
  const location = useLocation();

  // Are we on a page we have a title for?
  const isPreset = Maybe.of(
    Object.entries(PAGES).find((a) => location.pathname === a[1])
  );

  // Don't have any title
  if (isPreset.isNothing) {
    return <></>;
  }

  return isPreset
    .andThen((url) => pageToName(url[1]))
    .map((name) => {
      // TODO: use queries
      const pageTitle = buildPageTitle({ name });
      return (
        <Helmet>
          <title>{pageTitle}</title>
        </Helmet>
      );
    })
    .unwrapOr(<></>);
}
