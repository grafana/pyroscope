import React from 'react';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faDownload } from '@fortawesome/free-solid-svg-icons/faDownload';
import { Maybe } from 'true-myth';
import { BuildInfo, buildInfo, latestVersionInfo } from '../util/buildInfo';

const START_YEAR = '2020';

function copyrightYears(start: string, end: string) {
  return start === end ? start : `${start} – ${end}`;
}

function buildInfoStr(buildInfo: BuildInfo) {
  return `
    BUILD INFO:
    js_version: ${buildInfo.jsVersion}
    goos: ${buildInfo?.goos}
    goarch: ${buildInfo?.goarch}
    go_version: ${buildInfo?.goVersion}
    version: ${buildInfo?.version}
    time: ${buildInfo?.time}
    gitSHA: ${buildInfo?.gitSHA}
    gitDirty: ${buildInfo?.gitDirty}
    embeddedAssets: ${buildInfo?.useEmbeddedAssets}
`.replace(/^\s+/gm, '');
}

function NewerVersionCheck() {
  const { version } = buildInfo();
  const maybeLatestVersionInfo = latestVersionInfo();

  if (maybeLatestVersionInfo.isNothing) {
    return null;
  }

  const latestVersion = maybeLatestVersionInfo.value.latest_version;

  interface Version {
    major: number;
    minor: number;
    patch: number;
  }

  const splitVersion = function (s: string): Maybe<Version> {
    // Since we control the format, there's no need for a complex parser
    const split = s.split('.');
    if (split.length !== 3) {
      return Maybe.nothing();
    }

    return Maybe.of({
      // handle cases like v1.0.0
      major: parseInt(split[0].replace('v', ''), 10),
      minor: parseInt(split[1], 10),
      patch: parseInt(split[2], 10),
    });
  };

  const isUpdateAvailable = function (v1: Maybe<Version>, v2: Maybe<Version>) {
    // we can't infer anything
    if (v1.isNothing || v2.isNothing) {
      return false;
    }

    const v1Value = v1.value;
    const v2Value = v2.value;

    // Compare major
    if (v2Value.major > v1Value.major) {
      return true;
    }
    if (v2Value.major < v1Value.major) {
      return false;
    }
    // major value is equal

    // compare minor
    if (v2Value.minor > v1Value.minor) {
      return true;
    }
    if (v2Value.minor < v1Value.minor) {
      return false;
    }

    // minor is the same
    // compare patch
    if (v2Value.patch > v1Value.patch) {
      return true;
    }
    if (v2Value.patch < v1Value.patch) {
      return false;
    }

    return false;
  };

  const currVersionObj = splitVersion(version);
  const latestVersionObj = splitVersion(latestVersion);

  if (!isUpdateAvailable(currVersionObj, latestVersionObj)) {
    return null;
  }

  return (
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
  );
}

function Footer() {
  const info = buildInfo();

  return (
    <footer className="footer" title={buildInfoStr(info)}>
      <span>
        {`© Pyroscope ${copyrightYears(
          START_YEAR,
          new Date().getFullYear().toFixed()
        )}`}
      </span>
      &nbsp;&nbsp;|&nbsp;&nbsp;
      <span>{info.version}</span>
      <NewerVersionCheck />
    </footer>
  );
}

export default Footer;
