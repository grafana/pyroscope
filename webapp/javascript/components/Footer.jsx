import React from "react";
import { connect } from "react-redux";
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
  // let flags = BUILD_FLAGS.split("\n").map(x => x.replace("-X github.com/pyroscope-io/pyroscope/pkg/build.", ""));
  return (
    <div className="footer">
      <span title={buildInfo()}>
        {`© Pyroscope ${copyrightYears(START_YEAR, new Date().getFullYear())}`}
      </span>
    </div>
    /* <FontAwesomeIcon icon={faGitHub} /> */
  );
}

export default connect((x) => x, {})(Footer);
