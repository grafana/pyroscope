import React from "react";
import { connect } from "react-redux";

import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faSyncAlt } from "@fortawesome/free-solid-svg-icons";
import { refresh } from "../redux/actions";

function RefreshButton() {
  return (
    <div>
      <button className="btn refresh-btn" onClick={() => refresh()}>
        <FontAwesomeIcon icon={faSyncAlt} />
      </button>
    </div>
  );
}

export default connect((x) => x, { refresh })(RefreshButton);
