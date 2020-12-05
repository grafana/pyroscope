import React from 'react';
import { connect } from "react-redux";
import { addLabel, fetchNames } from "../redux/actions";

const initialState = {
  name: "",
  value: ""
}
class LabelsFilter extends React.Component {
  constructor(props) {
    super(props);
    this.state = initialState;
  }

  componentDidMount = () => {
    console.log('componentDidMount');
    this.props.fetchNames();
  }

  updateCurrentLabel = (name) => {
    this.setState({ name });
  };

  updateCurrentValue = (value) => {
    this.setState({ value });
  };

  addLabel = (e) => {
    this.props.addLabel(this.state.name, this.state.value);
    this.setState(initialState);
    e.preventDefault();
  };

  render() {
    return <form className="labels-new-label" onSubmit={this.addLabel}>
      <input
        className="labels-new-input"
        onChange={(e) => this.updateCurrentLabel(e.target.value)}
        placeholder="Name"
        value={this.state.name}
      />
      <input
        className="labels-new-input"
        onChange={(e) => this.updateCurrentValue(e.target.value)}
        placeholder="Value"
        value={this.state.value}
      />
      <button className="btn labels-new-btn" onClick={this.addLabel}>Add</button>
    </form>
  }
}

export default connect(
  (x) => x,
  { addLabel, fetchNames }
)(LabelsFilter);
