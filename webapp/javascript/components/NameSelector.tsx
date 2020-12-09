import React from 'react';
import { connect } from "react-redux";
import { addLabel } from "../redux/actions";

class NameSelector extends React.Component {
  constructor(props) {
    super(props);
  }

  select = (event) =>{
    this.props.addLabel("__name__", event.target.value);
  }

  render() {
    let names = this.props.names || [];
    let selectedName = this.props.labels.filter(x => x.name == "__name__")[0];
    selectedName = selectedName ? selectedName.value : "none";
    return <select className="label-select" value={selectedName} onChange={this.select}>
      <option
        disabled
        key="Select an app..."
        value="Select an app..."
      >Select an app...</option>
      {names.map(function(name) {
        return <option
          key={name}
          value={name}
        >{name}</option>;
      })}
    </select>
  }
}

export default connect(
  (x) => x,
  { addLabel }
)(NameSelector);
