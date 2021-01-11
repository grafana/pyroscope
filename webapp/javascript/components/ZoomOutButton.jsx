import React from 'react';
import { connect } from "react-redux";
import { setDateRange } from "../redux/actions";

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSearchMinus } from '@fortawesome/free-solid-svg-icons'

class ZoomOutButton extends React.Component {
  constructor(props) {
    super(props);
  }

  zoomOut = () => {
    let from = this.props.from;
    let until = this.props.until;
    this.props.setDateRange(from, until);
  };

  render() {
    return <div>
      <button className="btn" onClick={this.zoomOut}>
        &nbsp;
        <FontAwesomeIcon icon={faSearchMinus} />
        &nbsp;
      </button>
    </div>
  }
}

export default connect(
  (x) => x,
  { setDateRange }
)(ZoomOutButton);
