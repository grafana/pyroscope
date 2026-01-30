import React from 'react';

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
  return (
    <div className="header row border-bottom py-3 flex-column-reverse flex-sm-row">
      <div className="col-12 col-sm-9 text-center text-sm-start">
        <h1>{title}</h1>
        <p className="text-muted">
          {subtitle}
          {showNewQueryLink && (
            <>
              <span className="ms-2">|</span>
              <a href="/query-diagnostics" className="ms-2">
                <i className="bi bi-plus-circle"></i> New Query
              </a>
            </>
          )}
          {showStoredDiagnosticsLink && (
            <>
              <span className="ms-2">|</span>
              <a href="/query-diagnostics/list" className="ms-2">
                <i className="bi bi-clock-history"></i> View Stored Diagnostics
              </a>
            </>
          )}
        </p>
      </div>
      <div className="col-12 col-sm-3 text-center text-sm-end mb-3 mb-sm-0">
        <a href="/">
          <img
            alt="Pyroscope logo"
            className="pyroscope-brand"
            src="/static/pyroscope-logo.png"
          />
        </a>
      </div>
    </div>
  );
}
