import React from "react";
import { connect } from "react-redux";

import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faDownload } from "@fortawesome/free-solid-svg-icons";
import { version } from "../../../package.json";

const START_YEAR = 2020;
const PYROSCOPE_VERSION = version;

function copyrightYears(start, end) {
  return start === end ? start : `${start} – ${end}`;
}

function buildInfo() {
  return `
    BUILD INFO:
    js_version: v${PYROSCOPE_VERSION}
    goos: ${window.buildInfo.goos}
    goarch: ${window.buildInfo.goarch}
    go_version: ${window.buildInfo.goVersion}
    version: ${window.buildInfo.version}
    id: ${window.buildInfo.id}
    time: ${window.buildInfo.time}
    gitSHA: ${window.buildInfo.gitSHA}
    gitDirty: ${window.buildInfo.gitDirty}
    embeddedAssets: ${window.buildInfo.useEmbeddedAssets}
`.replace(/^\s+/gm, "");
}

function Footer() {
  const latestVersion = window.latestVersionInfo.latest_version;
  const newVersionAvailable =
    latestVersion && window.buildInfo.version !== latestVersion;

  return (
    <div className="footer" title={buildInfo()}>
      <span>
        {`© Pyroscope ${copyrightYears(START_YEAR, new Date().getFullYear())}`}
      </span>
      &nbsp;&nbsp;|&nbsp;&nbsp;
      <span>{window.buildInfo.version}</span>
      {newVersionAvailable && (
        <span>
          &nbsp;&nbsp;|&nbsp;&nbsp;
          <a
            href="https://pyroscope.io/downloads?utm_source=pyroscope_footer"
            rel="noreferrer"
            target="_blank"
          >
            <FontAwesomeIcon icon={faDownload} />&nbsp;<span>Newer Version Available ({latestVersion})</span>
          </a>
        </span>
      )}
    </div>
  );
}

export default connect((x) => x, {})(Footer);
