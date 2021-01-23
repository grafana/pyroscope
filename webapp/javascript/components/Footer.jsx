import React from "react";
import { connect } from "react-redux";

const START_YEAR = 2020;

function copyrightYears(start, end) {
  return start === end ? start : `${start} – ${end}`;
}

function version() {
  return `v${PYROSCOPE_VERSION}`;
}

function Footer(props) {
  // let flags = BUILD_FLAGS.split("\n").map(x => x.replace("-X github.com/pyroscope-io/pyroscope/pkg/build.", ""));
  // console.log(flags);
  return (
    <div className="footer">
      <span title={version()}>
        © Pyroscope {copyrightYears(START_YEAR, new Date().getFullYear())}
      </span>
    </div>
    /* <FontAwesomeIcon icon={faGitHub} /> */
  );
}

export default connect((x) => x, {})(Footer);
