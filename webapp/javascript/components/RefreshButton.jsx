import React from 'react';
import { connect } from "react-redux";
import { refresh } from "../redux/actions";

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSyncAlt } from '@fortawesome/free-solid-svg-icons'

class RefreshButton extends React.Component {
  constructor(props) {
    super(props);
  }

  refresh = () => {
    this.props.refresh();
  };

  render() {
    return <div>
      <button className="btn refresh-btn" onClick={this.refresh}>
        <FontAwesomeIcon icon={faSyncAlt} />
      </button>
    </div>
  }
}

export default connect(
  (x) => x,
  { refresh }
)(RefreshButton);
