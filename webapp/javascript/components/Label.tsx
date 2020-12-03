import React from 'react';
import { connect } from "react-redux";
import { removeLabel } from "../redux/actions";

class Label extends React.Component {
  constructor(props) {
    super(props);
  }

  removeLabel = () => {
    this.props.removeLabel(this.props.label.name);
  };

  render() {
    return <div className="label">
      <span className="label-name">{this.props.label.name}</span>
      <span className="label-value">{this.props.label.value}</span>
      <button className="label-delete-btn" onClick={this.removeLabel}></button>
    </div>
  }
}

export default connect(
  (x) => x,
  { removeLabel }
)(Label);
