import React from "react";
import { connect } from "react-redux";
import { bindActionCreators } from "redux";
import { setQuery } from "../redux/actions";

const defKey = "Select an app...";

function NameSelector(props) {
  const { actions, names, query } = props;
  const selectAppName = (event) => {
    actions.setQuery(`${event.target.value}{}`);
  };
  let defaultValue = (query || "").replace(/\{.*/g, "");
  if (names && names.indexOf(defaultValue) === -1) {
    defaultValue = defKey;
  }

  return (
    <span>
      Application:&nbsp;
      <select
        className="label-select"
        value={defaultValue}
        onChange={selectAppName}
      >
        <option disabled key={defKey} value="Select an app...">
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
      setQuery,
    },
    dispatch
  ),
});

export default connect(mapStateToProps, mapDispatchToProps)(NameSelector);
