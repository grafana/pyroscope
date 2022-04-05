import React from 'react';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faDownload } from '@fortawesome/free-solid-svg-icons/faDownload';
// eslint-disable-next-line import/no-relative-packages
import { version } from '../../../package.json';

const START_YEAR = '2020';
const PYROSCOPE_VERSION = version;

function copyrightYears(start: string, end: string) {
  return start === end ? start : `${start} – ${end}`;
}

const win = window as ShamefulAny;

function buildInfo() {
  return `
    BUILD INFO:
    js_version: v${PYROSCOPE_VERSION}
    goos: ${win.buildInfo.goos}
    goarch: ${win.buildInfo.goarch}
    go_version: ${win.buildInfo.goVersion}
    version: ${win.buildInfo.version}
    id: ${win.buildInfo.id}
    time: ${win.buildInfo.time}
    gitSHA: ${win.buildInfo.gitSHA}
    gitDirty: ${win.buildInfo.gitDirty}
    embeddedAssets: ${win.buildInfo.useEmbeddedAssets}
`.replace(/^\s+/gm, '');
}

function Footer() {
  const latestVersion = win.latestVersionInfo.latest_version;
  const newVersionAvailable =
    latestVersion && win.buildInfo.version !== latestVersion;

  return (
    <div className="footer" title={buildInfo()}>
      <span>
        {`© Pyroscope ${copyrightYears(
          START_YEAR,
          new Date().getFullYear().toFixed()
        )}`}
      </span>
      &nbsp;&nbsp;|&nbsp;&nbsp;
      <span>{win.buildInfo.version}</span>
      {newVersionAvailable && (
        <span>
          &nbsp;&nbsp;|&nbsp;&nbsp;
          <a
            href="https://pyroscope.io/downloads?utm_source=pyroscope_footer"
            rel="noreferrer"
            target="_blank"
          >
            <FontAwesomeIcon icon={faDownload} />
            &nbsp;
            <span>{`Newer Version Available (${latestVersion})`}</span>
          </a>
        </span>
      )}
    </div>
  );
}

export default Footer;
