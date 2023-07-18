import React, { useContext } from 'react';
import { Helmet } from 'react-helmet';

const defaultAppName = 'Pyroscope';
export const AppNameContext = React.createContext(defaultAppName);

function getFullTitle(title: string, appName: string) {
  const finalAppName = appName || defaultAppName;

  return `${title} | ${finalAppName}`;
}

interface PageTitleProps {
  /** Title of the page */
  title: string;
}

/**
 * PageTitleWithAppName will add a page name suffix from the context
 */
export default function PageTitleWithAppName({ title }: PageTitleProps) {
  const appName = useContext(AppNameContext);
  const fullTitle = getFullTitle(title, appName);

  return (
    <Helmet>
      <title>{fullTitle}</title>
    </Helmet>
  );
}
