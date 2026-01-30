import React from 'react';

import { getBasePath } from '../utils';

interface HeaderProps {
  title: string;
  subtitle: string;
  showNewQueryLink?: boolean;
  showStoredDiagnosticsLink?: boolean;
}

export function Header({
  title,
  subtitle,
  showNewQueryLink = true,
  showStoredDiagnosticsLink = true,
}: HeaderProps) {
  const basePath = getBasePath();

  return (
    <div className="header row border-bottom py-3 flex-column-reverse flex-sm-row">
      <div className="col-12 col-sm-9 text-center text-sm-start">
        <h1>{title}</h1>
        <p className="text-muted">
          {subtitle}
          {showNewQueryLink && (
            <>
              <span className="ms-2">|</span>
              <a href={`${basePath}/query-diagnostics`} className="ms-2">
                + New Query
              </a>
            </>
          )}
          {showStoredDiagnosticsLink && (
            <>
              <span className="ms-2">|</span>
              <a href={`${basePath}/query-diagnostics/list`} className="ms-2">
                View Stored Diagnostics
              </a>
            </>
          )}
        </p>
      </div>
      <div className="col-12 col-sm-3 text-center text-sm-end mb-3 mb-sm-0">
        <a href={`${basePath}/`}>
          <img
            alt="Pyroscope logo"
            className="pyroscope-brand"
            src={`${basePath}/static/pyroscope-logo.png`}
          />
        </a>
      </div>
    </div>
  );
}
