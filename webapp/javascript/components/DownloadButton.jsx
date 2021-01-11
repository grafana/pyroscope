import React from 'react';
import { connect } from "react-redux";

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faDownload } from '@fortawesome/free-solid-svg-icons'

class RefreshButton extends React.Component {
  constructor(props) {
    super(props);
  }

  download = () => {
    window.document.location.href = this.props.renderURL;
    // window.open();
  };

  render() {
    return <div>
      <button className="btn" onClick={this.download}>
        &nbsp;
        <FontAwesomeIcon icon={faDownload} />
        &nbsp;
      </button>
    </div>
  }
}

export default connect(
  (x) => x,
)(RefreshButton);
