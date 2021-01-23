import React from "react";
import { connect } from "react-redux";

import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faSearchMinus } from "@fortawesome/free-solid-svg-icons";
import { setDateRange } from "../redux/actions";

function ZoomOutButton(props) {
  const { from, until, setDateRange } = props;

  const zoomOut = () => {
    setDateRange(from, until);
  };

  return (
    <div>
      <button className="btn" onClick={zoomOut}>
        &nbsp;
        <FontAwesomeIcon icon={faSearchMinus} />
        &nbsp;
      </button>
    </div>
  );
}

export default connect((x) => x, { setDateRange })(ZoomOutButton);
