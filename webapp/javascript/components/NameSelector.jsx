import React from 'react';
import { connect } from "react-redux";
import { addLabel, receiveJSON} from "../redux/actions";
import { bindActionCreators } from "redux";
import ApiConnectedComponent from "./ApiConnectedComponent";


class NameSelector extends ApiConnectedComponent {
  constructor() {
    super();
    
  }

  selectName = (event) =>{
    this.props.actions.addLabel("__name__", event.target.value)
    .then(() => {
      this.refreshJson()
    })
  }

  render() {
    let names = this.props.names || [];
    let selectedName = this.props.labels.filter(x => x.name == "__name__")[0];
    selectedName = selectedName ? selectedName.value : "none";
    return <span>
      Metric:&nbsp;
      <select className="label-select" value={selectedName} onChange={this.selectName}>
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
    </span>
  }
}

const mapStateToProps = state => ({
  ...state,
});

const mapDispatchToProps = dispatch => ({
  actions: bindActionCreators(
    {
      receiveJSON,
      addLabel,
    },
    dispatch,
  ),
});

export default connect(
  mapStateToProps,
  mapDispatchToProps,
)(NameSelector);
