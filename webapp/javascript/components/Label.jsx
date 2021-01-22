import React from "react";
import { connect } from "react-redux";
import { removeLabel } from "../redux/actions";

function Label(props) {
  const { label, removeLabel } = props;

  return (
    <div className="label">
      <span className="label-name">{label.name}</span>
      <span className="label-value">{label.value}</span>
      <button
        className="label-delete-btn"
        onClick={() => removeLabel(label.name)}
      />
    </div>
  );
}

export default connect((x) => x, { removeLabel })(Label);
