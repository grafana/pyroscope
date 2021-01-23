import React, { useState } from "react";
import { connect } from "react-redux";
import { addLabel } from "../redux/actions";

const initialState = {
  name: "",
  value: "",
};

function LabelsFilter() {
  const [state, setState] = useState(initialState);
  const updateCurrentLabel = (name) => {
    setState({ name });
  };
  const updateCurrentValue = (value) => {
    setState({ value });
  };

  return (
    <form className="labels-new-label" onSubmit={addLabel}>
      <input
        className="labels-new-input"
        onChange={(e) => updateCurrentLabel(e.target.value)}
        placeholder="Name"
        value={state.name}
      />
      <input
        className="labels-new-input"
        onChange={(e) => updateCurrentValue(e.target.value)}
        placeholder="Value"
        value={state.value}
      />
      <button
        className="btn labels-new-btn"
        onClick={(e) => {
          addLabel(state.name, state.value);
          setState(initialState);
          e.preventDefault();
        }}
      >
        Add
      </button>
    </form>
  );
}

export default connect((x) => x, { addLabel })(LabelsFilter);
