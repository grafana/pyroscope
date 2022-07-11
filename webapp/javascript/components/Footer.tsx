import React from 'react';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faDownload } from '@fortawesome/free-solid-svg-icons/faDownload';
import { BuildInfo, buildInfo, latestVersionInfo } from '../util/buildInfo';

const START_YEAR = '2020';

function copyrightYears(start: string, end: string) {
  return start === end ? start : `${start} – ${end}`;
}

function buildInfoStr(buildInfo: BuildInfo) {
  return `
    BUILD INFO:
    js_version: v${buildInfo.jsVersion}
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

//function NewerVersionCheck() {
//  const latestVersion = (win as ShamefulAny).latestVersionInfo
//    ?.latest_version as string;
//  const newVersionAvailable =
//    latestVersion && win?.buildInfo?.version !== latestVersion;
//
//  if (!newVersionAvailable) {
//    return null;
//  }
//
//  return (
//    <span>
//      &nbsp;&nbsp;|&nbsp;&nbsp;
//      <a
//        href="https://pyroscope.io/downloads?utm_source=pyroscope_footer"
//        rel="noreferrer"
//        target="_blank"
//      >
//        <FontAwesomeIcon icon={faDownload} />
//        &nbsp;
//        <span>{`Newer Version Available (${latestVersion})`}</span>
//      </a>
//    </span>
//  );
//}
//
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
    </footer>
  );
}

export default Footer;
