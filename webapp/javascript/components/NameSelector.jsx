import React from "react";
import { connect } from "react-redux";
import { bindActionCreators } from "redux";
import { addLabel, receiveJSON } from "../redux/actions";

function NameSelector(props) {
  const { actions, names, labels } = props;
  const selectName = (event) => {
    actions.addLabel("__name__", event.target.value);
  };

  let selectedName = labels.filter((x) => x.name === "__name__")[0];
  selectedName = selectedName ? selectedName.value : "none";
  return (
    <span>
      Application:&nbsp;
      <select
        className="label-select"
        value={selectedName}
        onChange={selectName}
      >
        <option disabled key="Select an app..." value="Select an app...">
          Select an app...
        </option>
        {names &&
          names.map((name) => (
            <option key={name} value={name}>
              {name}
            </option>
          ))}
      </select>
    </span>
  );
}

const mapStateToProps = (state) => ({
  ...state,
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      receiveJSON,
      addLabel,
    },
    dispatch
  ),
});

export default connect(mapStateToProps, mapDispatchToProps)(NameSelector);
