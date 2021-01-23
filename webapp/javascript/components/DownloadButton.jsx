import React from "react";
import { connect } from "react-redux";

import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faDownload } from "@fortawesome/free-solid-svg-icons";

function RefreshButton(props) {
  const { renderURL } = props;
  const download = () => {
    window.document.location.href = renderURL;
    // window.open();
  };

  return (
    <div>
      <button className="btn" onClick={download}>
        &nbsp;
        <FontAwesomeIcon icon={faDownload} />
        &nbsp;
      </button>
    </div>
  );
}

export default connect((x) => x)(RefreshButton);
